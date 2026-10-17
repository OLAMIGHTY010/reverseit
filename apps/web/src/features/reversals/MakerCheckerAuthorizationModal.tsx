import React, { useState } from 'react';
import {
  ShieldAlert,
  KeyRound,
  X,
  Lock,
  ArrowRight,
  FileCheck2,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import { getMakerCheckerPolicy } from '../fraud/DisputeAbuseEngine';
import type { FinancialException, ReversalProposal, Role } from '../../types/domain';

interface MakerCheckerAuthorizationModalProps {
  isOpen: boolean;
  onClose: () => void;
  exception: FinancialException | null;
  proposal?: ReversalProposal | null;
  currentRole: Role;
  currentUserId: string;
  onConfirmAuthorization: (params: {
    decision: 'APPROVE' | 'REJECT';
    supervisorPin: string;
    authSignatureHash: string;
    notes: string;
  }) => Promise<void>;
}

export const MakerCheckerAuthorizationModal: React.FC<MakerCheckerAuthorizationModalProps> = ({
  isOpen,
  onClose,
  exception,
  proposal,
  currentRole,
  currentUserId,
  onConfirmAuthorization,
}) => {
  const [supervisorPin, setSupervisorPin] = useState('8842');
  const [notes, setNotes] = useState('Switch TSQ confirmed failed settlement session. GL re-credit verified compliant with CBN 24-hr mandate.');
  const [isAcknowledged, setIsAcknowledged] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!isOpen || !exception) return null;

  const numAmount = parseFloat(exception.amount) || 0;
  const policy = getMakerCheckerPolicy(numAmount, exception.currency);

  const makerName = proposal?.proposedBy || exception.assignedTo || 'Olusola Adeyemi (Maker)';
  const isMakerSelfApproving =
    policy.requiresChecker &&
    (currentUserId.toLowerCase().includes('maker') || currentRole === 'ANALYST_MAKER') &&
    makerName.toLowerCase().includes(currentUserId.toLowerCase());

  const isCheckerPermitted =
    currentRole === 'SUPERVISOR_CHECKER' || currentRole === 'ADMIN' || !policy.requiresChecker;

  // Generate deterministic mock verification hash
  const authSignatureHash = `SHA256:SIG_${exception.reference}_${Date.now().toString(16).toUpperCase()}_0x89F`;

  const handleSubmit = async (decision: 'APPROVE' | 'REJECT') => {
    if (decision === 'APPROVE') {
      if (policy.requiresChecker && supervisorPin.length < 4) {
        setError('Please enter your 4-digit Supervisor Authorization PIN.');
        return;
      }
      if (!isAcknowledged) {
        setError('You must acknowledge that this ledger reversal credit is final and irreversible.');
        return;
      }
    }

    setError(null);
    setIsSubmitting(true);
    try {
      await onConfirmAuthorization({
        decision,
        supervisorPin,
        authSignatureHash,
        notes,
      });
      onClose();
    } catch (err: unknown) {
      setError((err as Error)?.message || 'Authorization failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="maker-checker-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-xl w-full overflow-hidden flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-3">
            <div
              className={clsx(
                'p-2 rounded-md',
                policy.tier === 'TIER_3_DUAL_CONTROL'
                  ? 'bg-purple-100 text-purple-900 dark:bg-purple-950 dark:text-purple-300'
                  : 'bg-sky-100 text-sky-900 dark:bg-sky-950 dark:text-sky-300'
              )}
            >
              <KeyRound className="w-5 h-5" />
            </div>
            <div>
              <h3 id="maker-checker-title" className="text-[15px] font-bold text-[var(--text-primary)]">
                Tiered Maker-Checker Reversal Authorization
              </h3>
              <p className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                Mandate: {policy.tier.replace(/_/g, ' ')} • Threshold: {policy.policyDescription.split(':')[0]}
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

        {/* Form Body */}
        <div className="p-6 space-y-5 text-[13px] overflow-y-auto max-h-[80vh]">
          {/* Target Amount & Tier Badge */}
          <div className="p-4 rounded-md border border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
            <div>
              <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)] block">
                Reversal Credit Amount
              </span>
              <div className="text-[24px] font-extrabold font-mono-financial text-[var(--text-primary)]">
                {exception.currency} {Number(exception.amount).toLocaleString('en-US', { minimumFractionDigits: 2 })}
              </div>
              <span className="text-[11px] text-[var(--text-muted)] font-mono-financial">
                Target Account: {exception.accountNumber || '•••• ' + (exception.maskedPan?.slice(-4) || '9901')}
              </span>
            </div>

            <div className="text-right">
              <span
                className={clsx(
                  'inline-block px-2.5 py-1 rounded border text-[11px] font-bold font-mono-financial uppercase',
                  policy.tier === 'TIER_3_DUAL_CONTROL'
                    ? 'bg-purple-50 text-purple-900 border-purple-300'
                    : 'bg-sky-50 text-sky-900 border-sky-300'
                )}
              >
                {policy.tier.replace(/_/g, ' ')}
              </span>
              <span className="text-[10px] text-[var(--text-muted)] block mt-1">
                {policy.requiresChecker ? 'Dual-Control Sign-Off Required' : 'Single Officer Sign-Off'}
              </span>
            </div>
          </div>

          {/* Maker Proposal Summary */}
          <div className="p-3.5 rounded border border-[var(--surface-border-subtle)] bg-[var(--surface)] space-y-2">
            <div className="flex items-center justify-between text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">
              <span>Originating Maker Step</span>
              <span className="text-emerald-700 font-bold">✓ Proposed & Validated</span>
            </div>
            <div className="grid grid-cols-2 gap-2 text-[12px] font-mono-financial">
              <div>
                <span className="text-[var(--text-muted)] block text-[10px]">Maker Analyst:</span>
                <span className="font-bold text-[var(--text-primary)]">{makerName}</span>
              </div>
              <div>
                <span className="text-[var(--text-muted)] block text-[10px]">Reference:</span>
                <span className="font-bold text-[var(--text-primary)]">{exception.reference}</span>
              </div>
            </div>
          </div>

          {/* Maker != Checker Constraint Check */}
          {isMakerSelfApproving && (
            <div className="p-3 bg-rose-50 border border-rose-300 rounded text-rose-950 text-[12px] flex items-start gap-2">
              <ShieldAlert className="w-4 h-4 text-rose-600 shrink-0 mt-0.5" />
              <div>
                <strong className="block font-bold">Maker != Checker Constraint Enforced:</strong>
                As the originating Maker ({makerName}), you cannot authorize this Tier 3 transaction. A second supervisor checker must log in to counter-sign.
              </div>
            </div>
          )}

          {!isCheckerPermitted && (
            <div className="p-3 bg-amber-50 border border-amber-300 rounded text-amber-950 text-[12px] flex items-start gap-2">
              <ShieldAlert className="w-4 h-4 text-amber-600 shrink-0 mt-0.5" />
              <div>
                <strong className="block font-bold">Checker Authority Required:</strong>
                Current active role ({currentRole}) does not have Checker authorization permissions for this tier.
              </div>
            </div>
          )}

          {/* Checker Sign-Off Section */}
          <div className="space-y-3 pt-2">
            <h4 className="text-[12px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
              <FileCheck2 className="w-4 h-4 text-sky-600" /> Checker Counter-Signature & Audit Credentials
            </h4>

            {policy.requiresChecker && (
              <div>
                <label className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1">
                  Supervisor Authorization PIN / Security Token
                </label>
                <div className="relative">
                  <input
                    type="password"
                    maxLength={6}
                    value={supervisorPin}
                    onChange={(e) => setSupervisorPin(e.target.value)}
                    placeholder="Enter 4 or 6-digit PIN"
                    className="w-full px-3 py-2 pl-9 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[14px] font-mono-financial tracking-widest text-[var(--text-primary)] focus:ring-1 focus:ring-sky-500"
                  />
                  <Lock className="w-4 h-4 text-[var(--text-muted)] absolute left-3 top-2.5" />
                </div>
                <span className="text-[10px] text-[var(--text-muted)] mt-1 block">
                  Simulated Supervisor PIN: <code>8842</code> (Hardware Token / OTP verified)
                </span>
              </div>
            )}

            <div>
              <label className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1">
                Checker Approval Operational Justification
              </label>
              <textarea
                rows={2}
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:ring-1 focus:ring-sky-500"
              />
            </div>

            {/* Irreversible Acknowledgment Checkbox */}
            <label className="flex items-start gap-2.5 p-3 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] cursor-pointer">
              <input
                type="checkbox"
                checked={isAcknowledged}
                onChange={(e) => setIsAcknowledged(e.target.checked)}
                className="mt-0.5 rounded border-slate-300 text-sky-600 focus:ring-sky-500 cursor-pointer"
              />
              <span className="text-[11px] text-[var(--text-secondary)] leading-relaxed">
                I counter-sign that the 8 pre-flight financial checks have passed and authorize immediate general ledger posting to restore customer funds.
              </span>
            </label>

            {error && (
              <p className="text-[12px] text-rose-600 font-semibold">{error}</p>
            )}
          </div>
        </div>

        {/* Footer Actions */}
        <div className="px-6 py-3 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <Button
            variant="ghost"
            size="sm"
            onClick={onClose}
            disabled={isSubmitting}
          >
            Cancel
          </Button>

          <div className="flex items-center gap-2">
            <Button
              variant="danger"
              size="sm"
              onClick={() => handleSubmit('REJECT')}
              disabled={isSubmitting}
            >
              Reject Proposal
            </Button>

            <Button
              variant="primary"
              size="sm"
              isLoading={isSubmitting}
              disabled={isMakerSelfApproving || !isAcknowledged || !isCheckerPermitted}
              onClick={() => handleSubmit('APPROVE')}
              rightIcon={<ArrowRight className="w-4 h-4" />}
            >
              {policy.requiresChecker ? 'Counter-Sign & Dispatch' : 'Authorize Reversal'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
