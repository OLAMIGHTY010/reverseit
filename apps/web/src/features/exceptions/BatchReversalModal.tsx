import React, { useState } from 'react';
import {
  Layers,
  AlertTriangle,
  CheckCircle2,
  X,
  ArrowRight,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import { getMakerCheckerPolicy, calculateDisputeAbuseProfile } from '../fraud/DisputeAbuseEngine';
import type { FinancialException, BatchReversalResult, Role } from '../../types/domain';

interface BatchReversalModalProps {
  isOpen: boolean;
  onClose: () => void;
  exceptions: FinancialException[];
  currentRole: Role;
  onExecuteBatch: (result: BatchReversalResult) => Promise<void>;
}

export const BatchReversalModal: React.FC<BatchReversalModalProps> = ({
  isOpen,
  onClose,
  exceptions,
  currentRole,
  onExecuteBatch,
}) => {
  const [isExecuting, setIsExecuting] = useState(false);
  const [completedResult, setCompletedResult] = useState<BatchReversalResult | null>(null);
  const [batchJustification, setBatchJustification] = useState(
    'Batch settlement reversal for verified switch timeout incident (INC-8821). Approved under operational emergency fast-track.'
  );

  if (!isOpen || exceptions.length === 0) return null;

  // Aggregate stats
  const totalExposure = exceptions.reduce((acc, curr) => acc + (parseFloat(curr.amount) || 0), 0);
  const currency = exceptions[0]?.currency || 'NGN';

  // Analyze each item
  const analyzedItems = exceptions.map((exc) => {
    const policy = getMakerCheckerPolicy(exc.amount, exc.currency);
    const abuse = exc.disputeAbuseProfile || calculateDisputeAbuseProfile({
      amount: exc.amount,
      channel: exc.channel,
      repeatContactCount: exc.repeatContactCount,
    });
    const isExclusionRecommended = abuse.score >= 75;
    return {
      exception: exc,
      policy,
      abuse,
      isExclusionRecommended,
    };
  });

  const highRiskCount = analyzedItems.filter((i) => i.isExclusionRecommended).length;
  const eligibleCount = analyzedItems.length - highRiskCount;

  const handleConfirmBatch = async () => {
    setIsExecuting(true);
    try {
      const batchId = `BATCH-${Date.now().toString(36).toUpperCase()}-${Math.floor(1000 + Math.random() * 9000)}`;
      const result: BatchReversalResult = {
        batchId,
        dispatchedAt: new Date().toISOString(),
        totalCount: exceptions.length,
        successfulCount: eligibleCount,
        skippedCount: highRiskCount,
        totalExposure: totalExposure.toFixed(2),
        currency,
        makerId: `Analyst (${currentRole})`,
        checkerId: currentRole === 'SUPERVISOR_CHECKER' ? 'Supervisor Dual-Control Authorized' : undefined,
        glPostingRef: `GL-BATCH-RECON-${batchId}`,
        itemReferences: exceptions.map((e) => e.reference),
      };

      await onExecuteBatch(result);
      setCompletedResult(result);
    } finally {
      setIsExecuting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="batch-reversal-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-2xl w-full overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-md bg-sky-100 text-sky-900 dark:bg-sky-950 dark:text-sky-300">
              <Layers className="w-5 h-5" />
            </div>
            <div>
              <h3 id="batch-reversal-title" className="text-[15px] font-bold text-[var(--text-primary)]">
                Batch Reversal Processing & Settlement Dispatch
              </h3>
              <p className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                {exceptions.length} exceptions selected • Total Volume: {currency} {totalExposure.toLocaleString('en-US', { minimumFractionDigits: 2 })}
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-raised)]"
            aria-label="Close modal"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-5 text-[13px] overflow-y-auto">
          {completedResult ? (
            /* Success Manifest */
            <div className="space-y-4">
              <div className="p-4 bg-emerald-50 border border-emerald-300 rounded-md text-emerald-950 flex items-start gap-3">
                <CheckCircle2 className="w-6 h-6 text-emerald-600 shrink-0 mt-0.5" />
                <div className="space-y-1">
                  <h4 className="font-bold text-[14px]">Batch Reversal Dispatched Successfully</h4>
                  <p className="text-[12px] opacity-90 leading-relaxed">
                    Batch <strong>{completedResult.batchId}</strong> has been executed. General ledger postings generated and switch reversal packets dispatched.
                  </p>
                </div>
              </div>

              <div className="p-4 rounded-md border border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] space-y-2.5 font-mono-financial text-[12px]">
                <div className="flex justify-between py-1 border-b border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)]">Batch Identifier:</span>
                  <span className="font-bold text-[var(--text-primary)]">{completedResult.batchId}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)]">Core GL Posting Reference:</span>
                  <span className="font-bold text-sky-700">{completedResult.glPostingRef}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)]">Processed Reversals:</span>
                  <span className="font-bold text-emerald-700">{completedResult.successfulCount} / {completedResult.totalCount} accounts</span>
                </div>
                <div className="flex justify-between py-1 border-b border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)]">Total Dispatched Exposure:</span>
                  <span className="font-bold text-[var(--text-primary)]">
                    {completedResult.currency} {Number(completedResult.totalExposure).toLocaleString('en-US', { minimumFractionDigits: 2 })}
                  </span>
                </div>
              </div>
            </div>
          ) : (
            /* Configuration & Review */
            <>
              {/* Aggregate Exposure Box */}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <div className="p-3.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">
                  <span className="text-[10px] font-bold uppercase text-[var(--text-muted)] block">
                    Selected Items
                  </span>
                  <span className="text-[20px] font-extrabold font-mono-financial text-[var(--text-primary)]">
                    {exceptions.length}
                  </span>
                </div>

                <div className="p-3.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">
                  <span className="text-[10px] font-bold uppercase text-[var(--text-muted)] block">
                    Total Volume
                  </span>
                  <span className="text-[18px] font-extrabold font-mono-financial text-[var(--text-primary)]">
                    {currency} {totalExposure.toLocaleString('en-US', { minimumFractionDigits: 2 })}
                  </span>
                </div>

                <div className="p-3.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">
                  <span className="text-[10px] font-bold uppercase text-[var(--text-muted)] block">
                    High Risk / Excluded
                  </span>
                  <span
                    className={clsx(
                      'text-[20px] font-extrabold font-mono-financial',
                      highRiskCount > 0 ? 'text-rose-600' : 'text-emerald-600'
                    )}
                  >
                    {highRiskCount}
                  </span>
                </div>
              </div>

              {/* High Risk Warning Banner */}
              {highRiskCount > 0 && (
                <div className="p-3 bg-amber-50 border border-amber-300 rounded text-amber-950 text-[12px] flex items-start gap-2">
                  <AlertTriangle className="w-4 h-4 text-amber-600 shrink-0 mt-0.5" />
                  <div>
                    <strong className="block font-bold">Automatic Risk Segregation:</strong>
                    {highRiskCount} item{highRiskCount > 1 ? 's' : ''} have high dispute abuse scores (≥75) and will be held for individual forensic review to prevent friendly fraud leakage.
                  </div>
                </div>
              )}

              {/* Items List Preview */}
              <div className="space-y-2">
                <h4 className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
                  Batch Candidate Manifest ({exceptions.length})
                </h4>
                <div className="max-h-48 overflow-y-auto border border-[var(--surface-border)] rounded divide-y divide-[var(--surface-border-subtle)] text-[12px] font-mono-financial">
                  {analyzedItems.map(({ exception, policy, abuse, isExclusionRecommended }) => (
                    <div
                      key={exception.id}
                      className={clsx(
                        'p-2.5 flex items-center justify-between',
                        isExclusionRecommended ? 'bg-rose-50/50 opacity-75' : 'bg-[var(--surface)]'
                      )}
                    >
                      <div>
                        <span className="font-bold text-[var(--text-primary)]">{exception.reference}</span>
                        <span className="text-[var(--text-muted)] ml-2 text-[11px]">
                          {exception.channel} • {exception.accountNumber || '•••• ' + (exception.maskedPan?.slice(-4) || '9901')}
                        </span>
                      </div>
                      <div className="flex items-center gap-3">
                        <span className="font-bold">
                          {exception.currency} {Number(exception.amount).toLocaleString('en-US', { minimumFractionDigits: 2 })}
                        </span>
                        <span className="text-[10px] px-1.5 py-0.5 rounded font-mono font-bold bg-zinc-100 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300">
                          Abuse {abuse.score}
                        </span>
                        <span
                          className={clsx(
                            'text-[9px] uppercase px-1.5 py-0.2 rounded font-sans font-extrabold',
                            isExclusionRecommended
                              ? 'bg-rose-100 text-rose-900 border border-rose-300'
                              : 'bg-emerald-100 text-emerald-900 border border-emerald-300'
                          )}
                        >
                          {isExclusionRecommended ? 'HOLD / HIGH ABUSE' : policy.tier.replace(/_/g, ' ')}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Justification Note */}
              <div>
                <label className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1">
                  Batch Audit Justification
                </label>
                <textarea
                  rows={2}
                  value={batchJustification}
                  onChange={(e) => setBatchJustification(e.target.value)}
                  className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:ring-1 focus:ring-sky-500"
                />
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-3 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <Button variant="ghost" size="sm" onClick={onClose} disabled={isExecuting}>
            {completedResult ? 'Close' : 'Cancel'}
          </Button>

          {!completedResult && (
            <Button
              variant="primary"
              size="sm"
              isLoading={isExecuting}
              onClick={handleConfirmBatch}
              rightIcon={<ArrowRight className="w-4 h-4" />}
            >
              Execute Batch Reversal ({eligibleCount} Items)
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};
