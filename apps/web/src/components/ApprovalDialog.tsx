import React, { useState } from 'react';
import { ShieldCheck, ShieldAlert, AlertTriangle, X } from 'lucide-react';
import { Button } from './Button';
import { StatusBadge } from './StatusBadge';
import { useApp } from '../context/AppContext';
import type { ReversalProposal } from '../types/domain';

export interface ApprovalDialogProps {
  proposal: ReversalProposal | null;
  isOpen: boolean;
  onClose: () => void;
  onAuthorize: (proposalId: string, decision: 'APPROVE' | 'REJECT', notes?: string) => Promise<void>;
}

export const ApprovalDialog: React.FC<ApprovalDialogProps> = ({
  proposal,
  isOpen,
  onClose,
  onAuthorize,
}) => {
  const { role } = useApp();
  const [notes, setNotes] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen || !proposal) return null;

  const isChecker = role === 'SUPERVISOR_CHECKER' || role === 'ADMIN';

  const handleDecision = async (decision: 'APPROVE' | 'REJECT') => {
    setIsSubmitting(true);
    try {
      await onAuthorize(proposal.id, decision, notes);
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs"
      role="dialog"
      aria-modal="true"
      aria-labelledby="approval-dialog-title"
    >
      <div className="w-full max-w-lg bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xl overflow-hidden animate-in fade-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="px-5 py-4 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="p-1 rounded bg-purple-100 text-purple-900">
              <ShieldAlert className="w-4 h-4" />
            </span>
            <h2 id="approval-dialog-title" className="text-[15px] font-semibold text-[var(--text-primary)]">
              Maker-Checker Dual Authorization
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded cursor-pointer"
            aria-label="Close dialog"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content */}
        <div className="p-5 space-y-4 text-[13px]">
          {/* Amount & Reference Highlight */}
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md flex items-center justify-between">
            <div>
              <div className="text-[11px] text-[var(--text-muted)] font-medium">Reversal Amount</div>
              <div className="text-[20px] font-bold font-mono-financial text-[var(--text-primary)]">
                {proposal.currency} {Number(proposal.reversalAmount).toLocaleString('en-US', { minimumFractionDigits: 2 })}
              </div>
            </div>
            <StatusBadge status="approval" />
          </div>

          {/* Proposal Details Table */}
          <div className="grid grid-cols-2 gap-2 text-[12px] border border-[var(--surface-border)] rounded-md p-3 divide-y divide-[var(--surface-border-subtle)] col-span-2">
            <div className="col-span-2 pb-1.5 flex justify-between">
              <span className="text-[var(--text-muted)]">Transaction Ref:</span>
              <strong className="font-mono-financial text-[var(--text-primary)]">{proposal.reference}</strong>
            </div>
            <div className="col-span-2 py-1.5 flex justify-between">
              <span className="text-[var(--text-muted)]">Channel:</span>
              <span className="font-semibold">{proposal.channel}</span>
            </div>
            <div className="col-span-2 py-1.5 flex justify-between">
              <span className="text-[var(--text-muted)]">Destination Account:</span>
              <span className="font-mono-financial">{proposal.destinationAccount} ({proposal.destinationBank})</span>
            </div>
            <div className="col-span-2 py-1.5 flex justify-between">
              <span className="text-[var(--text-muted)]">Drafted By Maker:</span>
              <span className="font-medium text-[var(--text-primary)]">{proposal.proposedBy}</span>
            </div>
            <div className="col-span-2 pt-1.5">
              <span className="text-[var(--text-muted)] block mb-0.5">Maker Justification:</span>
              <p className="text-[var(--text-secondary)] bg-[var(--surface-raised)] p-2 rounded border border-[var(--surface-border-subtle)] text-[12px] italic">
                "{proposal.justification}"
              </p>
            </div>
          </div>

          {/* Checker Notes Input */}
          <div>
            <label htmlFor="checker-notes" className="block text-[12px] font-medium text-[var(--text-secondary)] mb-1">
              Checker Evaluation Notes:
            </label>
            <textarea
              id="checker-notes"
              rows={2}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Provide review observations or reasons for decision..."
              className="w-full px-3 py-1.5 text-[12px] bg-[var(--surface)] text-[var(--text-primary)] border border-[var(--surface-border)] rounded focus:outline-2 focus:outline-[var(--focus-ring)]"
            />
          </div>

          {/* Role Permission Alert */}
          {!isChecker && (
            <div className="p-2.5 bg-amber-50 border border-amber-300 rounded text-amber-950 text-[12px] flex items-start gap-2">
              <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
              <div>
                <strong>Dual-Control Authorization Required:</strong> Your current role is{' '}
                <span className="font-mono-financial font-semibold">{role}</span>. Only an authorized Supervisor Checker can execute this reversal. Switch role via header to approve.
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-end gap-2.5">
          <Button variant="ghost" size="md" onClick={onClose} disabled={isSubmitting}>
            Cancel
          </Button>
          {isChecker && (
            <Button
              variant="danger"
              size="md"
              isLoading={isSubmitting}
              onClick={() => handleDecision('REJECT')}
            >
              Reject Proposal
            </Button>
          )}
          <Button
            variant="primary"
            size="md"
            disabled={!isChecker}
            isLoading={isSubmitting}
            leftIcon={<ShieldCheck className="w-4 h-4" />}
            onClick={() => handleDecision('APPROVE')}
          >
            Authorize & Dispatch
          </Button>
        </div>
      </div>
    </div>
  );
};
