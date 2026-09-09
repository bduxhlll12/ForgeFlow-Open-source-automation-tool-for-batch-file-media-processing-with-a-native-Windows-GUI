"""Minimal plugin API — extend FlowForge with custom actions."""
from abc import ABC, abstractmethod
from typing import Any


class FlowForgePlugin(ABC):
    """Base class every plugin must implement."""

    name: str = "unnamed_plugin"

    @abstractmethod
    def run(self, input_path: str, output_path: str, **options: Any) -> None:
        """Execute the plugin's processing logic on a single file."""
        raise NotImplementedError

    def describe(self) -> str:
        return f"Plugin: {self.name}"


_REGISTRY: dict[str, FlowForgePlugin] = {}


def register(plugin: FlowForgePlugin) -> None:
    _REGISTRY[plugin.name] = plugin


def get_plugin(name: str) -> FlowForgePlugin:
    return _REGISTRY[name]
