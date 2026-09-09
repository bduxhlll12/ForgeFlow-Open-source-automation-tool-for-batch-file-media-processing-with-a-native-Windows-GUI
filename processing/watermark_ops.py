"""Apply a watermark image onto videos or images."""
import subprocess
from PIL import Image


def watermark_image(input_path: str, watermark_path: str, output_path: str) -> None:
    base = Image.open(input_path).convert("RGBA")
    mark = Image.open(watermark_path).convert("RGBA")
    base.alpha_composite(mark, (base.width - mark.width - 10, base.height - mark.height - 10))
    base.save(output_path)


def watermark_video(input_path: str, watermark_path: str, output_path: str) -> None:
    cmd = [
        "ffmpeg", "-y", "-i", input_path, "-i", watermark_path,
        "-filter_complex", "overlay=W-w-10:H-h-10", output_path,
    ]
    subprocess.run(cmd, check=True)
