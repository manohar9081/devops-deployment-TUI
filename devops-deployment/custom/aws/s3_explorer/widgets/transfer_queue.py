"""Transfer queue panel widget — bottom panel of the main window."""

from __future__ import annotations

import os
import sys
from pathlib import Path
from PyQt6.QtCore import Qt, QSize

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from PyQt6.QtGui import QColor
from PyQt6.QtWidgets import (
    QAbstractItemView,
    QHeaderView,
    QMenu,
    QProgressBar,
    QStyle,
    QTableWidget,
    QTableWidgetItem,
    QToolButton,
    QVBoxLayout,
    QWidget,
)

from transfer_manager import TransferItem, TransferManager, TransferStatus, TransferType
from utils import format_size


class TransferQueueWidget(QWidget):
    """Bottom panel showing all queued transfers with progress bars."""

    def __init__(self, manager: TransferManager, parent=None):
        super().__init__(parent)
        self._manager = manager

        # Connect manager signals
        manager.item_added.connect(self._on_item_added)
        manager.item_updated.connect(self._on_item_updated)
        manager.item_removed.connect(self._on_item_removed)
        manager.queue_cleared.connect(self._on_queue_cleared)

        # Toolbar
        from PyQt6.QtWidgets import QHBoxLayout
        toolbar = QHBoxLayout()
        self._btn_clear_finished = QToolButton()
        self._btn_clear_finished.setText("Clear Finished")
        self._btn_clear_finished.clicked.connect(manager.clear_finished)
        self._btn_cancel = QToolButton()
        self._btn_cancel.setText("Cancel Selected")
        self._btn_cancel.clicked.connect(self._cancel_selected)
        self._btn_retry = QToolButton()
        self._btn_retry.setText("Retry Selected")
        self._btn_retry.clicked.connect(self._retry_selected)
        toolbar.addWidget(self._btn_clear_finished)
        toolbar.addWidget(self._btn_cancel)
        toolbar.addWidget(self._btn_retry)
        toolbar.addStretch()

        # Table
        self.table = QTableWidget()
        self.table.setColumnCount(5)
        self.table.setHorizontalHeaderLabels(["Direction", "Source", "Destination", "Progress", "Status"])
        self.table.horizontalHeader().setStretchLastSection(True)
        self.table.horizontalHeader().setSectionResizeMode(1, QHeaderView.ResizeMode.Interactive)
        self.table.horizontalHeader().setSectionResizeMode(0, QHeaderView.ResizeMode.ResizeToContents)
        self.table.verticalHeader().setVisible(False)
        self.table.setSelectionBehavior(QAbstractItemView.SelectionBehavior.SelectRows)
        self.table.setEditTriggers(QAbstractItemView.EditTrigger.NoEditTriggers)
        self.table.setShowGrid(False)
        self.table.setAlternatingRowColors(True)
        self.table.setContextMenuPolicy(Qt.ContextMenuPolicy.CustomContextMenu)
        self.table.customContextMenuRequested.connect(self._show_context_menu)

        # Layout
        layout = QVBoxLayout(self)
        layout.setContentsMargins(4, 4, 4, 4)
        layout.addLayout(toolbar)
        layout.addWidget(self.table)

    def _id_to_row(self, item_id: str) -> int:
        for row in range(self.table.rowCount()):
            if self.table.item(row, 0).data(Qt.ItemDataRole.UserRole) == item_id:
                return row
        return -1

    def _on_item_added(self, item: TransferItem):
        self._add_row(item)

    def _on_item_updated(self, item: TransferItem):
        row = self._id_to_row(item.id)
        if row < 0:
            return

        # Status
        status_item = self.table.item(row, 4)
        if status_item:
            status_item.setText(self._status_label(item.status, item.error_message))
            status_item.setForeground(self._status_color(item.status))

        # Progress bar
        progress_widget = self.table.cellWidget(row, 3)
        if isinstance(progress_widget, QProgressBar):
            if item.size_bytes > 0:
                progress_widget.setMaximum(item.size_bytes)
                progress_widget.setValue(item.bytes_done)
            else:
                progress_widget.setMaximum(0)
                progress_widget.setValue(0)

    def _on_item_removed(self, item_id: str):
        row = self._id_to_row(item_id)
        if row >= 0:
            self.table.removeRow(row)

    def _on_queue_cleared(self):
        self.table.setRowCount(0)

    def _add_row(self, item: TransferItem):
        row = self.table.rowCount()
        self.table.insertRow(row)

        # Direction
        icon = "⬇" if item.type == TransferType.DOWNLOAD else "⬆"
        dir_item = QTableWidgetItem(f"{icon} {item.type.value.capitalize()}")
        dir_item.setData(Qt.ItemDataRole.UserRole, item.id)
        dir_item.setToolTip(item.type.value)
        self.table.setItem(row, 0, dir_item)

        # Source
        if item.type == TransferType.DOWNLOAD:
            src = f"s3://{item.bucket}/{item.key}"
        else:
            src = item.local_path
        src_item = QTableWidgetItem(src)
        src_item.setToolTip(src)
        self.table.setItem(row, 1, src_item)

        # Destination
        if item.type == TransferType.DOWNLOAD:
            dst = item.local_path
        else:
            dst = f"s3://{item.bucket}/{item.key}"
        dst_item = QTableWidgetItem(dst)
        dst_item.setToolTip(dst)
        self.table.setItem(row, 2, dst_item)

        # Progress bar
        progress = QProgressBar()
        progress.setMaximum(100 if item.size_bytes > 0 else 0)
        progress.setValue(0)
        progress.setTextVisible(True)
        progress.setFixedHeight(16)
        if item.size_bytes > 0:
            progress.setFormat(f"%p% ({format_size(item.size_bytes)})")
        self.table.setCellWidget(row, 3, progress)

        # Status
        status_item = QTableWidgetItem(self._status_label(item.status, item.error_message))
        status_item.setForeground(self._status_color(item.status))
        self.table.setItem(row, 4, status_item)

    @staticmethod
    def _status_label(status: TransferStatus, error: str = "") -> str:
        if status == TransferStatus.PENDING:
            return "Pending"
        elif status == TransferStatus.IN_PROGRESS:
            return "In Progress"
        elif status == TransferStatus.DONE:
            return "Done ✓"
        elif status == TransferStatus.FAILED:
            return f"Failed: {error[:60]}"
        elif status == TransferStatus.CANCELLED:
            return "Cancelled"
        return str(status)

    @staticmethod
    def _status_color(status: TransferStatus) -> QColor:
        if status == TransferStatus.DONE:
            return QColor("#2e7d32")
        elif status == TransferStatus.FAILED:
            return QColor("#c62828")
        elif status == TransferStatus.CANCELLED:
            return QColor("#757575")
        elif status == TransferStatus.IN_PROGRESS:
            return QColor("#1565c0")
        return QColor("#424242")

    @staticmethod
    def _item_size_str(item: TransferItem) -> str:
        from utils import format_size
        return format_size(item.size_bytes)

    def _show_context_menu(self, pos):
        menu = QMenu(self)
        cancel_action = menu.addAction("Cancel")
        retry_action = menu.addAction("Retry")
        clear_action = menu.addAction("Clear Finished")
        menu.addSeparator()
        remove_action = menu.addAction("Remove from Queue")
        action = menu.exec(self.mapToGlobal(pos))
        if action == cancel_action:
            self._cancel_selected()
        elif action == retry_action:
            self._retry_selected()
        elif action == clear_action:
            self._manager.clear_finished()
        elif action == remove_action:
            self._remove_selected()

    def _cancel_selected(self):
        for idx in self.table.selectionModel().selectedRows():
            row = idx.row()
            item_id = self.table.item(row, 0).data(Qt.ItemDataRole.UserRole)
            self._manager.cancel(item_id)

    def _retry_selected(self):
        for idx in self.table.selectionModel().selectedRows():
            row = idx.row()
            item_id = self.table.item(row, 0).data(Qt.ItemDataRole.UserRole)
            self._manager.retry(item_id)

    def _remove_selected(self):
        for idx in reversed(self.table.selectionModel().selectedRows()):
            row = idx.row()
            item_id = self.table.item(row, 0).data(Qt.ItemDataRole.UserRole)
            self._manager.remove(item_id)
