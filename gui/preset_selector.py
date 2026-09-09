"""Dropdown for choosing a preset."""
from PySide6.QtWidgets import QComboBox
from core.presets import list_presets


class PresetSelector(QComboBox):
    def __init__(self):
        super().__init__()
        self.refresh()

    def refresh(self) -> None:
        self.clear()
        self.addItems(list_presets() or ["default"])

    def selected(self) -> str:
        return self.currentText()
