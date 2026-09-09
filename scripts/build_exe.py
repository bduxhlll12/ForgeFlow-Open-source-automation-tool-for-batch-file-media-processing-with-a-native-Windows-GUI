"""Build a portable .exe using PyInstaller."""
import subprocess


def build() -> None:
    cmd = [
        "pyinstaller", "--noconfirm", "--onefile", "--windowed",
        "--name", "FlowForge", "main.py",
    ]
    subprocess.run(cmd, check=True)


if __name__ == "__main__":
    build()
