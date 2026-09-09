"""Settings dialog window."""
from PySide6.QtWidgets import QDialog, QFormLayout, QSpinBox, QComboBox, QDialogButtonBox


class SettingsDialog(QDialog):
    def __init__(self, settings):
        super().__init__()
        self.settings = settings
        self.setWindowTitle("Settings")

        layout = QFormLayout(self)
        self.workers_spin = QSpinBox()
        self.workers_spin.setRange(1, 32)
        self.workers_spin.setValue(settings.max_parallel_tasks)
        layout.addRow("Parallel tasks:", self.workers_spin)

        self.theme_box = QComboBox()
        self.theme_box.addItems(["dark", "light"])
        self.theme_box.setCurrentText(settings.theme)
        layout.addRow("Theme:", self.theme_box)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel)
        buttons.accepted.connect(self.accept)
        buttons.rejected.connect(self.reject)
        layout.addRow(buttons)

    def apply(self) -> None:
        self.settings.max_parallel_tasks = self.workers_spin.value()
        self.settings.theme = self.theme_box.currentText()
        self.settings.save()
