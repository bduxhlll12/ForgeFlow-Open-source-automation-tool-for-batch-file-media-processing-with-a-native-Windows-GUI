"""Load/save preset-based workflows."""
import json
import os

PRESETS_DIR = "presets"


def list_presets() -> list[str]:
    if not os.path.isdir(PRESETS_DIR):
        return []
    return [f[:-5] for f in os.listdir(PRESETS_DIR) if f.endswith(".json")]


def load_preset(name: str) -> dict:
    path = os.path.join(PRESETS_DIR, f"{name}.json")
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_preset(name: str, data: dict) -> None:
    os.makedirs(PRESETS_DIR, exist_ok=True)
    path = os.path.join(PRESETS_DIR, f"{name}.json")
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
