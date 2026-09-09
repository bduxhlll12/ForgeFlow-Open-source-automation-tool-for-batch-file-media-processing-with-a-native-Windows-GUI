"""Remove build artifacts and caches."""
import shutil
import os

TARGETS = ["build", "dist", "__pycache__", "logs"]


def clean() -> None:
    for target in TARGETS:
        if os.path.exists(target):
            shutil.rmtree(target)
            print(f"removed: {target}")


if __name__ == "__main__":
    clean()
