"""Main application window."""
from PySide6.QtWidgets import QMainWindow, QWidget, QVBoxLayout

from gui.drag_drop import DropArea
from gui.progress_widget import ProgressPanel
from core.task_queue import TaskQueue


class MainWindow(QMainWindow):
    def __init__(self, settings):
        super().__init__()
        self.settings = settings
        self.queue = TaskQueue(max_workers=settings.max_parallel_tasks)

        self.setWindowTitle("FlowForge")
        self.resize(900, 600)

        central = QWidget()
        layout = QVBoxLayout(central)
        self.drop_area = DropArea(on_files_dropped=self.handle_files)
        self.progress_panel = ProgressPanel()
        layout.addWidget(self.drop_area)
        layout.addWidget(self.progress_panel)
        self.setCentralWidget(central)

    def handle_files(self, paths: list[str]) -> None:
        for path in paths:
            self.progress_panel.add_item(path)
