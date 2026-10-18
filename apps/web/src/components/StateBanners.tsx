import React from 'react';
import {
  Inbox,
  AlertCircle,
  RefreshCw,
  ClockAlert,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from './Button';

// 1. Empty State Component
export interface EmptyStateProps {
  title: string;
  description: string;
  actionText?: string;
  onAction?: () => void;
  icon?: React.ReactNode;
  className?: string;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  title,
  description,
  actionText,
  onAction,
  icon,
  className,
}) => {
  return (
    <div
      className={clsx(
        'p-10 text-center flex flex-col items-center justify-center border border-dashed border-[var(--surface-border)] rounded-md bg-[var(--surface)] text-[var(--text-secondary)] space-y-3',
        className
      )}
      role="status"
    >
      <div className="p-3 rounded-full bg-[var(--canvas-subdued)] text-[var(--text-muted)]">
        {icon || <Inbox className="w-8 h-8" aria-hidden="true" />}
      </div>
      <div className="max-w-md space-y-1">
        <h4 className="text-[15px] font-semibold text-[var(--text-primary)]">{title}</h4>
        <p className="text-[13px] text-[var(--text-muted)] leading-relaxed">{description}</p>
      </div>
      {actionText && onAction && (
        <Button variant="secondary" size="sm" onClick={onAction}>
          {actionText}
        </Button>
      )}
    </div>
  );
};

// 2. Error Boundary Banner
export interface ErrorBannerProps {
  title?: string;
  message: string;
  onRetry?: () => void;
  className?: string;
}

export const ErrorBanner: React.FC<ErrorBannerProps> = ({
  title = 'Service Error Encountered',
  message,
  onRetry,
  className,
}) => {
  return (
    <div
      className={clsx(
        'p-3.5 bg-rose-50 border border-rose-300 rounded-md text-rose-950 flex items-start justify-between gap-3 text-[13px]',
        className
      )}
      role="alert"
    >
      <div className="flex items-start gap-2.5">
        <AlertCircle className="w-4 h-4 text-rose-600 shrink-0 mt-0.5" aria-hidden="true" />
        <div>
          <div className="font-semibold text-rose-900">{title}</div>
          <div className="text-[12px] text-rose-800 leading-relaxed mt-0.5">{message}</div>
        </div>
      </div>
      {onRetry && (
        <Button
          variant="secondary"
          size="sm"
          onClick={onRetry}
          leftIcon={<RefreshCw className="w-3.5 h-3.5" />}
          className="shrink-0 bg-white"
        >
          Retry Request
        </Button>
      )}
    </div>
  );
};

// 3. Stale-Data Banner
export interface StaleDataBannerProps {
  lastUpdated: string;
  onRefresh?: () => void;
  isRefreshing?: boolean;
  className?: string;
}

export const StaleDataBanner: React.FC<StaleDataBannerProps> = ({
  lastUpdated,
  onRefresh,
  isRefreshing = false,
  className,
}) => {
  return (
    <div
      className={clsx(
        'px-3 sm:px-4 py-2 bg-amber-50 dark:bg-amber-950/40 border border-amber-300 dark:border-amber-800 rounded text-amber-950 dark:text-amber-200 flex flex-col sm:flex-row sm:items-center justify-between gap-2 text-[12px]',
        className
      )}
      role="status"
    >
      <div className="flex items-center gap-2 min-w-0">
        <ClockAlert className="w-4 h-4 text-amber-700 dark:text-amber-400 shrink-0" aria-hidden="true" />
        <span className="text-[11px] sm:text-[12px]">
          <strong>Telemetry Cached:</strong> Feed last synchronized at{' '}
          <span className="font-mono-financial font-semibold">{lastUpdated}</span>. Switch polling active.
        </span>
      </div>
      {onRefresh && (
        <button
          type="button"
          onClick={onRefresh}
          disabled={isRefreshing}
          className="font-medium underline hover:text-amber-800 dark:hover:text-amber-300 cursor-pointer flex items-center gap-1 shrink-0 self-end sm:self-auto text-[11px] sm:text-[12px]"
        >
          <RefreshCw className={clsx('w-3 h-3', isRefreshing && 'animate-spin')} />
          <span>Refresh Now</span>
        </button>
      )}
    </div>
  );
};

// 4. Skeleton Loader
export const SkeletonLoader: React.FC<{ rows?: number; className?: string }> = ({
  rows = 5,
  className,
}) => {
  return (
    <div className={clsx('space-y-2 animate-pulse', className)} role="status" aria-label="Loading data">
      <div className="h-9 bg-[var(--canvas-subdued)] rounded w-full border border-[var(--surface-border)]" />
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="h-10 bg-[var(--surface-raised)] rounded w-full border border-[var(--surface-border-subtle)]"
        />
      ))}
    </div>
  );
};
