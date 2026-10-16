import React, { useState } from 'react';
import { X, ArrowRight, ShieldAlert, Check } from 'lucide-react';
import { Button } from '../../components/Button';
import { StatusBadge } from '../../components/StatusBadge';
import type { FinancialStatus, FinancialException } from '../../types/domain';

interface StateChangeModalProps {
  isOpen: boolean;
  exception: FinancialException | null;
  onClose: () => void;
  onSubmit: (newStatus: FinancialStatus, reasonCode: string, justification: string) => Promise<void>;
}

const REASON_CODES: Record<FinancialStatus, Array<{ code: string; label: string }>> = {
  detected: [
    { code: 'INITIAL_TRIAGE', label: 'Initial Automated Triage Flag' },
  ],
  awaiting: [
    { code: 'AWAITING_COUNTERPARTY_REPORT', label: 'Awaiting Counterparty Switch Settlement' },
    { code: 'AWAITING_CUSTOMER_DOCS', label: 'Awaiting Customer Dispute Receipts' },
  ],
  investigating: [
    { code: 'ANALYST_DEEP_DIVE', label: 'Analyst Ledger & Journal Investigation' },
    { code: 'ROOT_CAUSE_ANALYSIS', label: 'Root Cause Diagnostics in Progress' },
  ],
  approval: [
    { code: 'REVERSAL_RECOMMENDED', label: 'Maker Recommendation for Reversal Approval' },
    { code: 'CHECKER_AUTHORIZATION_REQUIRED', label: 'Dual-Control Checker Sign-Off Required' },
  ],
  submitted: [
    { code: 'DISPATCHED_TO_GATEWAY', label: 'Reversal Dispatched to Payment Switch' },
  ],
  verifying: [
    { code: 'SETTLEMENT_RECON_VERIFY', label: 'Verifying Settlement Journal Confirmation' },
  ],
  resolved: [
    { code: 'LEDGER_BALANCED_AND_REVERSED', label: 'Ledger Debits Reconciled & Customer Re-credited' },
    { code: 'LEGITIMATE_TRANSACTION_CONFIRMED', label: 'Transaction Verified Legitimate & Settled' },
  ],
  failed: [
    { code: 'SWITCH_GATEWAY_REJECT', label: 'Switch Gateway Rejection (Error 91/96)' },
    { code: 'DESTINATION_ACCOUNT_BLOCKED', label: 'Beneficiary Account Frozen/Dormant' },
  ],
  suppressed: [
    { code: 'FALSE_POSITIVE_GLITCH', label: 'False Positive Anomaly / Tolerance Match' },
    { code: 'DUPLICATE_INCIDENT_FLAG', label: 'Duplicate Flag Already Triaged Under Master INC' },
  ],
};

const AVAILABLE_TRANSITIONS: Record<FinancialStatus, FinancialStatus[]> = {
  detected: ['investigating', 'approval', 'suppressed'],
  awaiting: ['investigating', 'approval', 'suppressed'],
  investigating: ['approval', 'awaiting', 'resolved', 'suppressed'],
  approval: ['investigating', 'submitted', 'failed'],
  submitted: ['verifying', 'resolved', 'failed'],
  verifying: ['resolved', 'failed'],
  resolved: ['investigating'],
  failed: ['investigating', 'approval', 'suppressed'],
  suppressed: ['investigating'],
};

export const StateChangeModal: React.FC<StateChangeModalProps> = ({
  isOpen,
  exception,
  onClose,
  onSubmit,
}) => {
  const [selectedStatus, setSelectedStatus] = useState<FinancialStatus>('investigating');
  const [reasonCode, setReasonCode] = useState<string>('');
  const [justification, setJustification] = useState<string>('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!isOpen || !exception) return null;

  const validTransitions = AVAILABLE_TRANSITIONS[exception.status] || ['investigating'];
  const activeStatus = validTransitions.includes(selectedStatus) ? selectedStatus : validTransitions[0];
  const availableReasons = REASON_CODES[activeStatus] || [];
  const currentReason = reasonCode || (availableReasons[0]?.code ?? '');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!justification.trim() || justification.trim().length < 8) {
      setError('Please provide a meaningful operational justification (minimum 8 characters).');
      return;
    }
    setError(null);
    setIsSubmitting(true);
    try {
      await onSubmit(activeStatus, currentReason, justification.trim());
      onClose();
    } catch (err: unknown) {
      if ((err as Error)?.message === 'CONCURRENCY_CONFLICT') {
        // Will be caught by parent
      } else {
        setError((err as Error)?.message || 'Failed to update state');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="state-change-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-lg w-full overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <ShieldAlert className="w-4 h-4 text-sky-600" />
            <h3 id="state-change-title" className="text-[14px] font-bold text-[var(--text-primary)]">
              Transition Exception Lifecycle State
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            aria-label="Close state change dialog"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4 text-[13px]">
          {/* Target Exception Meta */}
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded flex items-center justify-between">
            <div>
              <span className="font-mono-financial font-bold text-[var(--text-primary)]">
                {exception.reference}
              </span>
              <p className="text-[11px] text-[var(--text-muted)] mt-0.5">
                {exception.classificationCode} • {exception.amount} {exception.currency}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <StatusBadge status={exception.status} size="sm" />
              <ArrowRight className="w-3.5 h-3.5 text-[var(--text-muted)]" />
              <StatusBadge status={activeStatus} size="sm" />
            </div>
          </div>

          {/* New Status Selection */}
          <div>
            <label className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1.5">
              Select Target Lifecycle Status
            </label>
            <div className="grid grid-cols-3 gap-2">
              {validTransitions.map((target) => (
                <button
                  key={target}
                  type="button"
                  onClick={() => {
                    setSelectedStatus(target);
                    const firstReason = REASON_CODES[target]?.[0]?.code || '';
                    setReasonCode(firstReason);
                  }}
                  className={`p-2 rounded border text-left flex items-center justify-between transition-colors ${
                    activeStatus === target
                      ? 'border-sky-500 bg-sky-50/50 text-sky-950 font-bold'
                      : 'border-[var(--surface-border-subtle)] hover:bg-[var(--surface-raised)] text-[var(--text-secondary)]'
                  }`}
                >
                  <span className="capitalize">{target}</span>
                  {activeStatus === target && <Check className="w-3.5 h-3.5 text-sky-600" />}
                </button>
              ))}
            </div>
          </div>

          {/* Reason Code */}
          <div>
            <label
              htmlFor="reason-code-select"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1.5"
            >
              Mandatory Regulatory Reason Code
            </label>
            <select
              id="reason-code-select"
              value={currentReason}
              onChange={(e) => setReasonCode(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500 font-mono-financial"
            >
              {availableReasons.map((r) => (
                <option key={r.code} value={r.code}>
                  [{r.code}] {r.label}
                </option>
              ))}
            </select>
          </div>

          {/* Justification Textarea */}
          <div>
            <label
              htmlFor="justification-textarea"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1.5"
            >
              Analyst Operational Justification (Logged to Audit Trail)
            </label>
            <textarea
              id="justification-textarea"
              rows={3}
              value={justification}
              onChange={(e) => {
                setJustification(e.target.value);
                if (error) setError(null);
              }}
              placeholder="State ledger evidence, settlement confirmation, or counterparty communications..."
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
            {error && <p className="text-[12px] text-rose-600 mt-1 font-medium">{error}</p>}
          </div>

          <div className="flex items-center justify-end gap-2 pt-3 border-t border-[var(--surface-border-subtle)]">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onClose}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              size="sm"
              isLoading={isSubmitting}
            >
              Confirm Transition
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};
