# S3 Explorer — Project Agents.md

A FileZilla-style cross-platform GUI for browsing AWS S3 buckets and local filesystems with drag-and-drop transfer support.

## Project Overview

**Type**: Cross-platform desktop GUI application (PyQt6 + boto3)
**Goal**: Dual-panel file browser — left: AWS S3, right: local filesystem. Drag-and-drop transfer between panels.
**Users**: DevOps engineers and developers managing S3 assets

## Tech Stack

- **GUI Framework**: PyQt6 (cross-platform, native look on Mac/Windows/Linux)
- **AWS SDK**: boto3 (consistent with existing shell scripts)
- **Concurrency**: QThread + signals (Qt-native, no extra deps)
- **Monitoring**: watchdog (optional, for local dir changes)
- **Python**: 3.9+

## Architecture

```
s3_explorer/
├── main.py                 # Entry point, app init
├── s3_client.py            # boto3 S3 operations (list, download, upload, sync)
├── s3_auth.py              # AWS SSO / profile detection and management
├── local_browser.py        # Local filesystem navigation (Qt file system model)
├── transfer_manager.py     # Download/upload queue with progress tracking
├── main_window.py          # PyQt6 main window (dual-panel layout)
├── widgets/
│   ├── s3_tree.py          # S3 bucket/object tree view widget
│   ├── transfer_queue.py   # Transfer queue panel widget
│   └── s3_auth_dialog.py   # AWS profile selector / login dialog
├── utils.py                # File size formatting, path helpers
└── requirements.txt
```

## Key Design Decisions

1. **AWS Auth**: Leverage existing `~/.aws/config` profiles. On startup, detect available profiles and let user pick one. Use `botocore.sso_token_provider` flow. If no credentials, show auth dialog.

2. **S3 Browser**: Lazy-load bucket contents on expand. Show buckets at root, objects grouped by prefix (folder simulation). Single-click to navigate, double-click folders to enter.

3. **Local Browser**: Use `QFileSystemModel` for the right panel. Navigate like a normal file explorer.

4. **Drag & Drop**:
   - Drag from S3 panel → Local panel = **download**
   - Drag from Local panel → S3 panel = **upload**
   - Implement as `QDrag` + `QMimeData` with custom URI format

5. **Transfer Queue**: Background thread pool (max 3 concurrent transfers). Progress bar per item + overall. Cancel support. Retry on failure.

6. **Consistency with existing scripts**: Use the same AWS profile/config approach as the bash scripts. Mirror `aws s3 cp --recursive` behavior.

## Running

```bash
pip install -r requirements.txt
python main.py
# or
python -m s3_explorer.main
```

## Profiles & Auth

- Reads profiles from `~/.aws/config` and `~/.aws/credentials`
- If using AWS SSO, botocore handles token refresh automatically
- Profile selector shown on startup if multiple profiles exist
