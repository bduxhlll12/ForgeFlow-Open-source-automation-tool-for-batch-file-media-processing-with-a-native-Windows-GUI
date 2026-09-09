"""Batch archive (zip) operations."""
import zipfile
import os


def zip_files(paths: list[str], output_path: str) -> str:
    with zipfile.ZipFile(output_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for path in paths:
            zf.write(path, os.path.basename(path))
    return output_path


def unzip(archive_path: str, dest_dir: str) -> list[str]:
    os.makedirs(dest_dir, exist_ok=True)
    with zipfile.ZipFile(archive_path, "r") as zf:
        zf.extractall(dest_dir)
        return zf.namelist()
