"""Image batch processing helpers built on Pillow."""
from PIL import Image


def resize(input_path: str, output_path: str, width: int, height: int) -> None:
    with Image.open(input_path) as img:
        img.resize((width, height)).save(output_path)


def convert_format(input_path: str, output_path: str) -> None:
    with Image.open(input_path) as img:
        img.save(output_path)
