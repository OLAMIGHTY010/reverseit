import React from 'react';
import {
  AlertTriangle,
  Clock,
  User,
  Layers,
  Flame,
  ShieldCheck,
  ShieldAlert,
} from 'lucide-react';
import { StatusBadge } from '../../components/StatusBadge';
import { CountdownTimer } from '../exceptions/CountdownTimer';
import type { InvestigationCase } from '../../types/domain';

interface StickyCaseHeaderProps {
  investigationCase: InvestigationCase;
}

export const StickyCaseHeader: React.FC<StickyCaseHeaderProps> = ({ investigationCase }) => {
  const { exception } = investigationCase;

  const srSummary = `Case ${investigationCase.caseNumber}, Reference ${investigationCase.reference}, Classification ${exception.classificationCode}, Severity ${exception.severity}, Status ${investigationCase.status}, Amount ${exception.currency} ${exception.amount}, Exposure ${exception.currency} ${exception.exposure}, Assigned to ${investigationCase.assignedAnalyst}.`;

  return (
    <header
      role="banner"
      aria-label={`Case Header: ${investigationCase.caseNumber}`}
      className="sticky top-0 z-20 bg-[var(--surface)] border-b border-[var(--surface-border)] shadow-xs"
    >
      {/* Screen reader summary */}
      <div className="sr-only" aria-live="polite">
        {srSummary}
      </div>

      {/* Kill-switch and Degraded-Source Alert Banners */}
      {investigationCase.killSwitchActive && (
        <div className="px-4 py-1.5 bg-rose-600 text-white text-[12px] font-bold flex items-center justify-between">
          <div className="flex items-center gap-2">
            <ShieldAlert className="w-4 h-4 text-white" />
            <span>
              KILL SWITCH ENGAGED: Automated zero-touch reversal disabled for {exception.channel} gateway. Manual dual authorization required.
            </span>
          </div>
          <span className="font-mono-financial text-[11px] opacity-90">Protocol SEC-09</span>
        </div>
      )}

      {investigationCase.degradedSourceBanner && (
        <div className="px-4 py-1.5 bg-amber-50 border-b border-amber-300 text-amber-950 text-[12px] font-medium flex items-center justify-between">
          <div className="flex items-center gap-2">
            <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0" />
            <span>{investigationCase.degradedSourceBanner}</span>
          </div>
          <span className="font-mono-financial text-[11px] text-amber-800">Delayed feed</span>
        </div>
      )}

      {/* Case Header Details Grid */}
      <div className="p-4 flex flex-wrap items-center justify-between gap-4">
        {/* Left: Case ID, Classification, Reference, Severity, Status */}
        <div className="space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono-financial font-extrabold text-[16px] text-[var(--text-primary)]">
              {investigationCase.caseNumber}
            </span>
            <span className="text-[11px] font-mono-financial font-bold px-2 py-0.5 rounded bg-sky-50 text-sky-900 border border-sky-300">
              {exception.classificationCode}
            </span>
            <span
              className={`text-[10px] font-bold px-2 py-0.5 rounded font-mono-financial border ${
                exception.severity === 'CRITICAL'
                  ? 'bg-rose-100 text-rose-900 border-rose-300'
                  : exception.severity === 'HIGH'
                  ? 'bg-amber-100 text-amber-900 border-amber-300'
                  : 'bg-slate-100 text-slate-800 border-slate-300'
              }`}
            >
              {exception.severity}
            </span>
            <StatusBadge status={investigationCase.status} size="sm" />
            {investigationCase.isRootCauseConfirmed && (
              <span className="inline-flex items-center gap-1 text-[11px] font-medium px-2 py-0.5 rounded bg-emerald-50 text-emerald-800 border border-emerald-300">
                <ShieldCheck className="w-3 h-3 text-emerald-600" /> Root Cause Confirmed
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-3 text-[12px] text-[var(--text-secondary)] font-mono-financial">
            <span>
              Tx Ref: <strong className="text-[var(--text-primary)]">{investigationCase.reference}</strong>
            </span>
            <span>•</span>
            <span className="flex items-center gap-1">
              <User className="w-3.5 h-3.5 text-slate-500" />
              <span>Owner: {investigationCase.assignedAnalyst}</span>
            </span>
            <span>•</span>
            <span className="flex items-center gap-1">
              <Layers className="w-3.5 h-3.5 text-indigo-600" />
              <span>Lane: {exception.lane.replace(/_/g, ' ')}</span>
            </span>
          </div>
        </div>

        {/* Right: Dual Countdowns & Financial Exposure */}
        <div className="flex flex-wrap items-center gap-5">
          {/* Dual SLA Clocks */}
          <div className="flex items-center gap-3">
            <div className="text-right space-y-0.5">
              <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)] block flex items-center justify-end gap-1">
                <Clock className="w-3 h-3 text-amber-600" /> Customer Resolution Clock (CBN)
              </span>
              <CountdownTimer targetDate={exception.regulatoryDeadline} label="CBN 72h" type="regulatory" />
            </div>

            <div className="text-right space-y-0.5">
              <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)] block flex items-center justify-end gap-1">
                <Flame className="w-3 h-3 text-rose-600" /> Internal SLA Target
              </span>
              <CountdownTimer targetDate={exception.internalSlaTarget} label="Internal" />
            </div>
          </div>

          {/* Amount & Exposure */}
          <div className="pl-4 border-l border-[var(--surface-border)] text-right">
            <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] block">Disputed Amount</span>
            <div className="font-mono-financial font-extrabold text-[18px] text-[var(--text-primary)]">
              {exception.currency}{' '}
              {Number(exception.amount).toLocaleString('en-US', {
                minimumFractionDigits: 2,
              })}
            </div>
            <span className="text-[11px] font-mono-financial text-rose-700 font-semibold block">
              Exposure: {exception.currency} {Number(exception.exposure).toLocaleString('en-US')} ({exception.exposureType.replace(/_/g, ' ')})
            </span>
          </div>
        </div>
      </div>
    </header>
  );
};
