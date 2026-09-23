#!/usr/bin/env python3
"""S3 Explorer — FileZilla-style cross-platform GUI for AWS S3."""

import sys
import os

# Allow running as both `python main.py` and `python -m s3_explorer.main`
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))


def main():
    from PyQt6.QtWidgets import QApplication

    app = QApplication(sys.argv)
    app.setApplicationName("S3 Explorer")
    app.setOrganizationName("S3Explorer")
    app.setStyle("Fusion")

    from main_window import MainWindow

    window = MainWindow()
    window.show()

    sys.exit(app.exec())


if __name__ == "__main__":
    main()
