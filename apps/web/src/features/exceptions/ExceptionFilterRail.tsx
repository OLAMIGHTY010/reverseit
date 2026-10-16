import React from 'react';
import {
  Filter,
  RotateCcw,
  ChevronLeft,
  ChevronRight,
  User,
  Bot,
  AlertTriangle,
  Clock,
  Layers,
  Building,
  CreditCard,
  Hash,
} from 'lucide-react';
import clsx from 'clsx';
import type {
  ClassificationCode,
  ExceptionSource,
  RootCause,
  Severity,
  FinancialStatus,
  Lane,
  Channel,
  ExceptionTeam,
  SLAStatus,
} from '../../types/domain';

export interface ExceptionFiltersState {
  search: string;
  classificationCode: ClassificationCode | 'ALL';
  source: ExceptionSource | 'ALL';
  customerReportedOnly: boolean;
  myCasesOnly: boolean;
  unassignedOnly: boolean;
  rootCause: RootCause | 'ALL';
  severity: Severity | 'ALL';
  status: FinancialStatus | 'ALL';
  lane: Lane | 'ALL';
  channel: Channel | 'ALL';
  team: ExceptionTeam | 'ALL';
  slaState: SLAStatus | 'ALL';
  minAmount?: string;
  maxAmount?: string;
  incidentOnly: boolean;
}

interface ExceptionFilterRailProps {
  filters: ExceptionFiltersState;
  onChange: (newFilters: ExceptionFiltersState) => void;
  onReset: () => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  activeFilterCount: number;
}

const CLASSIFICATION_OPTIONS: Array<{ code: ClassificationCode; label: string; desc: string }> = [
  { code: 'DUP001', label: 'DUP001', desc: 'Duplicate Debit Detected' },
  { code: 'FAIL001', label: 'FAIL001', desc: 'Switch Timeout / Error' },
  { code: 'UNK001', label: 'UNK001', desc: 'Unmatched Settlement' },
  { code: 'REV001', label: 'REV001', desc: 'Reversal Attempt Failed' },
  { code: 'STL001', label: 'STL001', desc: 'Suspense Ledger Imbalance' },
  { code: 'CUS001', label: 'CUS001', desc: 'Customer Dispute Lodged' },
  { code: 'ATM001', label: 'ATM001', desc: 'ATM Hardware / Dispenser Jam' },
  { code: 'FEE001', label: 'FEE001', desc: 'Disputed Electronic Fee' },
  { code: 'VND001', label: 'VND001', desc: 'Third-Party Biller / Vendor Drop' },
  { code: 'MIS001', label: 'MIS001', desc: 'Amount / Currency Mismatch' },
  { code: 'INC001', label: 'INC001', desc: 'Intermediary Routing Drop' },
];

export const ExceptionFilterRail: React.FC<ExceptionFilterRailProps> = ({
  filters,
  onChange,
  onReset,
  isCollapsed,
  onToggleCollapse,
  activeFilterCount,
}) => {
  if (isCollapsed) {
    return (
      <aside
        aria-label="Filter Rail Collapsed"
        className="w-12 shrink-0 bg-[var(--surface)] border-r border-[var(--surface-border)] flex flex-col items-center py-3 gap-3"
      >
        <button
          type="button"
          onClick={onToggleCollapse}
          className="p-1.5 rounded hover:bg-[var(--surface-raised)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
          title="Expand Filter Rail"
          aria-label="Expand Filter Rail"
        >
          <ChevronRight className="w-4 h-4" />
        </button>

        <div className="relative p-1.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-secondary)]">
          <Filter className="w-4 h-4" />
          {activeFilterCount > 0 && (
            <span className="absolute -top-1 -right-1 w-4 h-4 bg-sky-600 text-white font-mono-financial text-[9px] font-bold rounded-full flex items-center justify-center">
              {activeFilterCount}
            </span>
          )}
        </div>
      </aside>
    );
  }

  return (
    <aside
      aria-label="Filter Rail Expanded"
      className="w-64 shrink-0 bg-[var(--surface)] border-r border-[var(--surface-border)] flex flex-col h-full overflow-hidden text-[12px]"
    >
      {/* Rail Header */}
      <div className="p-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
        <div className="flex items-center gap-1.5 font-bold text-[13px] text-[var(--text-primary)]">
          <Filter className="w-4 h-4 text-sky-600" />
          <span>Filters</span>
          {activeFilterCount > 0 && (
            <span className="px-1.5 py-0.2 rounded-full bg-sky-100 text-sky-800 font-mono-financial font-bold text-[10px]">
              {activeFilterCount}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          {activeFilterCount > 0 && (
            <button
              type="button"
              onClick={onReset}
              className="p-1 text-[11px] text-[var(--text-muted)] hover:text-rose-600 flex items-center gap-0.5 rounded hover:bg-[var(--surface-raised)]"
              title="Reset all filters"
            >
              <RotateCcw className="w-3 h-3" />
              <span>Reset</span>
            </button>
          )}
          <button
            type="button"
            onClick={onToggleCollapse}
            className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            title="Collapse Filter Rail"
            aria-label="Collapse Filter Rail"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Scrollable Filter Form */}
      <div className="flex-1 overflow-y-auto p-3 space-y-4">
        {/* Quick Toggles (Customer-reported, My cases, Unassigned) */}
        <div className="space-y-1.5 pt-1">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            Quick Scope
          </span>

          <label className="flex items-center gap-2 p-1.5 rounded hover:bg-[var(--surface-raised)] cursor-pointer text-[12px]">
            <input
              type="checkbox"
              checked={filters.customerReportedOnly}
              onChange={(e) =>
                onChange({ ...filters, customerReportedOnly: e.target.checked })
              }
              className="rounded border-[var(--surface-border)] text-sky-600 focus:ring-sky-500 w-3.5 h-3.5"
            />
            <span className="font-semibold text-[var(--text-primary)] flex items-center gap-1">
              <User className="w-3.5 h-3.5 text-indigo-600" /> Customer-Reported Only
            </span>
          </label>

          <label className="flex items-center gap-2 p-1.5 rounded hover:bg-[var(--surface-raised)] cursor-pointer text-[12px]">
            <input
              type="checkbox"
              checked={filters.myCasesOnly}
              onChange={(e) =>
                onChange({ ...filters, myCasesOnly: e.target.checked, unassignedOnly: false })
              }
              className="rounded border-[var(--surface-border)] text-sky-600 focus:ring-sky-500 w-3.5 h-3.5"
            />
            <span className="text-[var(--text-secondary)]">My Assigned Cases</span>
          </label>

          <label className="flex items-center gap-2 p-1.5 rounded hover:bg-[var(--surface-raised)] cursor-pointer text-[12px]">
            <input
              type="checkbox"
              checked={filters.unassignedOnly}
              onChange={(e) =>
                onChange({ ...filters, unassignedOnly: e.target.checked, myCasesOnly: false })
              }
              className="rounded border-[var(--surface-border)] text-sky-600 focus:ring-sky-500 w-3.5 h-3.5"
            />
            <span className="text-[var(--text-secondary)]">Unassigned Backlog</span>
          </label>

          <label className="flex items-center gap-2 p-1.5 rounded hover:bg-[var(--surface-raised)] cursor-pointer text-[12px]">
            <input
              type="checkbox"
              checked={filters.incidentOnly}
              onChange={(e) =>
                onChange({ ...filters, incidentOnly: e.target.checked })
              }
              className="rounded border-[var(--surface-border)] text-sky-600 focus:ring-sky-500 w-3.5 h-3.5"
            />
            <span className="text-[var(--text-secondary)] flex items-center gap-1">
              <AlertTriangle className="w-3.5 h-3.5 text-amber-600" /> Linked to Incident
            </span>
          </label>
        </div>

        {/* Classification Code (Scope v2 Mandatory) */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1">
              <Hash className="w-3 h-3 text-sky-600" /> Classification Code
            </span>
            {filters.classificationCode !== 'ALL' && (
              <button
                type="button"
                onClick={() => onChange({ ...filters, classificationCode: 'ALL' })}
                className="text-[10px] text-sky-600 hover:underline"
              >
                Clear
              </button>
            )}
          </div>
          <div className="grid grid-cols-2 gap-1.5">
            {CLASSIFICATION_OPTIONS.map((opt) => (
              <button
                key={opt.code}
                type="button"
                onClick={() =>
                  onChange({
                    ...filters,
                    classificationCode:
                      filters.classificationCode === opt.code ? 'ALL' : opt.code,
                  })
                }
                title={opt.desc}
                className={clsx(
                  'px-2 py-1 rounded text-left font-mono-financial text-[11px] border transition-colors flex flex-col',
                  filters.classificationCode === opt.code
                    ? 'bg-sky-50 text-sky-950 border-sky-500 font-bold'
                    : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border-subtle)] hover:bg-[var(--surface-raised)]'
                )}
              >
                <span>{opt.label}</span>
                <span className="text-[9px] font-sans truncate text-[var(--text-muted)]">
                  {opt.desc}
                </span>
              </button>
            ))}
          </div>
        </div>

        {/* Source: Bot vs Customer */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            Source Origin
          </span>
          <div className="grid grid-cols-2 gap-1.5">
            <button
              type="button"
              onClick={() =>
                onChange({
                  ...filters,
                  source: filters.source === 'BOT' ? 'ALL' : 'BOT',
                })
              }
              className={clsx(
                'px-2 py-1 rounded text-center border text-[11px] transition-colors flex items-center justify-center gap-1',
                filters.source === 'BOT'
                  ? 'bg-sky-50 text-sky-950 border-sky-500 font-bold'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border-subtle)] hover:bg-[var(--surface-raised)]'
              )}
            >
              <Bot className="w-3.5 h-3.5 text-sky-600" /> Bot (Auto)
            </button>
            <button
              type="button"
              onClick={() =>
                onChange({
                  ...filters,
                  source: filters.source === 'CUSTOMER' ? 'ALL' : 'CUSTOMER',
                })
              }
              className={clsx(
                'px-2 py-1 rounded text-center border text-[11px] transition-colors flex items-center justify-center gap-1',
                filters.source === 'CUSTOMER'
                  ? 'bg-indigo-50 text-indigo-950 border-indigo-500 font-bold'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border-subtle)] hover:bg-[var(--surface-raised)]'
              )}
            >
              <User className="w-3.5 h-3.5 text-indigo-600" /> Customer
            </button>
          </div>
        </div>

        {/* SLA Status */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1">
            <Clock className="w-3 h-3 text-amber-600" /> SLA Deadline Status
          </span>
          <select
            value={filters.slaState}
            onChange={(e) =>
              onChange({ ...filters, slaState: e.target.value as SLAStatus | 'ALL' })
            }
            className="w-full px-2 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)]"
          >
            <option value="ALL">All SLA States</option>
            <option value="BREACHED">Breached SLA Only</option>
            <option value="APPROACHING">Approaching Breach (&lt; 1h)</option>
            <option value="ON_TRACK">Healthy (On Track)</option>
          </select>
        </div>

        {/* Severity */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            Severity
          </span>
          <div className="grid grid-cols-2 gap-1.5">
            {(['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] as Severity[]).map((sev) => (
              <button
                key={sev}
                type="button"
                onClick={() =>
                  onChange({
                    ...filters,
                    severity: filters.severity === sev ? 'ALL' : sev,
                  })
                }
                className={clsx(
                  'px-2 py-1 rounded text-center border text-[11px] font-mono-financial transition-colors',
                  filters.severity === sev
                    ? sev === 'CRITICAL'
                      ? 'bg-rose-50 text-rose-900 border-rose-400 font-bold'
                      : sev === 'HIGH'
                      ? 'bg-amber-50 text-amber-900 border-amber-400 font-bold'
                      : 'bg-sky-50 text-sky-900 border-sky-400 font-bold'
                    : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border-subtle)] hover:bg-[var(--surface-raised)]'
                )}
              >
                {sev}
              </button>
            ))}
          </div>
        </div>

        {/* Operational Lane */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1">
            <Layers className="w-3 h-3 text-indigo-600" /> Operational Lane
          </span>
          <select
            value={filters.lane}
            onChange={(e) =>
              onChange({ ...filters, lane: e.target.value as Lane | 'ALL' })
            }
            className="w-full px-2 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)]"
          >
            <option value="ALL">All Triage Lanes</option>
            <option value="FAST_TRACK">Fast Track (Auto Candidate)</option>
            <option value="MANUAL_INVESTIGATION">Manual Investigation</option>
            <option value="SUPERVISOR_QUEUE">Supervisor Dual-Auth Queue</option>
            <option value="COMPLIANCE_HOLD">Compliance & Treasury Hold</option>
          </select>
        </div>

        {/* Channel / Rail */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1">
            <CreditCard className="w-3 h-3 text-slate-600" /> Channel / Payment Rail
          </span>
          <div className="grid grid-cols-2 gap-1.5">
            {(['NIP', 'POS', 'CARD', 'DIRECT_DEBIT'] as Channel[]).map((ch) => (
              <button
                key={ch}
                type="button"
                onClick={() =>
                  onChange({
                    ...filters,
                    channel: filters.channel === ch ? 'ALL' : ch,
                  })
                }
                className={clsx(
                  'px-2 py-1 rounded text-center border text-[11px] font-mono-financial transition-colors',
                  filters.channel === ch
                    ? 'bg-sky-50 text-sky-950 border-sky-500 font-bold'
                    : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border-subtle)] hover:bg-[var(--surface-raised)]'
                )}
              >
                {ch}
              </button>
            ))}
          </div>
        </div>

        {/* Team */}
        <div className="space-y-1.5 pt-2 border-t border-[var(--surface-border-subtle)]">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1">
            <Building className="w-3 h-3 text-slate-600" /> Team Queue
          </span>
          <select
            value={filters.team}
            onChange={(e) =>
              onChange({ ...filters, team: e.target.value as ExceptionTeam | 'ALL' })
            }
            className="w-full px-2 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)]"
          >
            <option value="ALL">All Operational Teams</option>
            <option value="TIER_1_TRIAGE">Tier-1 Automated Triage</option>
            <option value="TIER_2_RECON">Tier-2 Core Settlement Recon</option>
            <option value="DISPUTE_RESOLUTION">Customer Dispute Resolution</option>
            <option value="FRAUD_OPS">Fraud & Financial Crimes</option>
          </select>
        </div>
      </div>
    </aside>
  );
};
