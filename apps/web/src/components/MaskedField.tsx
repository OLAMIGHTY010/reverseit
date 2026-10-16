import React, { useState } from 'react';
import { Eye, EyeOff, Lock, X } from 'lucide-react';
import { Button } from './Button';
import { useApp } from '../context/AppContext';
import { apiClient } from '../services/apiClient';

export interface MaskedFieldProps {
  maskedValue: string;
  revealedValue: string;
  fieldLabel: string;
  entityId: string;
  className?: string;
}

export const MaskedField: React.FC<MaskedFieldProps> = ({
  maskedValue,
  revealedValue,
  fieldLabel,
  entityId,
  className,
}) => {
  const { role, addToast } = useApp();
  const [isRevealed, setIsRevealed] = useState(false);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleRevealClick = () => {
    if (isRevealed) {
      setIsRevealed(false);
      return;
    }
    setIsModalOpen(true);
  };

  const handleSubmitReason = async () => {
    if (!reason.trim()) return;
    setIsSubmitting(true);
    try {
      await apiClient.logDataReveal(fieldLabel, entityId, `Operator (${role})`, reason);
      setIsRevealed(true);
      setIsModalOpen(false);
      setReason('');
      addToast({
        type: 'info',
        title: 'Sensitive Field Unmasked',
        message: `Field "${fieldLabel}" unmasked. Reason logged to immutable audit trail.`,
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className={`inline-flex items-center gap-2 ${className || ''}`}>
      <span className="font-mono-financial tracking-wider text-[13px] text-[var(--text-primary)] font-medium">
        {isRevealed ? revealedValue : maskedValue}
      </span>
      <button
        type="button"
        onClick={handleRevealClick}
        className="p-1 rounded text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] cursor-pointer"
        title={isRevealed ? 'Hide sensitive data' : 'Reveal with mandatory reason'}
        aria-label={isRevealed ? `Hide ${fieldLabel}` : `Reveal ${fieldLabel} with reason`}
      >
        {isRevealed ? (
          <EyeOff className="w-3.5 h-3.5" aria-hidden="true" />
        ) : (
          <Eye className="w-3.5 h-3.5 text-[var(--primary)]" aria-hidden="true" />
        )}
      </button>

      {/* Reveal With Reason Dialog */}
      {isModalOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs"
          role="dialog"
          aria-modal="true"
          aria-labelledby="reveal-reason-title"
        >
          <div className="w-full max-w-sm bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xl overflow-hidden animate-in fade-in zoom-in-95">
            <div className="px-4 py-3 bg-[var(--canvas-subdued)] border-b border-[var(--surface-border)] flex items-center justify-between">
              <div className="flex items-center gap-1.5 text-[13px] font-semibold text-[var(--text-primary)]">
                <Lock className="w-4 h-4 text-amber-600" />
                <span id="reveal-reason-title">Mandatory Audit Reason</span>
              </div>
              <button
                type="button"
                onClick={() => setIsModalOpen(false)}
                className="text-[var(--text-muted)] hover:text-[var(--text-primary)]"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="p-4 space-y-3 text-[12px]">
              <p className="text-[var(--text-secondary)]">
                To unmask sensitive customer record <strong>{fieldLabel}</strong>, banking regulation requires an explicit investigation justification logged into the immutable audit trail.
              </p>

              <div>
                <label htmlFor="reveal-reason-input" className="block font-medium text-[var(--text-primary)] mb-1">
                  Reason for access:
                </label>
                <textarea
                  id="reveal-reason-input"
                  rows={2}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="e.g. Chargeback claim verification #8819..."
                  className="w-full px-2.5 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded focus:outline-2 focus:outline-[var(--focus-ring)] text-[12px]"
                  autoFocus
                />
              </div>
            </div>

            <div className="px-4 py-2.5 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-end gap-2">
              <Button variant="ghost" size="sm" onClick={() => setIsModalOpen(false)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={!reason.trim()}
                isLoading={isSubmitting}
                onClick={handleSubmitReason}
              >
                Confirm & Unmask
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
