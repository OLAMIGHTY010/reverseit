import React, { useState } from 'react';
import {
  Copy,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
  ShieldAlert,
  HelpCircle,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import type { DuplicateComparisonRecord } from '../../types/domain';

interface DuplicateComparisonViewProps {
  comparison: DuplicateComparisonRecord;
  onConfirmDuplicate: () => void;
  onMarkLegitimateRepeat: (reason: string) => void;
  onRequestMoreEvidence: () => void;
}

export const DuplicateComparisonView: React.FC<DuplicateComparisonViewProps> = ({
  comparison,
  onConfirmDuplicate,
  onMarkLegitimateRepeat,
  onRequestMoreEvidence,
}) => {
  const [isLegitimateModalOpen, setIsLegitimateModalOpen] = useState(false);
  const [legitimateReason, setLegitimateReason] = useState('');

  const riskBadgeClass =
    comparison.riskGrade === 'HIGH'
      ? 'bg-rose-100 text-rose-900 border-rose-300'
      : comparison.riskGrade === 'MEDIUM'
      ? 'bg-amber-100 text-amber-900 border-amber-300'
      : 'bg-emerald-100 text-emerald-900 border-emerald-300';

  const handleLegitimateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!legitimateReason.trim()) return;
    onMarkLegitimateRepeat(legitimateReason.trim());
    setIsLegitimateModalOpen(false);
  };

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)] text-[13px]">
      {/* Header with Risk Grade & Timing Delta */}
      <div className="p-4 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="p-1 rounded bg-amber-100 text-amber-800">
              <Copy className="w-4 h-4" />
            </span>
            <h3 className="font-bold text-[14px] text-[var(--text-primary)]">
              Temporal Duplicate Detection Matrix
            </h3>
            <span
              className={clsx(
                'px-2 py-0.5 rounded font-mono-financial font-extrabold text-[11px] border',
                riskBadgeClass
              )}
            >
              DUPLICATE RISK: {comparison.riskGrade} ({comparison.riskScore}/100)
            </span>
          </div>
          <p className="text-[12px] text-[var(--text-muted)]">
            Side-by-side ledger posting comparison comparing client idempotency, STAN trace, and terminal journals.
          </p>
        </div>

        {/* Interval Delta Badge */}
        <div className="flex items-center gap-2 p-2 rounded bg-amber-50 border border-amber-200 text-amber-900 font-mono-financial text-[12px]">
          <Clock className="w-4 h-4 text-amber-700" />
          <span>Timing Delta: <strong>+{comparison.intervalMs}ms</strong> sub-second interval</span>
        </div>
      </div>

      {/* Risk Scoring Reasons (Scope v2 Mandatory) */}
      <div className="p-3.5 bg-amber-50/40 border-b border-amber-200/60 text-[12px] space-y-1.5">
        <div className="font-semibold text-amber-950 flex items-center gap-1.5">
          <AlertTriangle className="w-3.5 h-3.5 text-amber-700" />
          <span>Algorithmic Matching Factors ({comparison.reasons.length} criteria matched)</span>
        </div>
        <ul className="grid grid-cols-1 md:grid-cols-2 gap-1 pl-5 list-disc text-amber-900">
          {comparison.reasons.map((r, i) => (
            <li key={i} className="leading-tight">{r}</li>
          ))}
        </ul>
      </div>

      {/* Side-by-Side Comparison Table */}
      <div className="overflow-x-auto p-4">
        <table className="w-full text-left text-[12px] border border-[var(--surface-border)] rounded-md">
          <thead className="bg-[var(--canvas-subdued)] border-b border-[var(--surface-border)] text-[11px] font-bold uppercase tracking-wider text-[var(--text-secondary)]">
            <tr>
              <th scope="col" className="px-3 py-2 w-1/4">Field / Property</th>
              <th scope="col" className="px-3 py-2 w-1/3">Original Transaction (Record A)</th>
              <th scope="col" className="px-3 py-2 w-1/3">Suspected Duplicate (Record B)</th>
              <th scope="col" className="px-3 py-2 text-center w-16">Match</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--surface-border-subtle)] font-mono-financial">
            {comparison.fieldDiffs.map((diff) => (
              <tr
                key={diff.field}
                className={clsx(
                  'transition-colors',
                  diff.isMatch ? 'bg-[var(--surface)]' : 'bg-amber-50/50'
                )}
              >
                <td className="px-3 py-2 font-sans font-semibold text-[var(--text-primary)]">
                  {diff.label}
                </td>
                <td className="px-3 py-2 text-[var(--text-primary)]">
                  {diff.originalVal}
                </td>
                <td
                  className={clsx(
                    'px-3 py-2',
                    diff.isMatch
                      ? 'text-[var(--text-primary)] font-bold text-sky-950'
                      : 'text-amber-900 font-semibold'
                  )}
                >
                  {diff.suspectVal}
                </td>
                <td className="px-3 py-2 text-center">
                  {diff.isMatch ? (
                    <span
                      className="inline-flex items-center justify-center p-1 rounded-full bg-emerald-100 text-emerald-800"
                      title="Exact Match"
                    >
                      <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" />
                    </span>
                  ) : (
                    <span
                      className="inline-flex items-center justify-center p-1 rounded-full bg-amber-100 text-amber-800"
                      title="Field Value Differs"
                    >
                      <XCircle className="w-3.5 h-3.5 text-amber-600" />
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Verdict Panel & Decision Actions */}
      <div className="p-4 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-4">
        <div className="space-y-1 max-w-xl">
          <div className="flex items-center gap-2">
            <span className="font-bold text-[13px] text-[var(--text-primary)]">
              Automated Rule Verdict:
            </span>
            <span className="px-2 py-0.5 rounded font-mono-financial font-extrabold text-[11px] bg-rose-100 text-rose-900 border border-rose-300">
              {comparison.ruleOutcome.replace(/_/g, ' ')}
            </span>
          </div>
          <p className="text-[12px] text-[var(--text-secondary)] leading-relaxed">
            {comparison.verdictReason}
          </p>
        </div>

        {/* Action Buttons */}
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="ghost"
            onClick={onRequestMoreEvidence}
            leftIcon={<HelpCircle className="w-4 h-4" />}
          >
            Request More Evidence
          </Button>

          <Button
            size="sm"
            variant="secondary"
            onClick={() => setIsLegitimateModalOpen(true)}
          >
            Mark Legitimate Repeat
          </Button>

          <Button
            size="sm"
            variant="danger"
            onClick={onConfirmDuplicate}
            leftIcon={<ShieldAlert className="w-4 h-4" />}
          >
            Confirm Duplicate (Reverse Leg B)
          </Button>
        </div>
      </div>

      {/* Legitimate Repeat Modal */}
      {isLegitimateModalOpen && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="legitimate-modal-title"
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs animate-in fade-in duration-150"
        >
          <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-md w-full p-5 space-y-4">
            <h4 id="legitimate-modal-title" className="font-bold text-[14px] text-[var(--text-primary)]">
              Mark Transaction as Legitimate Repeat Purchase
            </h4>
            <p className="text-[12px] text-[var(--text-secondary)]">
              Please document why this sub-second second transaction is legitimate (e.g. customer bought two identical gift cards or tapped for two separate transit tickets).
            </p>

            <form onSubmit={handleLegitimateSubmit} className="space-y-3">
              <textarea
                rows={3}
                value={legitimateReason}
                onChange={(e) => setLegitimateReason(e.target.value)}
                placeholder="State merchant receipt proof, customer statement, or multi-basket confirmation..."
                className="w-full px-3 py-2 text-[12px] bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500"
              />

              <div className="flex items-center justify-end gap-2 pt-2 border-t border-[var(--surface-border-subtle)]">
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setIsLegitimateModalOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  size="sm"
                  variant="primary"
                  disabled={!legitimateReason.trim()}
                >
                  Save Exemption
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
