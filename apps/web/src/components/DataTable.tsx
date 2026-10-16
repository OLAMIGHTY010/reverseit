import React, { useState, useMemo, useRef } from 'react';
import {
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Columns3,
  Download,
  ChevronDown,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from './Button';
import { Pagination } from './Pagination';
import { useApp } from '../context/AppContext';

export interface ColumnDef<T> {
  id: string;
  header: string;
  accessorKey?: keyof T;
  cell?: (info: { row: T; value: unknown }) => React.ReactNode;
  enableSorting?: boolean;
  minWidth?: number;
  width?: number;
}

export interface BulkAction<T> {
  id: string;
  label: string;
  icon?: React.ReactNode;
  variant?: 'primary' | 'secondary' | 'danger';
  onClick: (selectedRows: T[]) => void;
  requiredRole?: string[];
}

export interface DataTableProps<T extends { id: string }> {
  data: T[];
  columns: ColumnDef<T>[];
  pageSize?: number;
  onRowClick?: (row: T) => void;
  bulkActions?: BulkAction<T>[];
  enableMultiSelect?: boolean;
  enableColumnChooser?: boolean;
  enableExport?: boolean;
  exportFilename?: string;
  emptyMessage?: string;
  className?: string;
}

export function DataTable<T extends { id: string }>({
  data,
  columns: initialColumns,
  pageSize = 10,
  onRowClick,
  bulkActions = [],
  enableMultiSelect = true,
  enableColumnChooser = true,
  enableExport = true,
  exportFilename = 'smartreversal-export.csv',
  emptyMessage = 'No records match current query or filter criteria.',
  className,
}: DataTableProps<T>) {
  const { density, role, addToast } = useApp();

  // Columns visibility state
  const [visibleColumnIds, setVisibleColumnIds] = useState<Set<string>>(
    () => new Set(initialColumns.map((c) => c.id))
  );
  const [columnChooserOpen, setColumnChooserOpen] = useState(false);

  // Sorting state
  const [sortColumnId, setSortColumnId] = useState<string | null>(null);
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');

  // Pagination state
  const [currentPage, setCurrentPage] = useState(1);
  const [currentPageSize, setCurrentPageSize] = useState(pageSize);

  // Row selection state
  const [selectedRowIds, setSelectedRowIds] = useState<Set<string>>(new Set());

  // Focused row for keyboard navigation
  const [focusedRowIndex, setFocusedRowIndex] = useState<number>(-1);
  const tableRef = useRef<HTMLTableElement>(null);

  // Filter columns based on visibility
  const visibleColumns = useMemo(
    () => initialColumns.filter((c) => visibleColumnIds.has(c.id)),
    [initialColumns, visibleColumnIds]
  );

  // Sort data
  const sortedData = useMemo(() => {
    if (!sortColumnId) return data;
    const col = initialColumns.find((c) => c.id === sortColumnId);
    if (!col || !col.accessorKey) return data;

    const key = col.accessorKey;
    return [...data].sort((a, b) => {
      const aVal = a[key];
      const bVal = b[key];
      if (aVal === bVal) return 0;
      if (aVal === undefined || aVal === null) return 1;
      if (bVal === undefined || bVal === null) return -1;
      const compareResult = aVal < bVal ? -1 : 1;
      return sortDirection === 'asc' ? compareResult : -compareResult;
    });
  }, [data, sortColumnId, sortDirection, initialColumns]);

  // Paginate data
  const totalPages = Math.ceil(sortedData.length / currentPageSize) || 1;
  const paginatedData = useMemo(() => {
    const start = (currentPage - 1) * currentPageSize;
    return sortedData.slice(start, start + currentPageSize);
  }, [sortedData, currentPage, currentPageSize]);

  // Selection helpers
  const isAllPageSelected =
    paginatedData.length > 0 &&
    paginatedData.every((item) => selectedRowIds.has(item.id));

  const toggleSelectAllPage = () => {
    const next = new Set(selectedRowIds);
    if (isAllPageSelected) {
      paginatedData.forEach((item) => next.delete(item.id));
    } else {
      paginatedData.forEach((item) => next.add(item.id));
    }
    setSelectedRowIds(next);
  };

  const toggleRowSelect = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const next = new Set(selectedRowIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    setSelectedRowIds(next);
  };

  const selectedRows = useMemo(
    () => data.filter((item) => selectedRowIds.has(item.id)),
    [data, selectedRowIds]
  );

  // Sorting handler
  const handleSort = (columnId: string) => {
    if (sortColumnId === columnId) {
      if (sortDirection === 'asc') {
        setSortDirection('desc');
      } else {
        setSortColumnId(null);
        setSortDirection('asc');
      }
    } else {
      setSortColumnId(columnId);
      setSortDirection('asc');
    }
  };

  // CSV Export gated by role permission
  const handleExportCsv = () => {
    if (role === 'AUDITOR' && !enableExport) {
      addToast({
        type: 'warning',
        title: 'Export Restricted',
        message: 'Direct bulk data export is restricted for read-only auditor role.',
      });
      return;
    }

    const itemsToExport = selectedRows.length > 0 ? selectedRows : data;
    const headers = visibleColumns.map((c) => c.header).join(',');
    const rows = itemsToExport
      .map((item) =>
        visibleColumns
          .map((c) => {
            const val = c.accessorKey ? item[c.accessorKey] : '';
            return `"${String(val ?? '').replace(/"/g, '""')}"`;
          })
          .join(',')
      )
      .join('\n');

    const csvContent = `data:text/csv;charset=utf-8,${headers}\n${rows}`;
    const encodedUri = encodeURI(csvContent);
    const link = document.createElement('a');
    link.setAttribute('href', encodedUri);
    link.setAttribute('download', exportFilename);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);

    addToast({
      type: 'success',
      title: 'CSV Export Generated',
      message: `Exported ${itemsToExport.length} rows to ${exportFilename}`,
    });
  };

  // Keyboard navigation across table
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (paginatedData.length === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setFocusedRowIndex((prev) => Math.min(prev + 1, paginatedData.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setFocusedRowIndex((prev) => Math.max(prev - 1, 0));
    } else if (e.key === ' ' && focusedRowIndex >= 0) {
      e.preventDefault();
      const row = paginatedData[focusedRowIndex];
      if (row) {
        const next = new Set(selectedRowIds);
        if (next.has(row.id)) next.delete(row.id);
        else next.add(row.id);
        setSelectedRowIds(next);
      }
    } else if (e.key === 'Enter' && focusedRowIndex >= 0) {
      e.preventDefault();
      const row = paginatedData[focusedRowIndex];
      if (row && onRowClick) {
        onRowClick(row);
      }
    }
  };

  const isCompact = density === 'compact';

  return (
    <div
      className={clsx(
        'border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs flex flex-col',
        className
      )}
      tabIndex={0}
      onKeyDown={handleKeyDown}
      role="region"
      aria-label="Data Table with Keyboard Navigation (Up/Down arrows to navigate, Space to select, Enter to view)"
    >
      {/* Table Toolbar (Column Chooser, Export, Bulk Actions) */}
      <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[13px]">
        {/* Left side: Bulk actions */}
        <div className="flex items-center gap-2">
          {enableMultiSelect && selectedRowIds.size > 0 ? (
            <div className="flex items-center gap-2 bg-[var(--primary-subtle)] text-[var(--primary)] px-2.5 py-1 rounded border border-[var(--primary)] text-[12px] font-medium">
              <span>{selectedRowIds.size} row(s) selected</span>
              <button
                type="button"
                onClick={() => setSelectedRowIds(new Set())}
                className="underline hover:text-[var(--primary-hover)] cursor-pointer ml-1"
              >
                Clear
              </button>

              {/* Bulk Action Buttons */}
              <div className="flex items-center gap-1.5 ml-2 pl-2 border-l border-[var(--primary)]">
                {bulkActions.map((action) => (
                  <Button
                    key={action.id}
                    size="sm"
                    variant={action.variant || 'secondary'}
                    onClick={() => action.onClick(selectedRows)}
                  >
                    {action.icon}
                    <span>{action.label}</span>
                  </Button>
                ))}
              </div>
            </div>
          ) : (
            <div className="text-[12px] text-[var(--text-muted)] font-mono-financial">
              Total {data.length} records available
            </div>
          )}
        </div>

        {/* Right side: Column Chooser & CSV Export */}
        <div className="flex items-center gap-2">
          {enableColumnChooser && (
            <div className="relative">
              <button
                type="button"
                onClick={() => setColumnChooserOpen((prev) => !prev)}
                className="inline-flex items-center gap-1.5 px-2.5 py-1 text-[12px] font-medium bg-[var(--surface)] text-[var(--text-secondary)] border border-[var(--surface-border)] rounded hover:bg-[var(--surface-raised)] cursor-pointer"
                aria-expanded={columnChooserOpen}
                aria-haspopup="true"
              >
                <Columns3 className="w-3.5 h-3.5" aria-hidden="true" />
                <span>Columns</span>
                <ChevronDown className="w-3 h-3 text-[var(--text-muted)]" aria-hidden="true" />
              </button>

              {columnChooserOpen && (
                <div
                  className="absolute right-0 top-full mt-1 w-52 bg-[var(--surface)] border border-[var(--surface-border)] rounded shadow-lg p-2 z-20 space-y-1 text-[12px]"
                  role="menu"
                >
                  <div className="text-[11px] font-semibold text-[var(--text-muted)] px-1 uppercase tracking-wider mb-1">
                    Toggle Columns
                  </div>
                  {initialColumns.map((col) => {
                    const isVisible = visibleColumnIds.has(col.id);
                    return (
                      <label
                        key={col.id}
                        className="flex items-center gap-2 px-1.5 py-1 rounded hover:bg-[var(--canvas-subdued)] cursor-pointer text-[var(--text-primary)]"
                      >
                        <input
                          type="checkbox"
                          checked={isVisible}
                          disabled={isVisible && visibleColumnIds.size === 1}
                          onChange={() => {
                            const next = new Set(visibleColumnIds);
                            if (isVisible) next.delete(col.id);
                            else next.add(col.id);
                            setVisibleColumnIds(next);
                          }}
                          className="rounded text-[var(--primary)] focus:ring-[var(--focus-ring)]"
                        />
                        <span>{col.header}</span>
                      </label>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {enableExport && (
            <Button
              size="sm"
              variant="secondary"
              leftIcon={<Download className="w-3.5 h-3.5" aria-hidden="true" />}
              onClick={handleExportCsv}
              title="Export visible rows to CSV"
            >
              Export CSV
            </Button>
          )}
        </div>
      </div>

      {/* Main Table Container with Sticky Header */}
      <div className="overflow-x-auto overflow-y-auto max-h-[600px] relative">
        <table
          ref={tableRef}
          className="w-full text-left border-collapse"
          style={{ fontSize: isCompact ? '13px' : '14px' }}
        >
          <thead className="sticky top-0 bg-[var(--canvas-subdued)] z-10 border-b border-[var(--surface-border)] text-[var(--text-secondary)] font-semibold select-none">
            <tr>
              {enableMultiSelect && (
                <th scope="col" className="w-10 px-3 py-2 text-center">
                  <input
                    type="checkbox"
                    checked={isAllPageSelected}
                    onChange={toggleSelectAllPage}
                    aria-label="Select all rows on current page"
                    className="rounded text-[var(--primary)] focus:ring-[var(--focus-ring)] cursor-pointer"
                  />
                </th>
              )}
              {visibleColumns.map((col) => {
                const isSorted = sortColumnId === col.id;
                const canSort = col.enableSorting !== false;

                return (
                  <th
                    key={col.id}
                    scope="col"
                    aria-sort={
                      isSorted
                        ? sortDirection === 'asc'
                          ? 'ascending'
                          : 'descending'
                        : 'none'
                    }
                    className={clsx(
                      'px-3 py-2 whitespace-nowrap text-[12px] font-semibold tracking-tight border-r border-[var(--surface-border-subtle)] last:border-r-0',
                      canSort && 'cursor-pointer hover:bg-[var(--surface-raised)]'
                    )}
                    onClick={() => canSort && handleSort(col.id)}
                    style={{ minWidth: col.minWidth, width: col.width }}
                  >
                    <div className="flex items-center justify-between gap-1.5">
                      <span>{col.header}</span>
                      {canSort && (
                        <span className="text-[var(--text-muted)]">
                          {isSorted ? (
                            sortDirection === 'asc' ? (
                              <ArrowUp className="w-3 h-3 text-[var(--primary)]" />
                            ) : (
                              <ArrowDown className="w-3 h-3 text-[var(--primary)]" />
                            )
                          ) : (
                            <ArrowUpDown className="w-3 h-3 opacity-40 hover:opacity-100" />
                          )}
                        </span>
                      )}
                    </div>
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--surface-border-subtle)]">
            {paginatedData.length === 0 ? (
              <tr>
                <td
                  colSpan={visibleColumns.length + (enableMultiSelect ? 1 : 0)}
                  className="px-6 py-12 text-center text-[var(--text-muted)]"
                >
                  <p>{emptyMessage}</p>
                </td>
              </tr>
            ) : (
              paginatedData.map((row, idx) => {
                const isSelected = selectedRowIds.has(row.id);
                const isFocused = focusedRowIndex === idx;

                return (
                  <tr
                    key={row.id}
                    onClick={() => onRowClick?.(row)}
                    className={clsx(
                      'transition-colors cursor-pointer',
                      isSelected
                        ? 'bg-[var(--primary-subtle)]/30 hover:bg-[var(--primary-subtle)]/50'
                        : isFocused
                        ? 'bg-[var(--canvas-subdued)]'
                        : 'hover:bg-[var(--surface-raised)]',
                      isFocused && 'ring-2 ring-[var(--focus-ring)] ring-inset'
                    )}
                  >
                    {enableMultiSelect && (
                      <td className="w-10 px-3 py-2 text-center" onClick={(e) => toggleRowSelect(row.id, e)}>
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => {}} // Handled by container TD click
                          aria-label={`Select row ${row.id}`}
                          className="rounded text-[var(--primary)] focus:ring-[var(--focus-ring)] cursor-pointer"
                        />
                      </td>
                    )}
                    {visibleColumns.map((col) => {
                      const rawValue = col.accessorKey ? row[col.accessorKey] : undefined;
                      return (
                        <td
                          key={col.id}
                          className={clsx(
                            'px-3 whitespace-nowrap text-[var(--text-primary)]',
                            isCompact ? 'py-1.5' : 'py-2.5'
                          )}
                        >
                          {col.cell
                            ? col.cell({ row, value: rawValue })
                            : String(rawValue ?? '')}
                        </td>
                      );
                    })}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination Footer */}
      <Pagination
        currentPage={currentPage}
        totalPages={totalPages}
        pageSize={currentPageSize}
        totalRecords={sortedData.length}
        onPageChange={setCurrentPage}
        onPageSizeChange={(newSize) => {
          setCurrentPageSize(newSize);
          setCurrentPage(1);
        }}
      />
    </div>
  );
}
