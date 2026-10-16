import React from 'react';
import { Search, X, Bookmark } from 'lucide-react';
import clsx from 'clsx';
import type { Channel, FinancialStatus } from '../types/domain';

export interface SavedView {
  id: string;
  name: string;
  count?: number;
  filters: {
    query?: string;
    channel?: Channel;
    status?: FinancialStatus;
  };
}

export interface SearchFilterBarProps {
  query: string;
  onQueryChange: (q: string) => void;
  selectedChannel?: Channel;
  onChannelChange: (c: Channel | undefined) => void;
  selectedStatus?: FinancialStatus;
  onStatusChange: (s: FinancialStatus | undefined) => void;
  savedViews?: SavedView[];
  activeSavedViewId?: string;
  onSelectSavedView?: (view: SavedView) => void;
  onClearFilters: () => void;
  className?: string;
}

const DEFAULT_SAVED_VIEWS: SavedView[] = [
  {
    id: 'sv-pending-approval',
    name: 'Pending Maker-Checker',
    count: 2,
    filters: { status: 'approval' },
  },
  {
    id: 'sv-unmatched-nip',
    name: 'Critical Unmatched NIP',
    count: 3,
    filters: { channel: 'NIP', status: 'failed' },
  },
  {
    id: 'sv-pos-timeouts',
    name: 'POS Timeout Exceptions',
    count: 1,
    filters: { channel: 'POS', status: 'approval' },
  },
  {
    id: 'sv-all-investigating',
    name: 'Active Investigations',
    count: 4,
    filters: { status: 'investigating' },
  },
];

export const SearchFilterBar: React.FC<SearchFilterBarProps> = ({
  query,
  onQueryChange,
  selectedChannel,
  onChannelChange,
  selectedStatus,
  onStatusChange,
  savedViews = DEFAULT_SAVED_VIEWS,
  activeSavedViewId,
  onSelectSavedView,
  onClearFilters,
  className,
}) => {
  const hasActiveFilters = Boolean(query || selectedChannel || selectedStatus || activeSavedViewId);

  return (
    <div className={clsx('space-y-2.5', className)}>
      {/* Primary Search and Filter Row */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Search Input */}
        <div className="relative flex-1 min-w-[260px]">
          <Search
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)] pointer-events-none"
            aria-hidden="true"
          />
          <input
            type="search"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Search by Reference, RRN, STAN, Account Number..."
            className="w-full pl-9 pr-3 py-1.5 text-[13px] bg-[var(--surface)] text-[var(--text-primary)] border border-[var(--surface-border)] rounded placeholder-[var(--text-muted)] focus:outline-2 focus:outline-[var(--focus-ring)]"
            aria-label="Filter transactions"
          />
        </div>

        {/* Channel Dropdown */}
        <div className="flex items-center gap-1.5">
          <label htmlFor="channel-filter" className="sr-only">
            Filter by channel
          </label>
          <select
            id="channel-filter"
            value={selectedChannel || ''}
            onChange={(e) => onChannelChange(e.target.value ? (e.target.value as Channel) : undefined)}
            className="px-2.5 py-1.5 text-[13px] bg-[var(--surface)] text-[var(--text-primary)] border border-[var(--surface-border)] rounded focus:outline-2 focus:outline-[var(--focus-ring)]"
          >
            <option value="">All Channels</option>
            <option value="NIP">NIP (Instant Payment)</option>
            <option value="POS">POS Terminal</option>
            <option value="CARD">Card Payment</option>
            <option value="WEBHOOK">Fintech Webhook</option>
            <option value="DIRECT_DEBIT">Direct Debit</option>
          </select>
        </div>

        {/* Status Dropdown */}
        <div className="flex items-center gap-1.5">
          <label htmlFor="status-filter" className="sr-only">
            Filter by status
          </label>
          <select
            id="status-filter"
            value={selectedStatus || ''}
            onChange={(e) =>
              onStatusChange(e.target.value ? (e.target.value as FinancialStatus) : undefined)
            }
            className="px-2.5 py-1.5 text-[13px] bg-[var(--surface)] text-[var(--text-primary)] border border-[var(--surface-border)] rounded focus:outline-2 focus:outline-[var(--focus-ring)]"
          >
            <option value="">All Statuses</option>
            <option value="detected">Detected</option>
            <option value="awaiting">Awaiting Action</option>
            <option value="investigating">Investigating</option>
            <option value="approval">Pending Approval</option>
            <option value="submitted">Submitted</option>
            <option value="verifying">Verifying</option>
            <option value="resolved">Resolved</option>
            <option value="failed">Failed</option>
            <option value="suppressed">Suppressed</option>
          </select>
        </div>

        {/* Clear Filters */}
        {hasActiveFilters && (
          <button
            type="button"
            onClick={onClearFilters}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] bg-[var(--surface)] border border-[var(--surface-border)] rounded hover:bg-[var(--surface-raised)] cursor-pointer"
          >
            <X className="w-3.5 h-3.5" aria-hidden="true" />
            <span>Reset</span>
          </button>
        )}
      </div>

      {/* Saved Views Row */}
      {savedViews.length > 0 && (
        <div className="flex items-center gap-2 overflow-x-auto pb-1 text-[12px]">
          <div className="inline-flex items-center gap-1 text-[var(--text-muted)] font-medium shrink-0">
            <Bookmark className="w-3 h-3" aria-hidden="true" />
            <span>Saved Views:</span>
          </div>
          <div className="flex items-center gap-1.5">
            {savedViews.map((sv) => {
              const isActive = activeSavedViewId === sv.id;
              return (
                <button
                  key={sv.id}
                  type="button"
                  onClick={() => onSelectSavedView?.(sv)}
                  className={clsx(
                    'inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full border transition-colors cursor-pointer whitespace-nowrap',
                    isActive
                      ? 'bg-[var(--primary-subtle)] text-[var(--primary)] border-[var(--primary)] font-semibold'
                      : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border)] hover:bg-[var(--surface-raised)] hover:text-[var(--text-primary)]'
                  )}
                >
                  <span>{sv.name}</span>
                  {sv.count !== undefined && (
                    <span
                      className={clsx(
                        'px-1.5 py-0.2 text-[10px] rounded-full font-mono-financial',
                        isActive
                          ? 'bg-[var(--primary)] text-white'
                          : 'bg-[var(--canvas-subdued)] text-[var(--text-muted)]'
                      )}
                    >
                      {sv.count}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};
