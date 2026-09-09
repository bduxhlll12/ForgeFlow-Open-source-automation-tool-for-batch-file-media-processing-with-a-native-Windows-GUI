"""Application settings, persisted as JSON."""
import json
import os
from dataclasses import dataclass, asdict, field

SETTINGS_PATH = "settings.json"


@dataclass
class Settings:
    theme: str = "dark"
    max_parallel_tasks: int = 4
    last_output_dir: str = ""
    recent_presets: list = field(default_factory=list)

    @classmethod
    def load(cls) -> "Settings":
        if os.path.exists(SETTINGS_PATH):
            with open(SETTINGS_PATH, "r", encoding="utf-8") as f:
                return cls(**json.load(f))
        return cls()

    def save(self) -> None:
        with open(SETTINGS_PATH, "w", encoding="utf-8") as f:
            json.dump(asdict(self), f, indent=2, ensure_ascii=False)
