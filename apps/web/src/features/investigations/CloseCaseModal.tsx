import React, { useState } from 'react';
import { X, CheckCircle2 } from 'lucide-react';
import { Button } from '../../components/Button';

interface CloseCaseModalProps {
  isOpen: boolean;
  caseNumber: string;
  onClose: () => void;
  onSubmit: (resolutionCode: string, notes: string) => Promise<void>;
}

const RESOLUTION_CODES = [
  { code: 'REVERSED_TO_CUSTOMER', label: 'Customer Account Re-credited (Reversal Complete)' },
  { code: 'CONFIRMED_LEGITIMATE_SETTLED', label: 'Transaction Verified Legitimate & Settled at Switch' },
  { code: 'SUSPENSE_BALANCED_AND_CLEARED', label: 'GL Suspense Imbalance Balanced & Reconciled' },
  { code: 'MERCHANT_REPRESENTMENT_HONOURED', label: 'Merchant Representment Verified & Honoured' },
  { code: 'FRAUD_CLAIM_BLOCKED', label: 'Fraud Dispute Denied Following Video/PIN Verification' },
  { code: 'WRITTEN_OFF_TOLERANCE', label: 'Sub-penny Roundoff Settled within Bank Tolerance' },
];

export const CloseCaseModal: React.FC<CloseCaseModalProps> = ({
  isOpen,
  caseNumber,
  onClose,
  onSubmit,
}) => {
  const [resolutionCode, setResolutionCode] = useState(RESOLUTION_CODES[0].code);
  const [notes, setNotes] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!notes.trim()) return;

    setIsSubmitting(true);
    try {
      await onSubmit(resolutionCode, notes.trim());
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="close-modal-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs animate-in fade-in duration-150 text-[13px]"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-md w-full overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="w-4 h-4 text-emerald-600" />
            <h3 id="close-modal-title" className="text-[14px] font-bold text-[var(--text-primary)]">
              Close Case: {caseNumber}
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            aria-label="Close dialog"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          <div>
            <label
              htmlFor="resolution-code-select"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1"
            >
              CBN Regulatory Resolution Code
            </label>
            <select
              id="resolution-code-select"
              value={resolutionCode}
              onChange={(e) => setResolutionCode(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500 font-mono-financial"
            >
              {RESOLUTION_CODES.map((r) => (
                <option key={r.code} value={r.code}>
                  [{r.code}] {r.label}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label
              htmlFor="closure-notes"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1"
            >
              Final Operational Closure Notes (Immutable)
            </label>
            <textarea
              id="closure-notes"
              required
              rows={3}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="State final settlement proof, authorization reference, or customer communication..."
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <div className="flex items-center justify-end gap-2 pt-3 border-t border-[var(--surface-border-subtle)]">
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={onClose}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              variant="primary"
              disabled={!notes.trim()}
              isLoading={isSubmitting}
            >
              Confirm Case Closure
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};
