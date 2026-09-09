"""Drag & drop area widget."""
from PySide6.QtWidgets import QLabel
from PySide6.QtCore import Qt
from typing import Callable


class DropArea(QLabel):
    def __init__(self, on_files_dropped: Callable[[list[str]], None]):
        super().__init__("Drag & drop files here")
        self.setAlignment(Qt.AlignCenter)
        self.setAcceptDrops(True)
        self._callback = on_files_dropped

    def dragEnterEvent(self, event):
        if event.mimeData().hasUrls():
            event.acceptProposedAction()

    def dropEvent(self, event):
        paths = [url.toLocalFile() for url in event.mimeData().urls()]
        self._callback(paths)
