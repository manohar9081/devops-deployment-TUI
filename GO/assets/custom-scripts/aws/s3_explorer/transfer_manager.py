"""Transfer queue — manages concurrent S3 download/upload operations.

Runs transfers in background QThreads so the GUI stays responsive.
"""

from __future__ import annotations

import enum
import os
import traceback
from dataclasses import dataclass, field
from typing import Callable, Optional

from PyQt6.QtCore import QObject, QThread, pyqtSignal, QMutex

from s3_client import S3Client, S3Error


class TransferType(enum.Enum):
    DOWNLOAD = "download"
    UPLOAD = "upload"


class TransferStatus(enum.Enum):
    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    DONE = "done"
    FAILED = "failed"
    CANCELLED = "cancelled"


@dataclass
class TransferItem:
    """One item in the transfer queue."""
    id: str
    type: TransferType
    # S3 side
    bucket: str
    key: str
    # Local side
    local_path: str
    # Progress
    status: TransferStatus = TransferStatus.PENDING
    size_bytes: int = 0
    bytes_done: int = 0
    error_message: str = ""
    local_file_count: int = 1  # for directories
    files_done: int = 0


# ── Worker thread ──────────────────────────────────────────────────────────────

class _TransferWorker(QThread):
    """Background thread that executes a single transfer.

    Uses the built-in QThread.finished signal (fires when run() returns) to
    notify the manager.  Result is stored on the instance so the handler can
    read it without relying on a custom signal across threads.
    """

    progress = pyqtSignal(int, int)   # bytes_chunk, cumulative_bytes
    file_done = pyqtSignal(str, str)  # item_id, filename (for dirs)

    def __init__(
        self,
        item: TransferItem,
        s3_client: S3Client,
        parent: Optional[QObject] = None,
    ):
        super().__init__(parent)
        self.item = item
        self._s3 = s3_client
        self._cancelled = False
        self._mutex = QMutex()
        # Result stored here; read after finished fires.
        self.result_ok = False
        self.result_error = ""

    def cancel(self):
        self._mutex.lock()
        self._cancelled = True
        self._mutex.unlock()

    def run(self):
        item = self.item
        ok = False
        error = ""

        try:
            if item.type == TransferType.DOWNLOAD:
                if item.key.endswith("/"):
                    self._s3.download_prefix(
                        item.bucket,
                        item.key,
                        item.local_path,
                        progress_callback=lambda key, total, done: (
                            self.file_done.emit(item.id, key),
                            self.progress.emit(done, total)
                        ),
                    )
                else:
                    self._s3.download_file(
                        item.bucket,
                        item.key,
                        item.local_path,
                        progress_callback=lambda now, total: self.progress.emit(now, total),
                    )
                ok = True

            elif item.type == TransferType.UPLOAD:
                if os.path.isdir(item.local_path):
                    self._s3.upload_directory(
                        item.local_path,
                        item.bucket,
                        prefix=item.key,
                        progress_callback=lambda path, total, done: (
                            self.file_done.emit(item.id, path),
                            self.progress.emit(done, total)
                        ),
                    )
                else:
                    self._s3.upload_file(
                        item.local_path,
                        item.bucket,
                        item.key,
                        progress_callback=lambda now, total: self.progress.emit(now, total),
                    )
                ok = True

        except S3Error as e:
            error = str(e)
        except Exception as e:
            error = f"{type(e).__name__}: {e}\n{traceback.format_exc()}"

        self.result_ok = ok
        self.result_error = error


# ── Manager ────────────────────────────────────────────────────────────────────

class TransferManager(QObject):
    """Manages the global transfer queue and worker threads.

    Signals:
      item_added:     TransferItem was added to the queue
      item_updated:   TransferItem progress/status changed
      item_removed:   TransferItem was removed from the queue
      queue_cleared:  All items removed
    """

    MAX_CONCURRENT = 3

    item_added = pyqtSignal(object)      # TransferItem
    item_updated = pyqtSignal(object)    # TransferItem
    item_removed = pyqtSignal(str)       # item_id
    queue_cleared = pyqtSignal()

    def __init__(self, s3_client: S3Client, parent: Optional[QObject] = None):
        super().__init__(parent)
        self._s3 = s3_client
        self._queue: dict[str, TransferItem] = {}
        self._active: dict[str, _TransferWorker] = {}
        self._next_id = 1
        self._mutex = QMutex()

    # ── Public API ─────────────────────────────────────────────────────────────

    def enqueue(self, item: TransferItem) -> str:
        """Add a transfer item to the queue. Returns the item ID."""
        self._mutex.lock()
        item.id = f"t{self._next_id}"
        self._next_id += 1
        self._queue[item.id] = item
        self._mutex.unlock()

        self.item_added.emit(item)
        self._kick_pending()
        return item.id

    def cancel(self, item_id: str):
        """Cancel a pending or in-progress transfer."""
        self._mutex.lock()
        if item_id in self._active:
            self._active[item_id].cancel()
        if item_id in self._queue:
            self._queue[item_id].status = TransferStatus.CANCELLED
            item = self._queue[item_id]
        self._mutex.unlock()
        # Worker will finish on its own; _on_worker_finished guards against
        # overwriting the CANCELLED status.
        self.item_updated.emit(item)

    def clear_finished(self):
        """Remove all completed (done/failed/cancelled) items from the queue."""
        self._mutex.lock()
        finished_ids = [
            iid for iid, it in self._queue.items()
            if it.status in (TransferStatus.DONE, TransferStatus.FAILED, TransferStatus.CANCELLED)
        ]
        for iid in finished_ids:
            del self._queue[iid]
            self.item_removed.emit(iid)
        self._mutex.unlock()

    def remove(self, item_id: str):
        """Remove a single item from the queue.

        Active transfers are cancelled first; pending/finished ones are just
        removed. Safe to call for any id (no-op if not present).
        """
        self._mutex.lock()
        if item_id in self._active:
            self._active[item_id].cancel()
        if item_id in self._queue:
            del self._queue[item_id]
            present = True
        else:
            present = False
        self._mutex.unlock()
        if present:
            self.item_removed.emit(item_id)

    def clear_all(self):
        """Cancel everything and clear the queue."""
        for worker in list(self._active.values()):
            worker.cancel()
        self._mutex.lock()
        self._queue.clear()
        self._active.clear()
        self._mutex.unlock()
        self.queue_cleared.emit()

    def retry(self, item_id: str):
        """Re-queue a failed or cancelled item."""
        self._mutex.lock()
        if item_id in self._queue:
            item = self._queue[item_id]
            item.status = TransferStatus.PENDING
            item.bytes_done = 0
            item.error_message = ""
            updated = item
        else:
            updated = None
        self._mutex.unlock()
        if updated:
            self.item_updated.emit(updated)
            self._kick_pending()

    @property
    def queue(self) -> dict[str, TransferItem]:
        """Return a copy of the current queue (for UI population)."""
        self._mutex.lock()
        copy = dict(self._queue)
        self._mutex.unlock()
        return copy

    # ── Internal ────────────────────────────────────────────────────────────────

    def _kick_pending(self):
        """Start transfers for pending items if we have free slots."""
        self._mutex.lock()
        running = len(self._active)
        available_slots = self.MAX_CONCURRENT - running
        if available_slots <= 0:
            self._mutex.unlock()
            return

        # Pick pending items (FIFO)
        pending = [
            it for it in self._queue.values()
            if it.status == TransferStatus.PENDING
        ]
        to_start = pending[:available_slots]

        for item in to_start:
            item.status = TransferStatus.IN_PROGRESS
            worker = _TransferWorker(item, self._s3)
            self._active[item.id] = worker
            worker.progress.connect(lambda b, t, iid=item.id: self._on_progress(iid, b, t))
            worker.file_done.connect(lambda fn, iid=item.id: self._on_file_done(iid, fn))
            worker.finished.connect(lambda w=worker, iid=item.id: self._on_worker_finished(iid, w))
            worker.start()

        self._mutex.unlock()
        for item in to_start:
            self.item_updated.emit(item)

    def _on_progress(self, item_id: str, bytes_done: int, total: int):
        self._mutex.lock()
        if item_id in self._queue:
            self._queue[item_id].bytes_done = bytes_done
            item = self._queue[item_id]
        else:
            item = None
        self._mutex.unlock()
        if item:
            self.item_updated.emit(item)

    def _on_file_done(self, item_id: str, filename: str):
        self._mutex.lock()
        if item_id in self._queue:
            self._queue[item_id].files_done += 1
            item = self._queue[item_id]
        else:
            item = None
        self._mutex.unlock()
        if item:
            self.item_updated.emit(item)

    def _on_worker_finished(self, item_id: str, worker: _TransferWorker):
        """Called when QThread.finished fires (after run() returns)."""
        self._mutex.lock()
        if item_id in self._queue:
            status = self._queue[item_id].status
            # Don't overwrite a CANCELLED status.
            if status != TransferStatus.CANCELLED:
                self._queue[item_id].status = TransferStatus.DONE if worker.result_ok else TransferStatus.FAILED
                self._queue[item_id].error_message = worker.result_error
            item = self._queue[item_id]
        else:
            item = None
        if item_id in self._active:
            del self._active[item_id]
        self._mutex.unlock()

        if item:
            self.item_updated.emit(item)
        self._kick_pending()
