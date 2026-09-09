"""Input validation helpers."""
import os

SUPPORTED_VIDEO = {".mp4", ".mov", ".mkv", ".avi"}
SUPPORTED_IMAGE = {".png", ".jpg", ".jpeg", ".webp", ".bmp"}


def is_video(path: str) -> bool:
    return os.path.splitext(path)[1].lower() in SUPPORTED_VIDEO


def is_image(path: str) -> bool:
    return os.path.splitext(path)[1].lower() in SUPPORTED_IMAGE


def ensure_writable_dir(path: str) -> None:
    os.makedirs(path, exist_ok=True)
    test_file = os.path.join(path, ".write_test")
    with open(test_file, "w") as f:
        f.write("ok")
    os.remove(test_file)
