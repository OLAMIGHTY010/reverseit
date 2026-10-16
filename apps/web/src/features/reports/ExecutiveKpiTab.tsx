import React from 'react';
import {
  TrendingUp,
  ShieldCheck,
  Clock,
  Zap,
  Users,
  Award,
  ArrowUpRight,
  BarChart3,
} from 'lucide-react';
import type { ExecutiveKpiSummary } from './reportsTypes';

interface ExecutiveKpiTabProps {
  kpi: ExecutiveKpiSummary;
}

export const ExecutiveKpiTab: React.FC<ExecutiveKpiTabProps> = ({ kpi }) => {
  return (
    <div className="space-y-6">
      {/* KPI Cards Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {/* Card 1: Value Recovered */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <span>Total Value Recovered</span>
            <div className="p-1.5 rounded-md bg-emerald-100 text-emerald-800">
              <TrendingUp className="w-4 h-4" />
            </div>
          </div>
          <div className="text-[26px] font-bold font-mono-financial text-emerald-700">
            {kpi.totalValueRecoveredNgn}
          </div>
          <div className="text-[11px] text-[var(--text-secondary)] flex items-center gap-1 font-mono-financial">
            <ArrowUpRight className="w-3.5 h-3.5 text-emerald-600" />
            <span className="text-emerald-700 font-bold">+14.8%</span>
            <span>vs previous monthly settlement cycle</span>
          </div>
        </div>

        {/* Card 2: SLA Compliance */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <span>CBN Statutory SLA Compliance</span>
            <div className="p-1.5 rounded-md bg-blue-100 text-blue-800">
              <ShieldCheck className="w-4 h-4" />
            </div>
          </div>
          <div className="text-[26px] font-bold font-mono-financial text-[var(--text-primary)]">
            {kpi.slaCompliancePct}%
          </div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Mandate: &gt;95% within 24h NIP / 72h POS. Zero regulatory citations.
          </div>
        </div>

        {/* Card 3: STP Auto-Resolution Rate */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <span>Straight-Through Processing (STP)</span>
            <div className="p-1.5 rounded-md bg-indigo-100 text-indigo-800">
              <Zap className="w-4 h-4" />
            </div>
          </div>
          <div className="text-[26px] font-bold font-mono-financial text-indigo-700">
            {kpi.stpResolutionRatePct}%
          </div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Zero-touch reversal execution through the 8-check automated gate.
          </div>
        </div>

        {/* Card 4: Mean Time to Resolve */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <span>Mean Time to Resolve (MTTR)</span>
            <div className="p-1.5 rounded-md bg-amber-100 text-amber-800">
              <Clock className="w-4 h-4" />
            </div>
          </div>
          <div className="text-[26px] font-bold font-mono-financial text-[var(--text-primary)]">
            {kpi.meanTimeToResolveMinutes} <span className="text-[16px] font-normal">mins</span>
          </div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Industry baseline: 48 to 72 hours. Accelerated 98.2% faster.
          </div>
        </div>

        {/* Card 5: Penalties Avoided */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <span>Regulatory Penalties Avoided</span>
            <div className="p-1.5 rounded-md bg-purple-100 text-purple-800">
              <Award className="w-4 h-4" />
            </div>
          </div>
          <div className="text-[26px] font-bold font-mono-financial text-purple-700">
            {kpi.regulatoryPenaltiesAvoidedNgn}
          </div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Saved based on CBN ₦10,000/day statutory late reversal fine schedule.
          </div>
        </div>

        {/* Card 6: Customer Retention Impact */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <span>Customer Retention Lift</span>
            <div className="p-1.5 rounded-md bg-teal-100 text-teal-800">
              <Users className="w-4 h-4" />
            </div>
          </div>
          <div className="text-[26px] font-bold font-mono-financial text-teal-700">
            +{kpi.customerRetentionImpactPct}%
          </div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Reduction in customer churn following fast, transparent dispute resolution.
          </div>
        </div>
      </div>

      {/* Trajectory & Volume Distribution Visual */}
      <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
        <div className="flex items-center justify-between pb-2 border-b border-[var(--surface-border-subtle)]">
          <span className="font-bold text-[14px] text-[var(--text-primary)] flex items-center gap-2">
            <BarChart3 className="w-4 h-4 text-indigo-600" />
            <span>Channel Resolution Volume & Velocity</span>
          </span>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            Current Quarter 2026
          </span>
        </div>

        <div className="space-y-4 text-[12px] font-mono-financial">
          <div>
            <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
              <span>NIP Instant Bank Transfers (₦342.8M Reconciled)</span>
              <span className="text-emerald-700 font-bold">99.63% Resolution Rate</span>
            </div>
            <div className="w-full bg-[var(--canvas-subdued)] h-3 rounded-full overflow-hidden">
              <div className="bg-emerald-600 h-full rounded-full" style={{ width: '99.63%' }} />
            </div>
          </div>

          <div>
            <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
              <span>POS Acquiring & Terminal Debits (₦41.2M Reconciled)</span>
              <span className="text-emerald-700 font-bold">99.71% Resolution Rate</span>
            </div>
            <div className="w-full bg-[var(--canvas-subdued)] h-3 rounded-full overflow-hidden">
              <div className="bg-blue-600 h-full rounded-full" style={{ width: '99.71%' }} />
            </div>
          </div>

          <div>
            <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
              <span>ATM Cash Dispense Errors (₦61.2M Reconciled)</span>
              <span className="text-emerald-700 font-bold">99.64% Resolution Rate</span>
            </div>
            <div className="w-full bg-[var(--canvas-subdued)] h-3 rounded-full overflow-hidden">
              <div className="bg-indigo-600 h-full rounded-full" style={{ width: '99.64%' }} />
            </div>
          </div>

          <div>
            <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
              <span>Cross-Border & Card Base II (₦18.4M Reconciled)</span>
              <span className="text-emerald-700 font-bold">99.79% Resolution Rate</span>
            </div>
            <div className="w-full bg-[var(--canvas-subdued)] h-3 rounded-full overflow-hidden">
              <div className="bg-purple-600 h-full rounded-full" style={{ width: '99.79%' }} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
