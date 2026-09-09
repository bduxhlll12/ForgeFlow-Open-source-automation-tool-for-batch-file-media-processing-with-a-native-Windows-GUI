"""System tray icon with minimize-to-tray support."""
from PySide6.QtWidgets import QSystemTrayIcon, QMenu
from PySide6.QtGui import QIcon


class TrayIcon(QSystemTrayIcon):
    def __init__(self, window):
        super().__init__(QIcon())
        self.window = window
        menu = QMenu()
        menu.addAction("Show", window.showNormal)
        menu.addAction("Quit", window.close)
        self.setContextMenu(menu)
        self.setToolTip("FlowForge")
