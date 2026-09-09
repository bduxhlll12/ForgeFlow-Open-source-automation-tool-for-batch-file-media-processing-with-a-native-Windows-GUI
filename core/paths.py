"""Central place for resolving app paths (works from source and from exe)."""
import os
import sys


def base_dir() -> str:
    if getattr(sys, "frozen", False):
        return os.path.dirname(sys.executable)
    return os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def resource_path(*parts: str) -> str:
    return os.path.join(base_dir(), "resources", *parts)


def preset_path(*parts: str) -> str:
    return os.path.join(base_dir(), "presets", *parts)
