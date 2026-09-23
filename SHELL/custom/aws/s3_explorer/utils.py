"""Shared utility helpers."""

from __future__ import annotations

import os
from datetime import datetime
from typing import Optional


def format_size(size_bytes: Optional[int]) -> str:
    """Human-readable file size (B / KB / MB / GB / TB)."""
    if size_bytes is None:
        return "—"
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if abs(size_bytes) < 1024:
            return f"{size_bytes:.1f} {unit}"
        size_bytes /= 1024
    return f"{size_bytes:.1f} PB"


def format_datetime(iso_str: Optional[str]) -> str:
    """Parse ISO datetime string and return a readable string."""
    if not iso_str:
        return "—"
    try:
        dt = datetime.fromisoformat(iso_str.replace("Z", "+00:00"))
        return dt.strftime("%Y-%m-%d %H:%M")
    except (ValueError, TypeError):
        return iso_str


def path_join(*parts: str) -> str:
    """Join path components with a forward slash, stripping extras."""
    return "/".join(p.strip("/") for p in parts if p.strip("/"))


def normalize_key(key: str) -> str:
    """Strip leading/trailing slashes from an S3 key."""
    return key.strip("/")


def is_folder(key: str) -> bool:
    """Return True if the S3 key looks like a folder (ends with /)."""
    return key.endswith("/")


def local_path_join(*parts: str) -> str:
    """Join local path components using os.sep."""
    return os.path.join(*parts)


def get_basename(path: str) -> str:
    """Return the last component of a path."""
    return os.path.basename(path.rstrip("/"))
