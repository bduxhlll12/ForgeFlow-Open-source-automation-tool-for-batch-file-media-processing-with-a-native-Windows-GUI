"""Custom exceptions used across FlowForge."""


class FlowForgeError(Exception):
    """Base exception for all FlowForge errors."""


class PresetNotFoundError(FlowForgeError):
    def __init__(self, name: str):
        super().__init__(f"Preset '{name}' not found")


class PluginError(FlowForgeError):
    pass


class ProcessingError(FlowForgeError):
    def __init__(self, path: str, reason: str):
        super().__init__(f"Failed to process '{path}': {reason}")
