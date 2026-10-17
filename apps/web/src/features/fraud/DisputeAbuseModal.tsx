import React, { useState } from 'react';
import {
  ShieldAlert,
  AlertTriangle,
  X,
  UserX,
  CheckCircle2,
  Activity,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import type { DisputeAbuseProfile, FinancialException } from '../../types/domain';

interface DisputeAbuseModalProps {
  isOpen: boolean;
  onClose: () => void;
  exception: FinancialException | null;
  profile: DisputeAbuseProfile;
  onAddAuditNote?: (note: string) => void;
  onPlaceOnWatchlist?: (account: string) => void;
}

export const DisputeAbuseModal: React.FC<DisputeAbuseModalProps> = ({
  isOpen,
  onClose,
  exception,
  profile,
  onAddAuditNote,
  onPlaceOnWatchlist,
}) => {
  const [isWatchlisted, setIsWatchlisted] = useState(false);

  if (!isOpen || !exception) return null;

  const { score, tier, flags, disputeVelocity7d, disputeVelocity30d, historicalFalseClaimRatePct, recommendation } =
    profile;

  const scoreColor =
    score >= 75 ? 'text-rose-600' : score >= 50 ? 'text-amber-600' : 'text-emerald-600';

  const progressBg =
    score >= 75 ? 'bg-rose-500' : score >= 50 ? 'bg-amber-500' : 'bg-emerald-500';

  const handleWatchlistToggle = () => {
    setIsWatchlisted((prev) => !prev);
    if (onPlaceOnWatchlist && exception.accountNumber) {
      onPlaceOnWatchlist(exception.accountNumber);
    }
    if (onAddAuditNote) {
      onAddAuditNote(
        `[FRAUD_DESK_ACTION] Account ${exception.accountNumber || 'Masked'} flagged for 30-day friendly-fraud monitoring.`
      );
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="dispute-abuse-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-2xl w-full overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-3">
            <div
              className={clsx(
                'p-2 rounded-md',
                score >= 75
                  ? 'bg-rose-100 text-rose-800'
                  : score >= 50
                  ? 'bg-amber-100 text-amber-800'
                  : 'bg-emerald-100 text-emerald-800'
              )}
            >
              <ShieldAlert className="w-5 h-5" />
            </div>
            <div>
              <h3 id="dispute-abuse-title" className="text-[15px] font-bold text-[var(--text-primary)]">
                Dispute Velocity & Friendly Fraud Risk Dossier
              </h3>
              <p className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                Account: {exception.accountNumber || '•••• ' + (exception.maskedPan?.slice(-4) || '9901')} • Case: {exception.reference}
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

        {/* Scrollable Content */}
        <div className="p-6 overflow-y-auto space-y-6 text-[13px]">
          {/* Top Score Meter */}
          <div className="p-5 rounded-lg border border-[var(--surface-border)] bg-[var(--canvas-subdued)] grid grid-cols-1 md:grid-cols-3 gap-4 items-center">
            <div className="md:col-span-1 text-center md:border-r md:border-[var(--surface-border-subtle)] md:pr-4">
              <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block mb-1">
                Dispute Abuse Score
              </span>
              <div className={clsx('text-[36px] font-extrabold font-mono-financial leading-none', scoreColor)}>
                {score}
                <span className="text-[16px] text-[var(--text-muted)] font-normal">/100</span>
              </div>
              <span
                className={clsx(
                  'inline-block mt-2 px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wide',
                  tier === 'HIGH_RISK_RESTRICTED'
                    ? 'bg-rose-100 text-rose-800 border border-rose-300'
                    : tier === 'ELEVATED_WATCH'
                    ? 'bg-amber-100 text-amber-800 border border-amber-300'
                    : 'bg-emerald-100 text-emerald-800 border border-emerald-300'
                )}
              >
                {tier.replace(/_/g, ' ')}
              </span>
            </div>

            <div className="md:col-span-2 space-y-3">
              <div>
                <div className="flex justify-between text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                  <span>Calculated Abuse Risk Severity</span>
                  <span className="font-mono-financial">{score}% Exposure</span>
                </div>
                <div className="w-full h-2.5 bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden">
                  <div
                    className={clsx('h-full transition-all duration-300 rounded-full', progressBg)}
                    style={{ width: `${score}%` }}
                  />
                </div>
              </div>

              <div className="grid grid-cols-3 gap-2 text-[11px] font-mono-financial">
                <div className="p-2 rounded bg-[var(--surface)] border border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)] block text-[10px]">7-Day Velocity:</span>
                  <span className="font-bold text-[13px] text-[var(--text-primary)]">
                    {disputeVelocity7d} claim{disputeVelocity7d > 1 ? 's' : ''}
                  </span>
                </div>
                <div className="p-2 rounded bg-[var(--surface)] border border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)] block text-[10px]">30-Day Velocity:</span>
                  <span className="font-bold text-[13px] text-[var(--text-primary)]">
                    {disputeVelocity30d} claims
                  </span>
                </div>
                <div className="p-2 rounded bg-[var(--surface)] border border-[var(--surface-border-subtle)]">
                  <span className="text-[var(--text-muted)] block text-[10px]">False Claim Rate:</span>
                  <span className="font-bold text-[13px] text-[var(--text-primary)]">
                    {historicalFalseClaimRatePct.toFixed(1)}%
                  </span>
                </div>
              </div>
            </div>
          </div>

          {/* Institutional Recommendation */}
          <div
            className={clsx(
              'p-4 rounded-md border flex items-start gap-3',
              recommendation === 'REJECT_SUSPECTED_FRAUD'
                ? 'bg-rose-50 border-rose-300 text-rose-950'
                : recommendation === 'MANDATORY_IN_BRANCH_VERIFICATION'
                ? 'bg-amber-50 border-amber-300 text-amber-950'
                : 'bg-emerald-50 border-emerald-300 text-emerald-950'
            )}
          >
            <Activity className="w-5 h-5 shrink-0 mt-0.5" />
            <div>
              <span className="text-[11px] font-bold uppercase tracking-wider block">
                Fraud Desk Operational Recommendation
              </span>
              <p className="text-[13px] font-semibold mt-0.5">
                {recommendation === 'REJECT_SUSPECTED_FRAUD' &&
                  'Reject Automated Reversal — Suspected First-Party Abuse / Terminal Confirmed Dispense'}
                {recommendation === 'MANDATORY_IN_BRANCH_VERIFICATION' &&
                  'Require Physical Branch Identification or Counterparty Confirmation before GL Credit'}
                {recommendation === 'STANDARD_OPS_REVIEW' &&
                  'Standard Operations Review — Verified Switch Response Discrepancy'}
                {recommendation === 'AUTO_APPROVE_ELIGIBLE' &&
                  'Auto-Reversal Eligible — Account Demonstrates High Integrity & Low Velocity'}
              </p>
            </div>
          </div>

          {/* Active Fraud Flags */}
          <div className="space-y-2.5">
            <h4 className="text-[12px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
              <ShieldAlert className="w-4 h-4 text-sky-600" /> Active Risk Flags & Signals ({flags.length})
            </h4>

            {flags.length === 0 ? (
              <div className="p-3 bg-emerald-50/50 border border-emerald-200 rounded text-emerald-800 text-[12px] flex items-center gap-2">
                <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                <span>No friendly-fraud indicators detected. Account has clean dispute history.</span>
              </div>
            ) : (
              <div className="space-y-2">
                {flags.map((flag, idx) => (
                  <div
                    key={idx}
                    className="p-3 rounded border border-[var(--surface-border-subtle)] bg-[var(--surface)] flex items-start gap-3"
                  >
                    {flag.severity === 'CRITICAL' ? (
                      <ShieldAlert className="w-4 h-4 text-rose-600 shrink-0 mt-0.5" />
                    ) : (
                      <AlertTriangle className="w-4 h-4 text-amber-600 shrink-0 mt-0.5" />
                    )}
                    <div className="flex-1">
                      <div className="flex items-center justify-between">
                        <span className="font-bold text-[var(--text-primary)]">{flag.title}</span>
                        <span
                          className={clsx(
                            'text-[9px] font-mono-financial font-extrabold uppercase px-1.5 py-0.2 rounded',
                            flag.severity === 'CRITICAL'
                              ? 'bg-rose-100 text-rose-900 border border-rose-300'
                              : 'bg-amber-100 text-amber-900 border border-amber-300'
                          )}
                        >
                          {flag.code}
                        </span>
                      </div>
                      <p className="text-[12px] text-[var(--text-secondary)] mt-0.5 leading-relaxed">
                        {flag.description}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Operations Actions & Watchlist */}
          <div className="p-4 rounded-md border border-[var(--surface-border)] bg-[var(--canvas-subdued)] space-y-3">
            <h4 className="text-[12px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
              Operations Controls & Audit Actions
            </h4>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant={isWatchlisted ? 'danger' : 'secondary'}
                onClick={handleWatchlistToggle}
                leftIcon={<UserX className="w-3.5 h-3.5" />}
              >
                {isWatchlisted ? 'Account Added to Fraud Watchlist ✓' : 'Add to 30-Day Fraud Watchlist'}
              </Button>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-3 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close Dossier
          </Button>
          <Button variant="primary" size="sm" onClick={onClose}>
            Acknowledge & Continue
          </Button>
        </div>
      </div>
    </div>
  );
};
