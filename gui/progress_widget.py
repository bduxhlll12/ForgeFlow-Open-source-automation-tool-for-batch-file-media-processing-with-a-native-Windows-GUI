"""Real-time progress tracking panel."""
from PySide6.QtWidgets import QListWidget, QListWidgetItem


class ProgressPanel(QListWidget):
    def add_item(self, label: str) -> QListWidgetItem:
        item = QListWidgetItem(f"[queued] {label}")
        self.addItem(item)
        return item

    def set_progress(self, item: QListWidgetItem, percent: int) -> None:
        base = item.text().split("]", 1)[1].strip()
        item.setText(f"[{percent}%] {base}")
