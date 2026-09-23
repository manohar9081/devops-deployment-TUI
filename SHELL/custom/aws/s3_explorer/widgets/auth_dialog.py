"""AWS profile selector / SSO login dialog — with demo mode for no credentials."""

from __future__ import annotations

from PyQt6.QtCore import Qt, QThread, pyqtSignal
from PyQt6.QtWidgets import (
    QComboBox,
    QDialog,
    QHBoxLayout,
    QLabel,
    QMessageBox,
    QPushButton,
    QVBoxLayout,
)

try:
    import sys as _sys
    import os as _os
    from pathlib import Path
    _sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
    from s3_auth import create_boto3_session, get_available_profiles, validate_credentials
    from s3_client import S3Client
    AWS_AVAILABLE = True
except ImportError:
    AWS_AVAILABLE = False
    S3Client = None


class _ValidationWorker(QThread):
    done = pyqtSignal(bool, str)

    def __init__(self, profile: str):
        super().__init__()
        self._profile = profile

    def run(self):
        ok, msg = validate_credentials(self._profile)
        self.done.emit(ok, msg)


class DemoS3Client:
    """Stub S3 client that serves dummy buckets/objects and *simulates*
    transfers so the full drag-and-drop flow can be exercised without AWS.

    Nothing touches the network. Downloads write small placeholder files
    locally so they show up in the local panel; uploads just simulate progress.
    """

    # ── Dummy dataset ──────────────────────────────────────────────────────────
    # A flat manifest of (key, size, last_modified) per bucket — exactly like
    # real S3, where folders are synthesized from key prefixes.
    _DEMO_DATA: dict[str, list[tuple[str, int, str]]] = {
        "demo-bucket-1": [
            ("README.md",                2_048,    "2024-01-01T09:00:00Z"),
            ("config/app.conf",          4_096,    "2024-02-12T11:30:00Z"),
            ("config/defaults.json",     8_192,    "2024-02-12T11:30:00Z"),
            ("documents/report-q1.pdf",  1_048_576, "2024-04-02T08:15:00Z"),
            ("documents/report-q2.pdf",  1_258_291, "2024-07-04T16:45:00Z"),
            ("documents/notes/meeting.txt",  6_144, "2024-03-18T13:00:00Z"),
            ("images/logo.png",          262_144,  "2024-01-15T10:00:00Z"),
            ("images/banners/hero.jpg",  524_288,  "2024-05-22T18:20:00Z"),
            ("data/export.csv",          524_288,  "2024-06-10T09:30:00Z"),
        ],
        "demo-bucket-2": [
            ("index.html",               16_384,   "2024-06-01T12:00:00Z"),
            ("assets/style.css",         32_768,   "2024-06-01T12:00:00Z"),
            ("assets/app.js",            65_536,   "2024-06-05T15:10:00Z"),
            ("assets/icons/favicon.ico", 4_096,    "2024-06-01T12:00:00Z"),
            ("backups/db-2024-01.sql.gz",4_194_304,"2024-01-31T23:59:00Z"),
            ("photos/2024/sunset.jpg",   1_572_864,"2024-09-12T19:45:00Z"),
            ("photos/2024/beach.png",    2_097_152,"2024-08-03T14:05:00Z"),
            ("photos/2025/snow.jpg",     1_887_436,"2025-01-20T07:30:00Z"),
        ],
    }

    def __init__(self):
        # Fake session so the UI can query profile name
        self._session = type("Session", (), {"profile_name": "demo", "region_name": "us-east-1"})()

    # ── Listing (S3 delimiter semantics) ───────────────────────────────────────

    def list_buckets(self):
        from s3_client import S3Bucket
        return [
            S3Bucket(name="demo-bucket-1", creation_date="2024-01-01T00:00:00Z"),
            S3Bucket(name="demo-bucket-2", creation_date="2024-06-15T00:00:00Z"),
        ]

    def list_objects(self, bucket, prefix="", **kwargs):
        """Mirror S3 list_objects_v2 with a '/' delimiter.

        Returns (items, None). Folders are derived from common prefixes.
        """
        from s3_client import S3Object

        manifest = self._DEMO_DATA.get(bucket, [])
        prefix = prefix or ""
        # Normalize: prefix should not have a leading slash; keep trailing.
        prefix = prefix.lstrip("/")

        files: list[S3Object] = []
        folders: set[str] = set()

        for key, size, mtime in manifest:
            if not key.startswith(prefix):
                continue
            remainder = key[len(prefix):]
            if not remainder:
                continue
            slash = remainder.find("/")
            if slash == -1:
                # Direct file under this prefix
                files.append(S3Object(
                    key=key, size=size, last_modified=mtime, is_folder=False,
                ))
            else:
                # Something nested → synthesize a sub-folder (common prefix)
                folder_name = remainder[:slash + 1]  # includes trailing '/'
                folders.add(prefix + folder_name)

        items: list[S3Object] = [
            S3Object(key=fp, size=None, last_modified=None, is_folder=True) for fp in folders
        ] + files
        items.sort(key=lambda x: (not x.is_folder, x.name.lower()))
        return items, None

    def list_all_objects(self, bucket, prefix="", progress_callback=None):
        """Yield every (non-folder) object under a prefix — used by folder download."""
        from s3_client import S3Object
        manifest = self._DEMO_DATA.get(bucket, [])
        prefix = (prefix or "").lstrip("/")
        count = 0
        for key, size, mtime in manifest:
            if key.startswith(prefix):
                count += 1
                yield S3Object(key=key, size=size, last_modified=mtime, is_folder=False)
                if progress_callback:
                    progress_callback(count)

    # ── Simulated transfers ────────────────────────────────────────────────────
    # progress_callback conventions match the real S3Client so the transfer
    # worker is unchanged: single-file cb(bytes_now, cumulative);
    # folder cb(name, total, done).

    @staticmethod
    def _simulate(total: int, progress_callback):
        """Feed smooth fake progress to the callback."""
        import time
        total = max(int(total), 1)
        chunk = max(total // 20, 1)
        done = 0
        while done < total:
            step = min(chunk, total - done)
            done += step
            if progress_callback:
                progress_callback(step, done)
            time.sleep(0.02)

    def download_file(self, bucket, key, local_path, overwrite=True, progress_callback=None, **kwargs):
        import os
        from s3_client import S3Error
        size = self._lookup_size(bucket, key)
        if size is None:
            raise S3Error(f"Demo: object not found: s3://{bucket}/{key}")
        os.makedirs(os.path.dirname(local_path) or ".", exist_ok=True)
        # Write a real (small) placeholder file so it appears in the local panel.
        # Cap actual bytes to keep the demo light; progress still reports full size.
        cap = min(size, 256 * 1024)
        chunk_size = 16_384
        written = 0
        with open(local_path, "wb") as f:
            while written < cap:
                f.write(b"\0" * min(chunk_size, cap - written))
                written += chunk_size
        self._simulate(size, progress_callback)
        return os.path.abspath(local_path)

    def download_prefix(self, bucket, prefix, local_dir, progress_callback=None, **kwargs):
        import os
        os.makedirs(local_dir, exist_ok=True)
        downloaded = []
        prefix = (prefix or "").lstrip("/")
        for obj in self.list_all_objects(bucket, prefix):
            rel = obj.key[len(prefix):]
            local_path = os.path.join(local_dir, rel)
            os.makedirs(os.path.dirname(local_path) or ".", exist_ok=True)
            # placeholder file
            with open(local_path, "wb") as f:
                f.write(b"demo placeholder\n")
            if progress_callback:
                progress_callback(obj.key, obj.size or 0, obj.size or 0)
            downloaded.append(local_path)
        return downloaded

    def upload_file(self, local_path, bucket, key, overwrite=True, progress_callback=None, **kwargs):
        import os
        from s3_client import S3Error
        if not os.path.isfile(local_path):
            raise S3Error(f"Demo: not a file: {local_path}")
        size = os.path.getsize(local_path)
        self._simulate(size, progress_callback)
        return f"s3://{bucket}/{key}"

    def upload_directory(self, local_dir, bucket, prefix="", progress_callback=None, **kwargs):
        import os
        prefix = (prefix or "").lstrip("/")
        uploaded = []
        for root, _dirs, files in os.walk(local_dir):
            for filename in files:
                local_path = os.path.join(root, filename)
                rel = os.path.relpath(local_path, local_dir)
                key = f"{prefix}/{rel}".lstrip("/")
                size = os.path.getsize(local_path)
                self._simulate(size, None)
                if progress_callback:
                    progress_callback(local_path, size, size)
                uploaded.append(f"s3://{bucket}/{key}")
        return uploaded

    # ── Helpers ─────────────────────────────────────────────────────────────────

    def _lookup_size(self, bucket, key):
        for k, size, _mtime in self._DEMO_DATA.get(bucket, []):
            if k == key:
                return size
        return None


class AuthDialog(QDialog):
    authenticated = pyqtSignal(object)  # S3Client

    def __init__(self, parent=None):
        super().__init__(parent)
        self.setWindowTitle("AWS S3 Explorer — Connect")
        self.setModal(True)
        self.setMinimumWidth(400)
        self._worker: _ValidationWorker | None = None

        self._build_ui()
        self._load_profiles()

    def _build_ui(self):
        layout = QVBoxLayout(self)

        if AWS_AVAILABLE:
            profile_row = QHBoxLayout()
            profile_row.addWidget(QLabel("AWS Profile:"))
            self.profile_combo = QComboBox()
            self.profile_combo.setMinimumWidth(220)
            self.profile_combo.currentIndexChanged.connect(self._on_profile_changed)
            profile_row.addWidget(self.profile_combo)
            self.btn_refresh = QPushButton("↻")
            self.btn_refresh.setFixedWidth(32)
            self.btn_refresh.setToolTip("Refresh profile list")
            self.btn_refresh.clicked.connect(self._load_profiles)
            profile_row.addWidget(self.btn_refresh)
            profile_row.addStretch()
            layout.addLayout(profile_row)
        else:
            self.profile_combo = None
            no_aws = QLabel("⚠️  boto3 not installed — demo mode only")
            no_aws.setStyleSheet("color: #c62828; font-size: 13px; font-weight: bold;")
            layout.addWidget(no_aws)

        self.status_label = QLabel("Select a profile to connect.")
        self.status_label.setWordWrap(True)
        self.status_label.setStyleSheet("color: #555; font-size: 12px;")
        layout.addWidget(self.status_label)

        btn_row = QHBoxLayout()
        btn_row.addStretch()

        if not AWS_AVAILABLE or get_available_profiles():
            self.btn_connect = QPushButton("Connect")
            self.btn_connect.setDefault(True)
            self.btn_connect.clicked.connect(self._on_connect)
            btn_row.addWidget(self.btn_connect)

        self.btn_demo = QPushButton("🧪 Demo Mode")
        self.btn_demo.setToolTip("Browse dummy S3 data without AWS credentials")
        self.btn_demo.clicked.connect(self._on_demo)
        btn_row.addWidget(self.btn_demo)

        self.btn_cancel = QPushButton("Cancel")
        self.btn_cancel.clicked.connect(self.reject)
        btn_row.addWidget(self.btn_cancel)
        layout.addLayout(btn_row)

    def _load_profiles(self):
        if self.profile_combo is None:
            return
        profiles = get_available_profiles()
        self.profile_combo.clear()
        if not profiles:
            self.profile_combo.addItem("(No profiles found)", userData=None)
            if self.btn_connect:
                self.btn_connect.setEnabled(False)
        else:
            self.profile_combo.addItems(profiles)
            if self.btn_connect:
                self.btn_connect.setEnabled(True)

    def _on_profile_changed(self, _):
        self.status_label.setText("Select a profile to connect.")
        self.status_label.setStyleSheet("color: #555; font-size: 12px;")

    def _on_connect(self):
        if not AWS_AVAILABLE:
            return
        profile = self.profile_combo.currentData()
        if profile is None:
            profile = self.profile_combo.currentText()

        if not profile or profile.startswith("(No profiles"):
            QMessageBox.warning(self, "No Profile", "Please select a valid AWS profile.")
            return

        self.btn_connect.setEnabled(False)
        self.btn_connect.setText("Connecting…")
        self.status_label.setText(f"Authenticating with profile '{profile}'…")
        self.status_label.setStyleSheet("color: #1565c0; font-size: 12px;")
        self.setCursor(Qt.CursorShape.BusyCursor)

        self._worker = _ValidationWorker(profile)
        self._worker.done.connect(self._on_validation_done)
        self._worker.start()

    def _on_validation_done(self, ok: bool, message: str):
        self.setCursor(Qt.CursorShape.ArrowCursor)
        self.btn_connect.setEnabled(True)
        self.btn_connect.setText("Connect")

        if ok:
            profile = self.profile_combo.currentData()
            if profile is None:
                profile = self.profile_combo.currentText()
            session = create_boto3_session(profile)
            if session:
                client = S3Client(session)
                self.authenticated.emit(client)
                self.accept()
            else:
                self.status_label.setText("Failed to create boto3 session.")
                self.status_label.setStyleSheet("color: #c62828; font-size: 12px;")
        else:
            self.status_label.setText(f"Authentication failed:\n{message}")
            self.status_label.setStyleSheet("color: #c62828; font-size: 12px;")

    def _on_demo(self):
        """Enter demo mode with stub data."""
        self.authenticated.emit(DemoS3Client())
        self.accept()
