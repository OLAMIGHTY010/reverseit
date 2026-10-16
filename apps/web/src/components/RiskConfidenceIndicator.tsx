import React from 'react';
import { Sparkles, ShieldCheck, AlertTriangle } from 'lucide-react';
import clsx from 'clsx';

export interface RiskConfidenceIndicatorProps {
  modelConfidence?: number; // 0 to 100
  isVerified?: boolean;
  className?: string;
  showDetails?: boolean;
}

export const RiskConfidenceIndicator: React.FC<RiskConfidenceIndicatorProps> = ({
  modelConfidence,
  isVerified = false,
  className,
  showDetails = false,
}) => {
  return (
    <div className={clsx('inline-flex items-center gap-2', className)}>
      {isVerified ? (
        <span
          className="inline-flex items-center gap-1.5 px-2 py-0.5 text-[11px] font-semibold bg-emerald-50 text-emerald-900 border border-emerald-300 rounded"
          title="Cryptographically sealed in immutable core ledger and switch logs"
        >
          <ShieldCheck className="w-3.5 h-3.5 text-emerald-700" aria-hidden="true" />
          <span>Verified Audit Provenance</span>
        </span>
      ) : modelConfidence !== undefined ? (
        <span
          className={clsx(
            'inline-flex items-center gap-1.5 px-2 py-0.5 text-[11px] font-medium border rounded',
            modelConfidence >= 90
              ? 'bg-sky-50 text-sky-950 border-sky-300'
              : modelConfidence >= 75
              ? 'bg-amber-50 text-amber-950 border-amber-300'
              : 'bg-rose-50 text-rose-950 border-rose-300'
          )}
          title="Inferred by AI Anomaly Detection Engine (Probabilistic)"
        >
          <Sparkles className="w-3 h-3 text-sky-700" aria-hidden="true" />
          <span className="font-semibold text-slate-700">Model Confidence:</span>
          <span className="font-mono-financial font-bold">{modelConfidence}%</span>
          {showDetails && (
            <span className="text-[10px] text-slate-500 italic">(Heuristic Match)</span>
          )}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1 text-[11px] text-[var(--text-muted)]">
          <AlertTriangle className="w-3 h-3 text-slate-400" aria-hidden="true" />
          <span>Unassessed</span>
        </span>
      )}
    </div>
  );
};
