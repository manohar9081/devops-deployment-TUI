"""AWS SSO / profile detection and session management.

Mirrors the approach used by the existing bash scripts:
  - Reads profiles from ~/.aws/config and ~/.aws/credentials
  - Uses botocore's built-in SSO token provider for session refresh
  - Validates credentials before use
"""

from __future__ import annotations

import configparser
import os
import webbrowser
from pathlib import Path
from typing import Optional

import boto3
from botocore import sso_token_provider
from botocore.exceptions import BotoCoreError, ClientError, SSOTokenLoadError


def aws_config_path() -> Path:
    return Path.home() / ".aws"


def get_available_profiles() -> list[str]:
    """Return all AWS profile names found in ~/.aws/config."""
    profiles: list[str] = []
    config_path = aws_config_path() / "config"
    if config_path.exists():
        cfg = configparser.ConfigParser()
        cfg.read(config_path)
        profiles = [
            sec.removeprefix("profile ")
            for sec in cfg.sections()
            if sec.startswith("profile ")
        ]
    # Also check credentials file
    creds_path = aws_config_path() / "credentials"
    if creds_path.exists():
        cfg = configparser.ConfigParser()
        cfg.read(creds_path)
        for sec in cfg.sections():
            if sec not in profiles:
                profiles.append(sec)
    return sorted(set(profiles))


def _has_sso_config(profile: str) -> bool:
    """Check if a profile uses AWS SSO."""
    config_path = aws_config_path() / "config"
    if not config_path.exists():
        return False
    cfg = configparser.ConfigParser()
    cfg.read(config_path)
    full_section = f"profile {profile}" if not profile.startswith("profile ") else profile
    return cfg.has_section(full_section) and cfg.has_option(full_section, "sso_session")


def _ensure_sso_token(profile: str) -> bool:
    """Attempt to load / refresh SSO token for the profile.

    Returns True if a valid token is available or was just obtained.
    Returns False if SSO is required but can't be completed (e.g., browser
    not available, user cancelled).
    """
    if not _has_sso_config(profile):
        return True  # Not an SSO profile, no token needed

    config_path = aws_config_path() / "config"
    cfg = configparser.ConfigParser()
    cfg.read(config_path)
    full_section = f"profile {profile}"

    if not cfg.has_section(full_section):
        return False

    sso_session = cfg.get(full_section, "sso_session", fallback="")
    sso_start_url = None
    sso_region = None

    for sec in cfg.sections():
        if sec == f"sso-session {sso_session}":
            sso_start_url = cfg.get(sec, "sso_start_url", fallback=None)
            sso_region = cfg.get(sec, "sso_region", fallback=None)
            break

    if not sso_start_url or not sso_region:
        return False

    # Try to load existing token first
    try:
        token = sso_token_provider.load_token(
            start_url=sso_start_url,
            sso_region=sso_region,
            sso_token_cache=None,
        )
        if token:
            return True
    except SSOTokenLoadError:
        pass

    # Token expired or missing — open browser for SSO login
    print(f"[S3 Explorer] SSO token missing or expired for profile '{profile}'.")
    print(f"[S3 Explorer] Opening browser for AWS SSO login...")
    webbrowser.open(sso_start_url)

    try:
        token = sso_token_provider.prompt_for_token(
            start_url=sso_start_url,
            sso_region=sso_region,
            sso_token_cache=None,
        )
        return token is not None
    except Exception as e:
        print(f"[S3 Explorer] SSO login failed: {e}")
        return False


def create_boto3_session(profile: str) -> Optional[boto3.Session]:
    """Create an authenticated boto3 session for the given profile.

    Handles SSO token acquisition automatically.
    Returns None if credentials cannot be obtained.
    """
    if not _ensure_sso_token(profile):
        return None

    try:
        session = boto3.Session(profile_name=profile)
        # Verify credentials are valid by calling STS caller identity
        sts = session.client("sts")
        sts.get_caller_identity()
        return session
    except (BotoCoreError, ClientError) as e:
        print(f"[S3 Explorer] Failed to initialize session for '{profile}': {e}")
        return None


def validate_credentials(profile: str) -> tuple[bool, str]:
    """Return (ok, message) after validating the profile's credentials."""
    if not _ensure_sso_token(profile):
        return False, "AWS SSO login failed or was cancelled."

    try:
        session = boto3.Session(profile_name=profile)
        sts = session.client("sts")
        identity = sts.get_caller_identity()
        return True, f"Authenticated as: {identity['Arn']}"
    except ClientError as e:
        return False, f"Credential error: {e.response['Error']['Message']}"
    except BotoCoreError as e:
        return False, f"BotoCore error: {e}"
