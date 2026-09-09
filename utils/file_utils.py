"""Small filesystem helpers reused across modules."""
import os


def human_size(num_bytes: int) -> str:
    for unit in ["B", "KB", "MB", "GB", "TB"]:
        if num_bytes < 1024:
            return f"{num_bytes:.1f}{unit}"
        num_bytes /= 1024
    return f"{num_bytes:.1f}PB"


def list_files_recursive(root: str) -> list[str]:
    result = []
    for dirpath, _, filenames in os.walk(root):
        for name in filenames:
            result.append(os.path.join(dirpath, name))
    return result
