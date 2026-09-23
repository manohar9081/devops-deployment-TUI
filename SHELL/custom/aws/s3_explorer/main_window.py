"""Main window — FileZilla-style dual-panel layout."""

from __future__ import annotations

import os
from pathlib import Path

import sys
import os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from PyQt6.QtCore import Qt, QEvent, QSize, pyqtSignal
from PyQt6.QtGui import QDragEnterEvent, QDropEvent
from PyQt6.QtWidgets import (
    QApplication,
    QDockWidget,
    QHBoxLayout,
    QLabel,
    QLineEdit,
    QMainWindow,
    QMessageBox,
    QProgressBar,
    QPushButton,
    QSplitter,
    QStatusBar,
    QTableWidget,
    QTableWidgetItem,
    QToolBar,
    QVBoxLayout,
    QWidget,
)

from local_browser import LocalTreeView
from s3_client import S3Client, S3Object
from transfer_manager import TransferItem, TransferManager, TransferType
from utils import format_size
from widgets.auth_dialog import AuthDialog
from widgets.s3_tree import S3TreeWidget
from widgets.transfer_queue import TransferQueueWidget


class MainWindow(QMainWindow):
    """FileZilla-style S3 Explorer main window.

    Layout:
      ┌────────────────────────────────────────────────────────────┐
      │  Toolbar: [Connect] [Disconnect] | [Refresh] | [Profile]   │
      ├────────────────────────────────────────────────────────────┤
      │  ┌─────────────────────┐ ║ ┌─────────────────────────────┐│
      │  │  S3 Browser Panel   │ ║ │   Local Browser Panel      ││
      │  │  (left: buckets,   │ ║ │   (right: local dirs)       ││
      │  │   objects)          │ ║ │                             ││
      │  │                     │ ║ │                             ││
      │  └─────────────────────┘ ║ └─────────────────────────────┘│
      ├────────────────────────────────────────────────────────────┤
      │  Transfer Queue (bottom panel)                            │
      └────────────────────────────────────────────────────────────┘
    │  Status bar                                                   │
    └────────────────────────────────────────────────────────────┘
    """

    def __init__(self):
        super().__init__()
        self._s3: S3Client | None = None
        self._transfer_manager: TransferManager | None = None

        self.setWindowTitle("S3 Explorer")
        self.setMinimumSize(1000, 600)
        self.resize(1280, 760)

        self._build_ui()
        self._connect_signals()
        self._update_ui_state()

        # Auto-prompt for connection on startup
        QApplication.instance().postEvent(self, _AuthPromptEvent())

    # ── UI Construction ─────────────────────────────────────────────────────────

    def _build_ui(self):
        self._build_toolbar()
        self._build_central_widget()
        self._build_transfer_panel()
        self._build_status_bar()

    def _build_toolbar(self):
        toolbar = QToolBar("Main Toolbar")
        toolbar.setIconSize(QSize(20, 20))
        toolbar.setMovable(False)
        self.addToolBar(toolbar)

        self.btn_connect = QPushButton("🔗 Connect")
        self.btn_connect.clicked.connect(self._on_connect_clicked)
        toolbar.addWidget(self.btn_connect)

        self.btn_disconnect = QPushButton("⛓ Disconnect")
        self.btn_disconnect.clicked.connect(self._on_disconnect_clicked)
        toolbar.addWidget(self.btn_disconnect)

        toolbar.addSeparator()

        self.btn_refresh = QPushButton("↻ Refresh")
        self.btn_refresh.clicked.connect(self._on_refresh_clicked)
        toolbar.addWidget(self.btn_refresh)

        toolbar.addSeparator()

        self.btn_s3_up = QPushButton("↑ Up")
        self.btn_s3_up.setToolTip("Navigate S3 up one level")
        self.btn_s3_up.clicked.connect(self._on_s3_up_clicked)
        toolbar.addWidget(self.btn_s3_up)

        self.btn_local_up = QPushButton("↑ Up")
        self.btn_local_up.setToolTip("Navigate local up one level")
        self.btn_local_up.clicked.connect(self._on_local_up_clicked)
        toolbar.addWidget(self.btn_local_up)

        self.btn_local_home = QPushButton("🏠 Home")
        self.btn_local_home.setToolTip("Go to home directory")
        self.btn_local_home.clicked.connect(self._on_local_home_clicked)
        toolbar.addWidget(self.btn_local_home)

        toolbar.addSeparator()

        # Manual transfer buttons
        self.btn_download = QPushButton("⬇ Download")
        self.btn_download.clicked.connect(self._on_download_clicked)
        toolbar.addWidget(self.btn_download)

        self.btn_upload = QPushButton("⬆ Upload")
        self.btn_upload.clicked.connect(self._on_upload_clicked)
        toolbar.addWidget(self.btn_upload)

        toolbar.addSeparator()

        # Profile display
        self.profile_label = QLabel("Not connected")
        self.profile_label.setStyleSheet("color: #666; font-size: 12px; padding: 0 8px;")
        toolbar.addWidget(self.profile_label)

        self.setContextMenuPolicy(Qt.ContextMenuPolicy.NoContextMenu)

    def _build_central_widget(self):
        central = QWidget()
        layout = QHBoxLayout(central)
        layout.setContentsMargins(4, 4, 4, 4)
        layout.setSpacing(4)

        # Splitter between S3 panel and local panel
        self.splitter = QSplitter(Qt.Orientation.Horizontal)

        # Left: S3 browser
        self.s3_panel = S3TreeWidget()
        self.s3_panel.setMinimumWidth(340)

        # Right: Local browser (drop zone)
        self.local_panel = LocalDropZone()
        self.local_panel.setMinimumWidth(340)

        self.splitter.addWidget(self.s3_panel)
        self.splitter.addWidget(self.local_panel)
        self.splitter.setStretchFactor(0, 1)
        self.splitter.setStretchFactor(1, 1)

        layout.addWidget(self.splitter)
        self.setCentralWidget(central)

    def _build_transfer_panel(self):
        self.transfer_dock = QDockWidget("Transfer Queue", self)
        self.transfer_dock.setObjectName("TransferQueueDock")
        self.transfer_dock.setFeatures(
            QDockWidget.DockWidgetFeature.DockWidgetClosable
            | QDockWidget.DockWidgetFeature.DockWidgetFloatable
        )
        self.transfer_queue_widget = TransferQueueWidget(
            self._transfer_manager or TransferManager(None)
        )
        self.transfer_dock.setWidget(self.transfer_queue_widget)
        self.addDockWidget(Qt.DockWidgetArea.BottomDockWidgetArea, self.transfer_dock)

    def _build_status_bar(self):
        self.status_bar = QStatusBar()
        self.setStatusBar(self.status_bar)
        self._status_label = QLabel("Not connected")
        self.status_bar.addWidget(self._status_label, 1)
        self._progress_bar = QProgressBar()
        self._progress_bar.setFixedWidth(200)
        self._progress_bar.setVisible(False)
        self.status_bar.addPermanentWidget(self._progress_bar)

    # ── Signals ────────────────────────────────────────────────────────────────

    def _connect_signals(self):
        # S3 panel double-click navigation (handled inside the tree widget)
        self.s3_panel.tree.doubleClicked.connect(self._on_s3_navigate)
        # Drag S3 → local panel = download; drag local → S3 panel = upload
        self.local_panel.download_requested.connect(self._enqueue_download)
        self.s3_panel.upload_dropped.connect(self._on_s3_upload_dropped)

    # ── Event Handlers ─────────────────────────────────────────────────────────

    def _on_connect_clicked(self):
        self._show_auth_dialog()

    def _on_disconnect_clicked(self):
        if self._transfer_manager:
            self._transfer_manager.clear_all()
        self._s3 = None
        self._transfer_manager = None
        self.s3_panel.set_client(None)
        self._transfer_queue_widget.setManager(None)
        self.transfer_queue_widget = TransferQueueWidget(TransferManager(None))
        self.profile_label.setText("Not connected")
        self._status_label.setText("Disconnected")
        self._update_ui_state()
        self.s3_panel.path_bar.setText("s3:// (not connected)")

    def _on_refresh_clicked(self):
        if self._s3 and self.s3_panel.tree._current_bucket:
            self.s3_panel.tree._load_current_path()
        elif self._s3:
            self.s3_panel.load_buckets()
        else:
            QMessageBox.information(self, "Not Connected", "Connect to AWS first.")

    def _on_s3_up_clicked(self):
        self.s3_panel.navigate_up()

    def _on_local_up_clicked(self):
        self.local_panel.browser.navigate_up()

    def _on_local_home_clicked(self):
        self.local_panel.browser.set_home()

    def _on_s3_navigate(self, index):
        # Handled inside S3TreeWidget itself
        pass

    def _on_download_clicked(self):
        """Download selected S3 items to current local directory."""
        items = self.s3_panel.selected_items()
        if not items:
            # Try current bucket items
            items = self.s3_panel.tree._model_items
        if not items:
            QMessageBox.information(self, "Nothing Selected", "Select items in the S3 panel first.")
            return
        local_dir = self.local_panel.current_dir()
        self._enqueue_download_from_items(items, local_dir)

    def _on_upload_clicked(self):
        """Upload selected local items to current S3 bucket/prefix."""
        paths = self.local_panel.selected_local_paths()
        if not paths:
            QMessageBox.information(self, "Nothing Selected", "Select files/folders in the local panel first.")
            return
        bucket = self.s3_panel.current_bucket() or ""
        prefix = self.s3_panel.tree._current_prefix or ""
        self._enqueue_upload_from_paths(paths, bucket, prefix)

    def _on_s3_upload_dropped(self, local_paths: list[str]):
        """Local files were dragged onto the S3 panel — enqueue uploads."""
        if not self._s3:
            QMessageBox.information(self, "Not Connected", "Connect to AWS (or use Demo Mode) first.")
            return
        bucket = self.s3_panel.current_bucket() or ""
        if not bucket:
            QMessageBox.information(
                self, "Open a Bucket",
                "Open a bucket on the left panel first, then drop files to upload.",
            )
            return
        prefix = self.s3_panel.tree._current_prefix or ""
        self._enqueue_upload(local_paths, bucket, prefix)

    # ── Auth ───────────────────────────────────────────────────────────────────

    def _show_auth_dialog(self):
        dialog = AuthDialog(self)
        dialog.authenticated.connect(self._on_authenticated)
        dialog.exec()

    def _on_authenticated(self, s3_client: S3Client):
        self._s3 = s3_client
        # DemoS3Client exposes a fake session; real clients have a boto3 session.
        profile = getattr(s3_client._session, "profile_name", None) or "default"
        is_demo = profile == "demo"
        self.profile_label.setText(f"Profile: {profile}" + ("  (DEMO)" if is_demo else ""))
        if is_demo:
            self.profile_label.setStyleSheet(
                "color: #b8860b; font-size: 12px; padding: 0 8px; font-weight: bold;"
            )
        else:
            self.profile_label.setStyleSheet("color: #666; font-size: 12px; padding: 0 8px;")

        # Transfer manager
        self._transfer_manager = TransferManager(s3_client)

        # Wire up panels
        self.s3_panel.set_client(s3_client)
        self.s3_panel.load_buckets()

        # Rebuild transfer queue widget with the real manager.
        # (download/upload drag signals are already connected once in
        # _connect_signals — do not re-connect here.)
        new_tqw = TransferQueueWidget(self._transfer_manager)
        self.transfer_dock.setWidget(new_tqw)
        self.transfer_queue_widget = new_tqw

        status = f"Demo mode — dummy data" if is_demo else f"Connected — {profile}"
        self._status_label.setText(status)
        self._update_ui_state()

    # ── Transfer Enqueueing ────────────────────────────────────────────────────

    def _enqueue_download(self, bucket: str, keys: list[str], local_dir: str):
        for key in keys:
            if not key:
                continue
            local_path = os.path.join(local_dir, os.path.basename(key.rstrip("/")))
            item = TransferItem(
                id="",
                type=TransferType.DOWNLOAD,
                bucket=bucket,
                key=key,
                local_path=local_path,
                size_bytes=0,
            )
            self._transfer_manager.enqueue(item)

    def _enqueue_upload(self, local_paths: list[str], bucket: str, prefix: str):
        for local_path in local_paths:
            if not local_path:
                continue
            name = os.path.basename(local_path.rstrip(os.sep))
            key = f"{prefix}/{name}".lstrip("/")
            is_dir = os.path.isdir(local_path)
            size = 0 if is_dir else os.path.getsize(local_path)
            item = TransferItem(
                id="",
                type=TransferType.UPLOAD,
                bucket=bucket,
                key=key,
                local_path=local_path,
                size_bytes=size,
                local_file_count=len(os.listdir(local_path)) if is_dir else 1,
            )
            self._transfer_manager.enqueue(item)

    def _enqueue_download_from_items(self, items: list[S3Object], local_dir: str):
        bucket = self.s3_panel.current_bucket()
        if not bucket:
            return
        for obj in items:
            local_path = os.path.join(local_dir, os.path.basename(obj.key.rstrip("/")))
            item = TransferItem(
                id="",
                type=TransferType.DOWNLOAD,
                bucket=bucket,
                key=obj.key,
                local_path=local_path,
                size_bytes=obj.size or 0,
            )
            self._transfer_manager.enqueue(item)

    def _enqueue_upload_from_paths(self, paths: list[str], bucket: str, prefix: str):
        for local_path in paths:
            name = os.path.basename(local_path.rstrip(os.sep))
            key = f"{prefix}/{name}".lstrip("/")
            is_dir = os.path.isdir(local_path)
            size = 0 if is_dir else os.path.getsize(local_path)
            item = TransferItem(
                id="",
                type=TransferType.UPLOAD,
                bucket=bucket,
                key=key,
                local_path=local_path,
                size_bytes=size,
                local_file_count=len(os.listdir(local_path)) if is_dir else 1,
            )
            self._transfer_manager.enqueue(item)

    # ── UI State ────────────────────────────────────────────────────────────────

    def _update_ui_state(self):
        connected = self._s3 is not None
        self.btn_disconnect.setEnabled(connected)
        self.btn_refresh.setEnabled(connected)
        self.btn_s3_up.setEnabled(connected)
        self.btn_download.setEnabled(connected and self.s3_panel.current_bucket() is not None)
        self.btn_upload.setEnabled(connected and self.s3_panel.current_bucket() is not None)
        self.btn_connect.setText("🔗 Connect" if not connected else "🔗 Reconnect")

    # ── Custom Event ───────────────────────────────────────────────────────────

    def customEvent(self, event):
        if isinstance(event, _AuthPromptEvent):
            self._show_auth_dialog()


class _AuthPromptEvent(QEvent):
    """Synthetic event to trigger the auth dialog after startup."""
    AUTH_TYPE = QEvent.Type(QEvent.registerEventType())

    def __init__(self):
        super().__init__(self.AUTH_TYPE)


# ── Local Drop Zone ────────────────────────────────────────────────────────────

class LocalDropZone(QWidget):
    """Wrapper around LocalTreeView that acts as a drop target for S3 URIs.

    Drops from S3 panel → download.
    """

    # Signals
    download_requested = pyqtSignal(str, list, str)   # bucket, [keys], local_dir
    upload_requested   = pyqtSignal(list, str, str)   # [local_paths], bucket, prefix

    MIME_S3 = "application/x-s3-uri"

    def __init__(self, parent=None):
        super().__init__(parent)

        layout = QVBoxLayout(self)
        layout.setContentsMargins(0, 0, 0, 0)

        # Address bar
        self.address_bar = QLineEdit()
        self.address_bar.setReadOnly(True)
        self.address_bar.setPlaceholderText("Local directory…")
        layout.addWidget(self.address_bar)

        # Tree view
        self.browser = LocalTreeView()
        self.browser.set_home()
        self.browser.folder_entered.connect(self._on_folder_entered)
        layout.addWidget(self.browser)

        self._update_address()

        self.setAcceptDrops(True)

    def current_dir(self) -> str:
        return self.browser.root_path()

    def selected_local_paths(self) -> list[str]:
        return self.browser.selected_paths()

    def _on_folder_entered(self, path: str):
        self._update_address()

    def _update_address(self):
        self.address_bar.setText(self.browser.root_path())

    # ── Drag & Drop ─────────────────────────────────────────────────────────────

    def dragEnterEvent(self, event: QDragEnterEvent):
        mime = event.mimeData()
        if mime.hasFormat(self.MIME_S3) or mime.hasText():
            event.acceptProposedAction()
        else:
            super().dragEnterEvent(event)

    def dropEvent(self, event: QDropEvent):
        mime = event.mimeData()
        if not mime.hasFormat(self.MIME_S3):
            super().dropEvent(event)
            return

        raw = bytes(mime.data(self.MIME_S3)).decode("utf-8")
        uris = [u.strip() for u in raw.splitlines() if u.strip()]

        if not uris:
            super().dropEvent(event)
            return

        # Parse S3 URIs: s3://bucket/key
        bucket = None
        keys = []
        for uri in uris:
            if uri.startswith("s3://"):
                parts = uri[5:].split("/", 1)
                bucket = parts[0]
                if len(parts) > 1 and parts[1]:
                    keys.append(parts[1])
                else:
                    keys.append("")

        if not bucket:
            super().dropEvent(event)
            return

        # Emit download to current local directory
        self.download_requested.emit(bucket, keys, self.current_dir())
        event.acceptProposedAction()
