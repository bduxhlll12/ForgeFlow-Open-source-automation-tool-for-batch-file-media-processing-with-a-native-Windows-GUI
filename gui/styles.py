"""Qt stylesheet strings for dark/light themes."""

DARK_QSS = """
QMainWindow { background-color: #1e1e1e; color: #e0e0e0; }
QPushButton { background-color: #2d2d2d; border: 1px solid #3c3c3c; padding: 6px; }
QPushButton:hover { background-color: #3a3a3a; }
"""

LIGHT_QSS = """
QMainWindow { background-color: #fafafa; color: #202020; }
QPushButton { background-color: #eaeaea; border: 1px solid #cfcfcf; padding: 6px; }
"""


def stylesheet_for(theme: str) -> str:
    return DARK_QSS if theme == "dark" else LIGHT_QSS
