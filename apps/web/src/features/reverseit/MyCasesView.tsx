import React, { useState } from 'react';
import {
  Clock,
  ArrowRight,
  Plus,
  Search,
  AlertCircle,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { CustomerDisputeCase } from './reverseitTypes';
import { INITIAL_CUSTOMER_CASES } from './reverseitData';

export interface MyCasesViewProps {
  cases: CustomerDisputeCase[];
  onSelectCase: (disputeCase: CustomerDisputeCase) => void;
  onStartNewReport: () => void;
}

export const MyCasesView: React.FC<MyCasesViewProps> = ({
  cases,
  onSelectCase,
  onStartNewReport,
}) => {
  const [trackingQuery, setTrackingQuery] = useState('');
  const [lookupError, setLookupError] = useState<string | null>(null);

  const handleLookup = (e: React.FormEvent) => {
    e.preventDefault();
    const query = trackingQuery.trim().toLowerCase();
    if (!query) return;

    // Search within customer's own reports first, then match by exact tracking ID / Case reference
    const found =
      cases.find(
        (c) =>
          c.caseNumber.toLowerCase() === query ||
          c.trackingId.toLowerCase() === query ||
          c.reference.toLowerCase() === query
      ) ||
      INITIAL_CUSTOMER_CASES.find(
        (c) =>
          c.caseNumber.toLowerCase() === query ||
          c.trackingId.toLowerCase() === query ||
          c.reference.toLowerCase() === query
      );

    if (found) {
      setLookupError(null);
      onSelectCase(found);
    } else {
      setLookupError('No dispute case found matching that Tracking ID or Case Reference.');
    }
  };

  return (
    <div className="max-w-3xl mx-auto py-4 px-4 space-y-6 animate-in fade-in duration-200">
      {/* Title & Action */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-[20px] font-bold text-[var(--text-primary)]">
            My Dispute Cases ({cases.length})
          </h2>
          <p className="text-[13px] text-[var(--text-muted)] mt-0.5">
            Real-time status tracking and Central Bank of Nigeria (CBN) resolution clocks.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={onStartNewReport}
          leftIcon={<Plus className="w-4 h-4" />}
        >
          Report Another Issue
        </Button>
      </div>

      {/* Private Tracking Lookup by Reference / Tracking ID */}
      <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl space-y-2">
        <label htmlFor="tracking-search-input" className="block text-xs font-semibold text-[var(--text-primary)]">
          Track a Case by Tracking ID or Reference Number
        </label>
        <form onSubmit={handleLookup} className="flex gap-2">
          <div className="relative flex-1">
            <Search className="w-4 h-4 text-[var(--text-muted)] absolute left-3 top-2.5 pointer-events-none" />
            <input
              id="tracking-search-input"
              type="text"
              value={trackingQuery}
              onChange={(e) => {
                setTrackingQuery(e.target.value);
                setLookupError(null);
              }}
              placeholder="e.g. TRK-990142 or CUS-2026-004417"
              className="w-full pl-9 pr-3 py-2 text-xs bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md font-mono-financial focus:outline-none focus:border-[var(--primary)] text-[var(--text-primary)]"
            />
          </div>
          <Button variant="secondary" size="sm" type="submit">
            Track
          </Button>
        </form>
        {lookupError && (
          <div className="flex items-center gap-1.5 text-xs text-rose-600 mt-1">
            <AlertCircle className="w-3.5 h-3.5" />
            <span>{lookupError}</span>
          </div>
        )}
      </div>

      {/* Cases List (Only displays this customer's own reports) */}
      {cases.length === 0 ? (
        <div className="p-12 text-center bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl space-y-3">
          <Clock className="w-12 h-12 mx-auto text-slate-300" />
          <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
            No active disputes filed
          </h3>
          <p className="text-[13px] text-[var(--text-muted)] max-w-sm mx-auto">
            You currently have no open transaction investigations. Report an alert anytime something goes wrong.
          </p>
          <Button variant="primary" size="md" onClick={onStartNewReport}>
            Report a Transaction Problem
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {cases.map((c) => (
            <div
              key={c.id}
              onClick={() => onSelectCase(c)}
              className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] hover:border-sky-500 rounded-xl shadow-xs hover:shadow-sm transition-all cursor-pointer space-y-3 group"
            >
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-[15px] font-mono-financial text-[var(--text-primary)] group-hover:text-sky-700">
                      {c.caseNumber}
                    </span>
                    <span className="px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider bg-sky-100 text-sky-800">
                      {c.channel}
                    </span>
                  </div>
                  <div className="text-[12px] font-mono-financial text-[var(--text-muted)] mt-0.5">
                    Ref: {c.reference} &bull; Reported {new Date(c.reportedAt).toLocaleDateString()}
                  </div>
                </div>

                <div className="text-right">
                  <span className="text-[18px] font-bold font-mono-financial text-[var(--text-primary)] block">
                    {c.currency} {c.amount}
                  </span>
                  <span className="text-[11px] font-semibold text-sky-700 bg-sky-50 px-2 py-0.5 rounded border border-sky-200">
                    {c.statusLabel}
                  </span>
                </div>
              </div>

              {/* Status summary line */}
              <p className="text-[12px] text-[var(--text-secondary)] line-clamp-1">
                {c.customerSafeStatusText}
              </p>

              {/* Countdown & details pill */}
              <div className="flex items-center justify-between pt-2 border-t border-[var(--surface-border-subtle)] text-[11px]">
                <div className="flex items-center gap-1.5 font-semibold text-sky-700">
                  <Clock className="w-3.5 h-3.5" />
                  <span>{c.timeRemainingText}</span>
                </div>
                <span className="text-sky-600 group-hover:underline flex items-center gap-1 font-medium">
                  <span>Track lifecycle</span>
                  <ArrowRight className="w-3 h-3" />
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
