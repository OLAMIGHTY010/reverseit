import React from 'react';
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react';
import clsx from 'clsx';

export interface PaginationProps {
  currentPage: number;
  totalPages: number;
  pageSize: number;
  totalRecords: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
  pageSizeOptions?: number[];
  className?: string;
}

export const Pagination: React.FC<PaginationProps> = ({
  currentPage,
  totalPages,
  pageSize,
  totalRecords,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 25, 50, 100],
  className,
}) => {
  const startRecord = Math.min((currentPage - 1) * pageSize + 1, totalRecords);
  const endRecord = Math.min(currentPage * pageSize, totalRecords);

  return (
    <div
      className={clsx(
        'flex flex-wrap items-center justify-between gap-3 px-4 py-2 text-[12px] text-[var(--text-secondary)] border-t border-[var(--surface-border)] bg-[var(--surface)]',
        className
      )}
      aria-label="Table pagination"
    >
      {/* Records info */}
      <div className="flex items-center gap-2">
        <span>
          Showing <strong className="text-[var(--text-primary)] font-mono-financial">{totalRecords === 0 ? 0 : startRecord}</strong> to{' '}
          <strong className="text-[var(--text-primary)] font-mono-financial">{endRecord}</strong> of{' '}
          <strong className="text-[var(--text-primary)] font-mono-financial">{totalRecords}</strong> records
        </span>

        {/* Page size select */}
        {onPageSizeChange && (
          <div className="flex items-center gap-1.5 ml-4">
            <label htmlFor="page-size-select" className="text-[var(--text-muted)]">
              Per page:
            </label>
            <select
              id="page-size-select"
              value={pageSize}
              onChange={(e) => onPageSizeChange(Number(e.target.value))}
              className="px-2 py-0.5 text-[12px] bg-[var(--canvas-subdued)] text-[var(--text-primary)] border border-[var(--surface-border)] rounded focus:outline-2 focus:outline-[var(--focus-ring)]"
            >
              {pageSizeOptions.map((opt) => (
                <option key={opt} value={opt}>
                  {opt}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {/* Navigation buttons */}
      <div className="flex items-center gap-1">
        <button
          type="button"
          onClick={() => onPageChange(1)}
          disabled={currentPage <= 1}
          aria-label="Go to first page"
          className="p-1 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
        >
          <ChevronsLeft className="w-4 h-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={() => onPageChange(currentPage - 1)}
          disabled={currentPage <= 1}
          aria-label="Go to previous page"
          className="p-1 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
        >
          <ChevronLeft className="w-4 h-4" aria-hidden="true" />
        </button>

        <span className="px-2 py-0.5 font-mono-financial font-medium text-[var(--text-primary)]">
          Page {currentPage} of {Math.max(1, totalPages)}
        </span>

        <button
          type="button"
          onClick={() => onPageChange(currentPage + 1)}
          disabled={currentPage >= totalPages}
          aria-label="Go to next page"
          className="p-1 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
        >
          <ChevronRight className="w-4 h-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={() => onPageChange(totalPages)}
          disabled={currentPage >= totalPages}
          aria-label="Go to last page"
          className="p-1 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] disabled:opacity-40 disabled:pointer-events-none cursor-pointer"
        >
          <ChevronsRight className="w-4 h-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
};
