"""Generic batch file operations."""
import os
import shutil


def batch_copy(paths: list[str], dest_dir: str) -> list[str]:
    os.makedirs(dest_dir, exist_ok=True)
    results = []
    for path in paths:
        target = os.path.join(dest_dir, os.path.basename(path))
        shutil.copy2(path, target)
        results.append(target)
    return results


def batch_rename(paths: list[str], pattern: str) -> list[str]:
    results = []
    for i, path in enumerate(paths, start=1):
        directory = os.path.dirname(path)
        ext = os.path.splitext(path)[1]
        new_name = pattern.format(index=i, ext=ext)
        new_path = os.path.join(directory, new_name)
        os.rename(path, new_path)
        results.append(new_path)
    return results
