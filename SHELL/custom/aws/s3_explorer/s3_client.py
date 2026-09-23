"""S3 operations — list buckets/objects, download, upload.

Mirrors the logic in the existing bash scripts but exposes it as Python objects
so the PyQt6 GUI can call it from background threads.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from datetime import datetime
from typing import Callable, Generator, Optional

import boto3
from botocore.exceptions import BotoCoreError, ClientError

from utils import format_size, is_folder, normalize_key


@dataclass
class S3Object:
    """Represents an S3 object or folder (prefix) within a bucket."""
    key: str
    size: Optional[int]
    last_modified: Optional[str]
    is_folder: bool
    storage_class: Optional[str] = None

    @property
    def name(self) -> str:
        """Display name — last path component."""
        return os.path.basename(self.key.rstrip("/")) or self.key

    @property
    def size_str(self) -> str:
        return format_size(self.size)

    @property
    def last_modified_str(self) -> str:
        if not self.last_modified:
            return "—"
        try:
            dt = datetime.fromisoformat(self.last_modified.replace("Z", "+00:00"))
            return dt.strftime("%Y-%m-%d %H:%M")
        except (ValueError, TypeError):
            return str(self.last_modified)


@dataclass
class S3Bucket:
    """Represents an S3 bucket."""
    name: str
    creation_date: Optional[str] = None

    @property
    def name_str(self) -> str:
        return self.name


class S3Client:
    """Thread-safe(ish) S3 client backed by boto3.

    All network calls raise `S3Error` on failure.
    """

    def __init__(self, session: boto3.Session):
        self._session = session
        self._s3 = session.client("s3")
        self._resource = session.resource("s3")

    # ── Listing ────────────────────────────────────────────────────────────────

    def list_buckets(self) -> list[S3Bucket]:
        """Return all S3 buckets accessible with the current credentials."""
        try:
            response = self._s3.list_buckets()
            return [
                S3Bucket(
                    name=b["Name"],
                    creation_date=b.get("CreationDate"),
                )
                for b in response.get("Buckets", [])
            ]
        except (BotoCoreError, ClientError) as e:
            raise S3Error(f"Failed to list buckets: {e}") from e

    def list_objects(
        self,
        bucket: str,
        prefix: str = "",
        delimiter: str = "/",
        max_keys: int = 1000,
        continuation_token: Optional[str] = None,
    ) -> tuple[list[S3Object], Optional[str]]:
        """List objects and common prefixes (folders) under a prefix.

        Returns (items, next_continuation_token). Token is None when done.
        Folders are synthesized from CommonPrefixes.
        """
        kwargs: dict = {
            "Bucket": bucket,
            "Prefix": prefix,
            "Delimiter": delimiter,
            "MaxKeys": max_keys,
        }
        if continuation_token:
            kwargs["ContinuationToken"] = continuation_token

        try:
            response = self._s3.list_objects_v2(**kwargs)
        except (BotoCoreError, ClientError) as e:
            raise S3Error(f"Failed to list objects in s3://{bucket}/{prefix}: {e}") from e

        items: list[S3Object] = []

        # Objects
        for obj in response.get("Contents", []):
            key = normalize_key(obj["Key"])
            # Skip the prefix itself (root folder marker)
            if key == normalize_key(prefix):
                continue
            items.append(S3Object(
                key=key,
                size=obj.get("Size"),
                last_modified=obj.get("LastModified"),
                is_folder=is_folder(key),
                storage_class=obj.get("StorageClass"),
            ))

        # Folders (CommonPrefixes)
        for cp in response.get("CommonPrefixes", []):
            folder_prefix = normalize_key(cp["Prefix"])
            if folder_prefix == normalize_key(prefix):
                continue
            items.append(S3Object(
                key=folder_prefix + "/",
                size=None,
                last_modified=None,
                is_folder=True,
            ))

        # Sort: folders first, then files, both alphabetically
        items.sort(key=lambda x: (not x.is_folder, x.name.lower()))

        return items, response.get("NextContinuationToken")

    def list_all_objects(
        self,
        bucket: str,
        prefix: str = "",
        progress_callback: Optional[Callable[[int], None]] = None,
    ) -> Generator[S3Object, None, None]:
        """Yield all objects under a prefix, auto-paginating."""
        token = None
        total = 0
        while True:
            items, token = self.list_objects(bucket, prefix, continuation_token=token)
            for item in items:
                if not item.is_folder:
                    yield item
                    total += 1
                    if progress_callback:
                        progress_callback(total)
            if not token:
                break

    # ── Transfers ──────────────────────────────────────────────────────────────

    def download_file(
        self,
        bucket: str,
        key: str,
        local_path: str,
        overwrite: bool = True,
        progress_callback: Optional[Callable[[int, int], None]] = None,
    ) -> str:
        """Download a single S3 object to a local path.

        Creates parent directories if needed.
        Returns the absolute local path on success.
        Raises `S3Error` on failure.
        """
        if os.path.exists(local_path) and not overwrite:
            raise S3Error(f"Local file already exists: {local_path}")

        os.makedirs(os.path.dirname(local_path) or ".", exist_ok=True)

        try:
            self._s3.download_file(
                bucket,
                key,
                local_path,
                Callback=_BotoCallback(progress_callback) if progress_callback else None,
            )
        except (BotoCoreError, ClientError) as e:
            raise S3Error(f"Download failed: s3://{bucket}/{key} -> {local_path}: {e}") from e

        return os.path.abspath(local_path)

    def download_prefix(
        self,
        bucket: str,
        prefix: str,
        local_dir: str,
        progress_callback: Optional[Callable[[str, int, int], None]] = None,
    ) -> list[str]:
        """Download all objects under an S3 prefix to a local directory.

        Mirrors the S3 key structure inside local_dir.
        Returns list of downloaded local paths.
        """
        downloaded: list[str] = []
        for obj in self.list_all_objects(bucket, prefix):
            rel_key = obj.key[len(prefix):].lstrip("/")
            local_path = os.path.join(local_dir, rel_key)
            self.download_file(bucket, obj.key, local_path)
            downloaded.append(local_path)
            if progress_callback:
                progress_callback(obj.key, obj.size or 0, os.path.getsize(local_path))
        return downloaded

    def upload_file(
        self,
        local_path: str,
        bucket: str,
        key: str,
        overwrite: bool = True,
        progress_callback: Optional[Callable[[int, int], None]] = None,
    ) -> str:
        """Upload a local file to S3.

        Raises `S3Error` if the object already exists and overwrite=False.
        Returns the S3 URI on success.
        """
        if not os.path.isfile(local_path):
            raise S3Error(f"Not a file: {local_path}")

        if not overwrite:
            try:
                self._s3.head_object(Bucket=bucket, Key=key)
                raise S3Error(f"S3 object already exists: s3://{bucket}/{key}")
            except ClientError as e:
                if e.response["Error"]["Code"] != "404":
                    raise S3Error(f"Error checking S3 object: {e}") from e

        try:
            self._s3.upload_file(
                local_path,
                bucket,
                key,
                Callback=_BotoCallback(progress_callback) if progress_callback else None,
            )
        except (BotoCoreError, ClientError) as e:
            raise S3Error(f"Upload failed: {local_path} -> s3://{bucket}/{key}: {e}") from e

        return f"s3://{bucket}/{key}"

    def upload_directory(
        self,
        local_dir: str,
        bucket: str,
        prefix: str = "",
        progress_callback: Optional[Callable[[str, int, int], None]] = None,
    ) -> list[str]:
        """Upload a local directory tree to S3 under a key prefix.

        Mirrors the local file structure under the prefix.
        Returns list of uploaded S3 URIs.
        """
        uploaded: list[str] = []
        for root, _, files in os.walk(local_dir):
            for filename in files:
                local_path = os.path.join(root, filename)
                rel_path = os.path.relpath(local_path, local_dir)
                key = f"{prefix}/{rel_path}".lstrip("/")
                self.upload_file(local_path, bucket, key)
                uploaded.append(f"s3://{bucket}/{key}")
                if progress_callback:
                    progress_callback(local_path, os.path.getsize(local_path), os.path.getsize(local_path))
        return uploaded

    # ── Utility ─────────────────────────────────────────────────────────────────

    def head_object(self, bucket: str, key: str) -> S3Object:
        """Fetch metadata for a single object."""
        try:
            resp = self._s3.head_object(Bucket=bucket, Key=key)
            return S3Object(
                key=key,
                size=resp.get("ContentLength"),
                last_modified=resp.get("LastModified"),
                is_folder=is_folder(key),
                storage_class=resp.get("StorageClass"),
            )
        except (BotoCoreError, ClientError) as e:
            raise S3Error(f"head_object failed: s3://{bucket}/{key}: {e}") from e

    def object_exists(self, bucket: str, key: str) -> bool:
        """Return True if the object exists in S3."""
        try:
            self._s3.head_object(Bucket=bucket, Key=key)
            return True
        except ClientError as e:
            if e.response["Error"]["Code"] == "404":
                return False
            raise

    def get_bucket_region(self, bucket: str) -> str:
        """Return the region of a bucket (or the session's default region)."""
        try:
            region = self._s3.head_bucket(Bucket=bucket)["ResponseMetadata"]["HTTPHeaders"].get(
                "x-amz-bucket-region"
            )
            return region or self._session.region_name or "us-east-1"
        except (BotoCoreError, ClientError):
            return self._session.region_name or "us-east-1"


class S3Error(Exception):
    """Raised for any S3 operation failure."""
    pass


class _BotoCallback:
    """Adapt a progress_callback(bytes_transferred, total_bytes) to boto3's
    convention of calling callback(downloaded_bytes) on each chunk."""
    def __init__(self, callback: Callable[[int, int], None]):
        self._callback = callback
        self._total = 0

    def __call__(self, chunk: int):
        self._total += chunk
        self._callback(chunk, self._total)
