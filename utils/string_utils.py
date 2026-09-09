"""String helpers for filenames and labels."""
import re

_INVALID = re.compile(r'[<>:"/\\|?*]')


def safe_filename(name: str) -> str:
    return _INVALID.sub("_", name).strip()


def truncate(text: str, max_len: int = 40) -> str:
    return text if len(text) <= max_len else text[: max_len - 1] + "…"
