"""Audio extraction/processing built on ffmpeg."""
import subprocess


def extract_audio(input_path: str, output_path: str) -> None:
    cmd = ["ffmpeg", "-y", "-i", input_path, "-vn", "-acodec", "copy", output_path]
    subprocess.run(cmd, check=True)


def normalize_volume(input_path: str, output_path: str) -> None:
    cmd = ["ffmpeg", "-y", "-i", input_path, "-af", "loudnorm", output_path]
    subprocess.run(cmd, check=True)
