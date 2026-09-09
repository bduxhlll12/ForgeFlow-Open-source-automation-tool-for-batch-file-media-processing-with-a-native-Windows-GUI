"""FlowForge entry point."""
import sys
from PySide6.QtWidgets import QApplication

from gui.main_window import MainWindow
from core.logger import get_logger
from core.settings import Settings

log = get_logger(__name__)


def main() -> int:
    settings = Settings.load()
    app = QApplication(sys.argv)
    window = MainWindow(settings)
    window.show()
    log.info("FlowForge started")
    return app.exec()


if __name__ == "__main__":
    sys.exit(main())
