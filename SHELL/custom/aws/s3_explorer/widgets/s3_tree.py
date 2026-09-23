"""S3 bucket/object tree browser widget.

Left panel of the FileZilla-style layout.
Draggable items (mime: application/x-s3-uri).
"""

from __future__ import annotations

import os
import sys
import uuid
from pathlib import Path
from typing import Optional

# Allow absolute imports when run as a script
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from PyQt6.QtCore import QMimeData, Qt, QThread, pyqtSignal, QModelIndex, QUrl
from PyQt6.QtGui import QDrag, QDragEnterEvent, QDragMoveEvent, QDropEvent, QIcon, QStandardItem, QStandardItemModel
from PyQt6.QtWidgets import (
    QAbstractItemView,
    QLineEdit,
    QMessageBox,
    QProgressDialog,
    QTreeView,
    QVBoxLayout,
    QWidget,
)

from s3_client import S3Bucket, S3Client, S3Error, S3Object


# ── Background listing thread ───────────────────────────────────────────────────

class _ListWorker(QThread):
    """Loads bucket contents in the background."""
    resultsReady = pyqtSignal(list)       # list of S3Object
    errorOccurred = pyqtSignal(str)        # error message

    def __init__(self, s3: S3Client, bucket: str, prefix: str, parent=None):
        super().__init__(parent)
        self._s3 = s3
        self._bucket = bucket
        self._prefix = prefix

    def run(self):
        try:
            items, _ = self._s3.list_objects(self._bucket, self._prefix)
            self.resultsReady.emit(items)
        except S3Error as e:
            self.errorOccurred.emit(str(e))


# ── Tree model ─────────────────────────────────────────────────────────────────

class S3TreeModel(QTreeView):
    """Tree view showing buckets at the root and objects grouped by prefix."""

    # Emitted when the user double-clicks a folder
    folder_requested = pyqtSignal(str, str)  # bucket, prefix
    # Emitted when local files are dropped here (upload). Payload: list of local paths.
    upload_dropped = pyqtSignal(list)

    # Supported MIME type for drag
    MIME_TYPE = "application/x-s3-uri"

    def __init__(self, s3_client: Optional[S3Client] = None, parent=None):
        super().__init__(parent)
        self._s3 = s3_client
        self._current_bucket: Optional[str] = None
        self._current_prefix: str = ""
        self._current_items: list[S3Object] = []
        self._child_workers: dict[str, _ListWorker] = {}
        self._is_loading = False

        # Internal flat list model
        self._model_items: list[S3Object] = []
        self._buckets: list[S3Bucket] = []
        self._is_showing_buckets = True

        self.setHeaderHidden(False)
        self.setColumnWidth(0, 280)   # Name
        self.setColumnWidth(1, 100)   # Size
        self.setColumnWidth(2, 140)   # Last Modified
        self.setColumnWidth(3, 120)   # Type
        self.setAlternatingRowColors(True)
        self.setSelectionMode(QAbstractItemView.SelectionMode.ExtendedSelection)
        self.setAnimated(True)
        self.setIndentation(16)

        # Drag (out) and drop (in: local files → upload)
        self.setDragEnabled(True)
        self.setAcceptDrops(True)
        self.setDropIndicatorShown(True)
        self.setDefaultDropAction(Qt.DropAction.CopyAction)
        self.setDragDropMode(QAbstractItemView.DragDropMode.DragDrop)

        self.doubleClicked.connect(self._on_double_click)
        # Connect once — previously this was re-connected on every refresh.
        self.expanded.connect(self._on_item_expanded)

    def set_client(self, s3_client: S3Client):
        self._s3 = s3_client

    # ── Public API ─────────────────────────────────────────────────────────────

    def load_buckets(self):
        """Load and display the list of S3 buckets."""
        if not self._s3:
            return
        self._is_showing_buckets = True
        self._current_bucket = None
        self._current_prefix = ""
        self._model_items.clear()
        self._buckets.clear()
        try:
            self._buckets = self._s3.list_buckets()
        except S3Error as e:
            self._show_error(str(e))
            return
        self._update_view()

    def navigate_to(self, bucket: str, prefix: str = ""):
        """Navigate to a specific bucket + prefix."""
        if not self._s3:
            return
        self._current_bucket = bucket
        self._current_prefix = prefix
        self._is_showing_buckets = False
        self._load_current_path()

    def navigate_up(self):
        """Go up one prefix level."""
        if not self._current_bucket:
            self.load_buckets()
            return
        parts = self._current_prefix.rstrip("/").split("/")
        if len(parts) <= 1:
            self._current_bucket = None
            self._current_prefix = ""
            self._is_showing_buckets = True
            self.load_buckets()
        else:
            parts.pop()
            self._current_prefix = "/".join(parts) + "/"
            self._load_current_path()

    def selected_items(self) -> list[S3Object]:
        """Return S3Object list for the currently selected rows."""
        rows = self.selectionModel().selectedRows()
        items = []
        for idx in rows:
            row = idx.row()
            if 0 <= row < len(self._model_items):
                items.append(self._model_items[row])
        return items

    # ── Internal ────────────────────────────────────────────────────────────────

    def _load_current_path(self):
        if not self._s3 or not self._current_bucket:
            return
        self._is_loading = True
        self._model_items.clear()
        self._update_view()
        self.setSortingEnabled(False)

        self._worker = _ListWorker(
            self._s3,
            self._current_bucket,
            self._current_prefix,
            self,
        )
        self._worker.resultsReady.connect(self._on_items_loaded)
        self._worker.errorOccurred.connect(self._show_error)
        self._worker.finished.connect(lambda: setattr(self, "_is_loading", False))
        self._worker.start()

    def _on_items_loaded(self, items: list[S3Object]):
        self._model_items = items
        self._current_items = items
        self._is_loading = False
        self.setSortingEnabled(True)
        self.sortByColumn(0, Qt.SortOrder.AscendingOrder)
        self._update_view()

    def _update_view(self):
        """Re-populate the tree from _model_items / _buckets."""
        model = QStandardItemModel()
        model.setColumnCount(4)
        model.setHeaderData(0, Qt.Orientation.Horizontal, "Name")
        model.setHeaderData(1, Qt.Orientation.Horizontal, "Size")
        model.setHeaderData(2, Qt.Orientation.Horizontal, "Modified")
        model.setHeaderData(3, Qt.Orientation.Horizontal, "Type")

        if self._is_showing_buckets:
            # Show buckets
            for bucket in self._buckets:
                row = [
                    QStandardItem(bucket.name),
                    QStandardItem("Bucket"),
                    QStandardItem(bucket.creation_date or "—" if bucket.creation_date else "—"),
                    QStandardItem("AWS S3"),
                ]
                for col, item in enumerate(row):
                    item.setEditable(col == 0)
                    if col == 0:
                        item.setData({"type": "bucket", "bucket": bucket.name}, Qt.ItemDataRole.UserRole)
                model.appendRow(row)
        else:
            # Show objects / folders
            # Add ".." parent row if not at root
            if self._current_prefix:
                parent_item = [
                    QStandardItem("📁 .."),
                    QStandardItem(""),
                    QStandardItem(""),
                    QStandardItem("Folder"),
                ]
                parent_item[0].setData(
                    {"type": "parent", "bucket": self._current_bucket, "prefix": self._current_prefix},
                    Qt.ItemDataRole.UserRole,
                )
                parent_item[0].setEditable(False)
                parent_item[3].setEditable(False)
                model.appendRow(parent_item)

            for obj in self._model_items:
                if obj.is_folder:
                    icon = "📁"
                    type_label = "Folder"
                else:
                    icon = "📄"
                    type_label = "File"
                row = [
                    QStandardItem(f"{icon} {obj.name}"),
                    QStandardItem(obj.size_str),
                    QStandardItem(obj.last_modified_str),
                    QStandardItem(type_label),
                ]
                for col, item in enumerate(row):
                    item.setEditable(False)
                    if col == 0:
                        item.setData(
                            {
                                "type": "folder" if obj.is_folder else "object",
                                "bucket": self._current_bucket,
                                "key": obj.key,
                                "size": obj.size,
                            },
                            Qt.ItemDataRole.UserRole,
                        )
                model.appendRow(row)

        self.setModel(model)
        self.resizeColumnToContents(0)

    def _on_item_expanded(self, index: QModelIndex):
        """Auto-load children when a folder is expanded."""
        pass  # Flat list for now; future: lazy-load sub-folders

    def _on_double_click(self, index: QModelIndex):
        data = index.data(Qt.ItemDataRole.UserRole)
        if not data:
            return
        t = data.get("type")
        if t == "bucket":
            self.navigate_to(data["bucket"])
        elif t == "parent":
            # Go up
            parts = self._current_prefix.rstrip("/").split("/")
            if len(parts) <= 1:
                self._current_bucket = None
                self._current_prefix = ""
                self._is_showing_buckets = True
                self.load_buckets()
            else:
                parts.pop()
                self._current_prefix = "/".join(parts) + "/"
                self._load_current_path()
        elif t == "folder":
            prefix = data["key"]
            self.navigate_to(self._current_bucket, prefix)

    def _show_error(self, msg: str):
        QMessageBox.warning(self, "S3 Error", msg)

    # ── Drag support ───────────────────────────────────────────────────────────

    def startDrag(self, supportedActions):
        """Start a drag operation for selected S3 items."""
        items = self.selected_items()
        if not items:
            return

        mime = QMimeData()
        uris = []
        for obj in items:
            uri = f"s3://{self._current_bucket}/{obj.key}"
            uris.append(uri)

        mime.setData(self.MIME_TYPE, "\n".join(uris).encode("utf-8"))
        mime.setText("\n".join(uris))

        drag = QDrag(self)
        drag.setMimeData(mime)
        drag.exec()

    # ── Drop support (local files → upload) ─────────────────────────────────────

    def dragEnterEvent(self, event: QDragEnterEvent):
        """Accept drags that carry local file URLs."""
        mime = event.mimeData()
        if mime.hasUrls():
            event.acceptProposedAction()
        else:
            super().dragEnterEvent(event)

    def dragMoveEvent(self, event: QDragMoveEvent):
        mime = event.mimeData()
        if mime.hasUrls():
            event.acceptProposedAction()
        else:
            super().dragMoveEvent(event)

    def dropEvent(self, event: QDropEvent):
        """Drop local files → emit upload_dropped with their paths."""
        mime = event.mimeData()
        if not mime.hasUrls():
            super().dropEvent(event)
            return

        # Only meaningful once a bucket is open. If we're still at the bucket
        # list, there's nowhere to upload to.
        if not self._current_bucket:
            QMessageBox.information(
                self,
                "Open a bucket first",
                "Double-click a bucket on the left, then drop files here to upload.",
            )
            event.ignore()
            return

        paths: list[str] = []
        for url in mime.urls():
            if url.isLocalFile():
                paths.append(url.toLocalFile())

        if paths:
            self.upload_dropped.emit(paths)
            event.acceptProposedAction()
        else:
            event.ignore()


class S3TreeWidget(QWidget):
    """S3 browser panel: toolbar + tree view + path bar."""

    folder_requested = pyqtSignal(str, str)  # bucket, prefix
    upload_dropped = pyqtSignal(list)         # local paths dropped onto the S3 panel

    def __init__(self, s3_client: Optional[S3Client] = None, parent=None):
        super().__init__(parent)
        self._s3 = s3_client

        self.path_bar = QLineEdit()
        self.path_bar.setReadOnly(True)
        self.path_bar.setPlaceholderText("s3://")

        self.tree = S3TreeModel(s3_client, self)
        self.tree.folder_requested.connect(self.folder_requested)
        self.tree.upload_dropped.connect(self.upload_dropped)

        layout = QVBoxLayout(self)
        layout.setContentsMargins(0, 0, 0, 0)
        layout.addWidget(self.path_bar)
        layout.addWidget(self.tree)

        self.setMinimumWidth(320)

    def set_client(self, s3_client: S3Client):
        self._s3 = s3_client
        self.tree.set_client(s3_client)

    def load_buckets(self):
        self.path_bar.setText("s3://")
        self.tree.load_buckets()

    def navigate_to(self, bucket: str, prefix: str = ""):
        path = f"s3://{bucket}/{prefix}".rstrip("/")
        self.path_bar.setText(path)
        self.tree.navigate_to(bucket, prefix)

    def navigate_up(self):
        self.tree.navigate_up()
        if self.tree._current_bucket:
            self.path_bar.setText(
                f"s3://{self.tree._current_bucket}/{self.tree._current_prefix}".rstrip("/")
            )
        else:
            self.path_bar.setText("s3://")

    def selected_items(self) -> list[S3Object]:
        return self.tree.selected_items()

    def current_bucket(self) -> Optional[str]:
        return self.tree._current_bucket
