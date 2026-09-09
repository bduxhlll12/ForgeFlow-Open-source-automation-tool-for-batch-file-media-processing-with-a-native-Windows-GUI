"""Time formatting helpers."""
import time


def format_duration(seconds: float) -> str:
    m, s = divmod(int(seconds), 60)
    h, m = divmod(m, 60)
    return f"{h:02d}:{m:02d}:{s:02d}"


def timestamp() -> str:
    return time.strftime("%Y-%m-%d_%H-%M-%S")
