"""Video processing helpers built on ffmpeg."""
import subprocess


def transcode(input_path: str, output_path: str, codec: str = "libx264") -> None:
    cmd = ["ffmpeg", "-y", "-i", input_path, "-c:v", codec, output_path]
    subprocess.run(cmd, check=True)


def extract_frame(input_path: str, output_path: str, timestamp: str = "00:00:01") -> None:
    cmd = ["ffmpeg", "-y", "-ss", timestamp, "-i", input_path, "-frames:v", "1", output_path]
    subprocess.run(cmd, check=True)
