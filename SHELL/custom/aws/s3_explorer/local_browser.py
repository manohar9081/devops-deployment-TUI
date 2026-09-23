"""Local filesystem browser backed by Qt's QFileSystemModel.

Provides the right panel of the FileZilla-style layout.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

from PyQt6.QtCore import QDir, QModelIndex, Qt, pyqtSignal
from PyQt6.QtGui import QFileSystemModel
from PyQt6.QtWidgets import QAbstractItemView, QTreeView


class LocalTreeModel(QFileSystemModel):
    """QFileSystemModel with drag-enabled rows."""

    def flags(self, index: QModelIndex) -> Qt.ItemFlag:
        return (
            Qt.ItemFlag.ItemIsEnabled
            | Qt.ItemFlag.ItemIsSelectable
            | Qt.ItemFlag.ItemIsDragEnabled
            | Qt.ItemFlag.ItemIsDropEnabled
        )


class LocalTreeView(QTreeView):
    """Tree view for local filesystem navigation."""

    # Emitted when the user double-clicks a folder to navigate into it
    folder_entered = pyqtSignal(str)

    def __init__(self, root_path: Optional[str] = None, parent=None):
        super().__init__(parent)
        self._model = LocalTreeModel()

        # Columns: Name | Size | Type | Modified
        self._model.setRootPath(root_path or QDir.homePath())
        self.setModel(self._model)
        self.setRootIndex(self._model.index(root_path or QDir.homePath()))

        # Show all columns
        for col in range(1, self._model.columnCount()):
            self.hideColumn(col)

        # Appearance
        self.setAnimated(True)
        self.setIndentation(16)
        self.setSortingEnabled(True)
        self.sortByColumn(0, Qt.SortOrder.AscendingOrder)
        self.setSelectionMode(QAbstractItemView.SelectionMode.ExtendedSelection)
        self.setAcceptDrops(True)
        self.setDragEnabled(True)
        self.setDropIndicatorShown(True)
        self.setAcceptDrops(True)

        # Auto-expand single-child folders
        self.setAutoExpandDelay(400)

        # Double-click to enter folder
        self.doubleClicked.connect(self._on_double_click)

    def _on_double_click(self, index: QModelIndex):
        if self._model.isDir(index):
            path = self._model.filePath(index)
            self.setRootIndex(index)
            self.folder_entered.emit(path)

    def navigate_to(self, path: str):
        """Navigate the tree to the given directory path."""
        if os.path.isdir(path):
            idx = self._model.index(path)
            self.setRootIndex(idx)

    def navigate_up(self):
        """Go up one directory level."""
        current = self.rootIndex()
        parent = self._model.parent(current)
        if parent.isValid():
            self.setRootIndex(parent)

    def current_path(self) -> Optional[str]:
        """Return the full path of the currently selected item, or root."""
        idx = self.currentIndex()
        if idx.isValid():
            return self._model.filePath(idx)
        return self._model.rootPath()

    def root_path(self) -> str:
        return self._model.rootPath()

    def selected_paths(self) -> list[str]:
        """Return paths of all selected items."""
        return [self._model.filePath(i) for i in self.selectedIndexes()
                if i.column() == 0]

    def set_home(self):
        """Navigate to the user's home directory."""
        home = str(Path.home())
        self.setRootIndex(self._model.index(home))


class PathNavigator:
    """Simple breadcrumb / path bar helper for local browser.

    Not wired into the UI yet — placeholder for future address bar.
    """

    @staticmethod
    def split_path(path: str) -> list[str]:
        parts = []
        while True:
            head, tail = os.path.split(path)
            if tail:
                parts.insert(0, tail)
                path = head
            else:
                if head:
                    parts.insert(0, head)
                break
        return parts
