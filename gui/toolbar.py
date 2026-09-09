"""Top toolbar with common actions."""
from PySide6.QtWidgets import QToolBar
from PySide6.QtGui import QAction
from typing import Callable


class MainToolbar(QToolBar):
    def __init__(self, on_add_files: Callable, on_start: Callable, on_clear: Callable):
        super().__init__("Main")
        self.addAction(self._make_action("Add files", on_add_files))
        self.addAction(self._make_action("Start", on_start))
        self.addAction(self._make_action("Clear", on_clear))

    def _make_action(self, label: str, handler: Callable) -> QAction:
        action = QAction(label, self)
        action.triggered.connect(handler)
        return action
