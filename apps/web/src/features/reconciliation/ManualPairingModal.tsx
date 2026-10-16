import React, { useState } from 'react';
import { X, Link2, AlertTriangle, CheckCircle2, ShieldCheck } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ReconciliationBreak } from './reconciliationTypes';

interface ManualPairingModalProps {
  isOpen: boolean;
  onClose: () => void;
  reconciliationBreak: ReconciliationBreak | null;
  onConfirmPair: (breakId: string, notes: string) => void;
}

export const ManualPairingModal: React.FC<ManualPairingModalProps> = ({
  isOpen,
  onClose,
  reconciliationBreak,
  onConfirmPair,
}) => {
  const [makerNotes, setMakerNotes] = useState('');
  const [hasAcknowledgedVariance, setHasAcknowledgedVariance] = useState(false);

  if (!isOpen || !reconciliationBreak) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!makerNotes.trim()) return;
    onConfirmPair(reconciliationBreak.id, makerNotes);
    onClose();
  };

  const isVariance = reconciliationBreak.amountDifference > 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-200">
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl w-full max-w-xl overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <Link2 className="w-5 h-5 text-indigo-600" />
            <h3 className="text-[15px] font-bold text-[var(--text-primary)]">
              Manual Break Pairing & Force Match (Maker-Checker)
            </h3>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          <div className="p-3 bg-indigo-50/60 border border-indigo-200 rounded text-[12px] text-indigo-900 flex items-start gap-2.5">
            <ShieldCheck className="w-4 h-4 text-indigo-600 shrink-0 mt-0.5" />
            <div>
              <span className="font-semibold">Dual-Control Governance Required: </span>
              Manual pairing binds disparate settlement artifacts. Once submitted by Maker, this match
              requires verification and sign-off by a designated Checker before ledger posting.
            </div>
          </div>

          {/* Dual Sided Preview */}
          <div className="grid grid-cols-2 gap-3 text-[11px] font-mono-financial">
            {/* Ledger Side */}
            <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded space-y-1.5">
              <div className="font-bold text-[var(--text-primary)] font-sans text-[12px] flex items-center justify-between">
                <span>Core Ledger Record</span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-blue-100 text-blue-800">
                  {reconciliationBreak.ledgerRecord?.coreSystem || 'Missing'}
                </span>
              </div>
              <div className="text-[var(--text-muted)]">
                Ref: {reconciliationBreak.ledgerRecord?.reference || 'N/A'}
              </div>
              <div className="text-[var(--text-muted)]">
                Acct: {reconciliationBreak.ledgerRecord?.accountNumber || 'N/A'}
              </div>
              <div className="text-[13px] font-bold text-[var(--text-primary)]">
                ₦{reconciliationBreak.ledgerRecord?.amount || '0.00'}
              </div>
              <div className="text-[10px] text-[var(--text-muted)]">
                {reconciliationBreak.ledgerRecord?.postedAt || 'No ledger entry recorded'}
              </div>
            </div>

            {/* Switch Side */}
            <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded space-y-1.5">
              <div className="font-bold text-[var(--text-primary)] font-sans text-[12px] flex items-center justify-between">
                <span>Switch Clearing File</span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-purple-100 text-purple-800">
                  {reconciliationBreak.switchRecord?.switchPartner || 'Missing'}
                </span>
              </div>
              <div className="text-[var(--text-muted)]">
                STAN: {reconciliationBreak.switchRecord?.stan || 'N/A'}
              </div>
              <div className="text-[var(--text-muted)]">
                RRN: {reconciliationBreak.switchRecord?.rrn || 'N/A'}
              </div>
              <div className="text-[13px] font-bold text-[var(--text-primary)]">
                ₦{reconciliationBreak.switchRecord?.clearingAmount || '0.00'}
              </div>
              <div className="text-[10px] text-[var(--text-muted)]">
                {reconciliationBreak.switchRecord?.clearedAt || 'No switch clearing record'}
              </div>
            </div>
          </div>

          {/* Variance warning if any */}
          {isVariance && (
            <div className="p-3 bg-amber-50 border border-amber-200 rounded text-[12px] text-amber-900 space-y-2">
              <div className="flex items-center gap-2 font-bold text-amber-950">
                <AlertTriangle className="w-4 h-4 text-amber-600 shrink-0" />
                <span>Amount Variance Detected: ₦{reconciliationBreak.amountDifference.toLocaleString('en-NG', { minimumFractionDigits: 2 })}</span>
              </div>
              <p className="text-[11px] leading-relaxed">
                The settlement amount differs from the ledger amount. Ensure your justification accounts
                for the difference (e.g. interchange fee deduction, VAT, or partial reversal).
              </p>
              <label className="flex items-center gap-2 cursor-pointer pt-1 font-medium text-[11px]">
                <input
                  type="checkbox"
                  checked={hasAcknowledgedVariance}
                  onChange={(e) => setHasAcknowledgedVariance(e.target.checked)}
                  className="w-4 h-4 text-amber-600 rounded"
                />
                <span>I verify that the variance has been reconciled to an allowable fee account</span>
              </label>
            </div>
          )}

          <div>
            <label className="block text-[12px] font-bold text-[var(--text-primary)] mb-1">
              Maker Pairing Justification & Audit Narrative *
            </label>
            <textarea
              required
              rows={3}
              value={makerNotes}
              onChange={(e) => setMakerNotes(e.target.value)}
              placeholder="Explain why these two records correspond, citing switch query ticket or cut-off timing..."
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
          </div>

          <div className="flex items-center justify-end gap-2 pt-3 border-t border-[var(--surface-border-subtle)]">
            <Button variant="ghost" size="sm" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              type="submit"
              disabled={!makerNotes.trim() || (isVariance && !hasAcknowledgedVariance)}
              leftIcon={<CheckCircle2 className="w-4 h-4" />}
            >
              Submit for Checker Sign-off
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};
