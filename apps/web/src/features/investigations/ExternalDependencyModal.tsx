import React, { useState } from 'react';
import { X, ExternalLink } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ExternalDependency } from '../../types/domain';

interface ExternalDependencyModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (dependency: Omit<ExternalDependency, 'id'>) => Promise<void>;
}

export const ExternalDependencyModal: React.FC<ExternalDependencyModalProps> = ({
  isOpen,
  onClose,
  onSubmit,
}) => {
  const [namedParty, setNamedParty] = useState('NIBSS Tier-3 Switch Operations');
  const [ticketReference, setTicketReference] = useState('');
  const [reason, setReason] = useState('');
  const [expectedByHours, setExpectedByHours] = useState('4');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ticketReference.trim() || !reason.trim()) return;

    setIsSubmitting(true);
    const expectedDate = new Date(Date.now() + parseFloat(expectedByHours) * 60 * 60 * 1000).toISOString();

    try {
      await onSubmit({
        namedParty,
        ticketReference: ticketReference.trim(),
        reason: reason.trim(),
        expectedBy: expectedDate,
        status: 'PENDING',
      });
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="dep-modal-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs animate-in fade-in duration-150 text-[13px]"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-md w-full overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <ExternalLink className="w-4 h-4 text-sky-600" />
            <h3 id="dep-modal-title" className="text-[14px] font-bold text-[var(--text-primary)]">
              Mark External Dependency
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            aria-label="Close dependency modal"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          <div>
            <label
              htmlFor="party-select"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1"
            >
              Named External Party / Switch Node
            </label>
            <select
              id="party-select"
              value={namedParty}
              onChange={(e) => setNamedParty(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            >
              <option value="NIBSS Tier-3 Switch Operations">NIBSS Tier-3 Switch Operations</option>
              <option value="Interswitch Transaction Verification Desk">Interswitch Transaction Verification Desk</option>
              <option value="Mastercard Settlement Dispute Team">Mastercard Settlement Dispute Team</option>
              <option value="Counterparty Destination Bank NOC">Counterparty Destination Bank NOC</option>
              <option value="Merchant Acquirer Operations">Merchant Acquirer Operations</option>
            </select>
          </div>

          <div>
            <label
              htmlFor="ticket-input"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1"
            >
              External Support Ticket Reference
            </label>
            <input
              id="ticket-input"
              type="text"
              required
              value={ticketReference}
              onChange={(e) => setTicketReference(e.target.value)}
              placeholder="e.g. NIBSS-INC-88120, ISW-DISP-4401"
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <div>
            <label
              htmlFor="reason-input"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1"
            >
              Dependency Reason & Required Artifact
            </label>
            <textarea
              id="reason-input"
              required
              rows={2}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="State what report or manual confirmation is required..."
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <div>
            <label
              htmlFor="hours-select"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1"
            >
              Expected Response SLA Window
            </label>
            <select
              id="hours-select"
              value={expectedByHours}
              onChange={(e) => setExpectedByHours(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
            >
              <option value="1">1 Hour (Critical SLA)</option>
              <option value="2">2 Hours</option>
              <option value="4">4 Hours (Standard)</option>
              <option value="12">12 Hours (End of Window)</option>
              <option value="24">24 Hours (Next Business Day)</option>
            </select>
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
              isLoading={isSubmitting}
            >
              Register External Dependency
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};
