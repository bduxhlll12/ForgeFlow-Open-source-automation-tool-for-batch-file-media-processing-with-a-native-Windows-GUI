"""Chains multiple processing steps into a single pipeline run."""
from typing import Callable


class Pipeline:
    def __init__(self):
        self._steps: list[Callable[[str], str]] = []

    def add_step(self, func: Callable[[str], str]) -> "Pipeline":
        self._steps.append(func)
        return self

    def run(self, input_path: str) -> str:
        current = input_path
        for step in self._steps:
            current = step(current)
        return current
