import React, { useState, useEffect } from 'react';
import {
  X,
  User,
  Bot,
  AlertTriangle,
  Send,
  MessageSquare,
  FileSpreadsheet,
} from 'lucide-react';
import { Button } from '../../components/Button';
import { StatusBadge } from '../../components/StatusBadge';
import { CountdownTimer } from './CountdownTimer';
import { PriorityWhyPopover } from './PriorityWhyPopover';
import { MaskedField } from '../../components/MaskedField';
import { EvidencePanel } from '../../components/EvidencePanel';
import { useApp } from '../../context/AppContext';
import type { FinancialException, EvidenceItem } from '../../types/domain';

interface ExceptionDetailDrawerProps {
  exception: FinancialException | null;
  onClose: () => void;
  onClaim: (exception: FinancialException) => void;
  onOpenAssign: (exception: FinancialException) => void;
  onOpenStateChange: (exception: FinancialException) => void;
  onAddNote: (exceptionId: string, text: string) => Promise<void>;
  onLinkIncident?: (exception: FinancialException) => void;
  onProposeReversal: (exception: FinancialException) => void;
}

export const ExceptionDetailDrawer: React.FC<ExceptionDetailDrawerProps> = ({
  exception,
  onClose,
  onClaim,
  onOpenAssign,
  onOpenStateChange,
  onAddNote,
  onProposeReversal,
}) => {
  const { role, addToast } = useApp();
  const [newNote, setNewNote] = useState('');
  const [isAddingNote, setIsAddingNote] = useState(false);

  // Close on ESC
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        onClose();
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  if (!exception) return null;

  const isMaker = role === 'ANALYST_MAKER' || role === 'ADMIN';
  const isClaimedByMe = exception.assignedTo === 'analyst.maker@wema.ng';

  // Evidence items derived from exception details
  const evidenceList: EvidenceItem[] = [
    {
      id: `ev-fact-1`,
      type: 'FACT',
      title: 'Core Ledger Entry Recorded',
      source: `GL-Account ${exception.accountNumber || '0123456789'}`,
      timestamp: exception.debitTimestamp,
      details: `Debit posted: ${exception.currency} ${Number(exception.amount).toLocaleString('en-US', { minimumFractionDigits: 2 })}. STAN: ${exception.stan || '009901'}.`,
      verified: true,
      freshness: 'FRESH',
      freshnessLabel: 'Verified Fresh',
      isStale: false,
      canSupportAction: true,
    },
    {
      id: `ev-fact-2`,
      type: 'FACT',
      title: `${exception.channel} Switch Transaction Journal`,
      source: `${exception.channel} Switch Network Gateway`,
      timestamp: exception.detectedAt,
      details: `RRN: ${exception.rrn || '9901847120'}. Root cause code: ${exception.rootCause}. Status: ${exception.status}.`,
      verified: true,
      freshness: 'FRESH',
      freshnessLabel: 'Verified Fresh',
      isStale: false,
      canSupportAction: true,
    },
    {
      id: `ev-inf-1`,
      type: 'INFERRED',
      title: 'ML Anomaly Pattern Match',
      source: 'XGBoost Heuristic Classifier',
      details: `Classification: ${exception.classificationCode}. Confidence: ${exception.modelConfidence}%. ${exception.modelExplanation}`,
      verified: false,
      freshness: 'FRESH',
      freshnessLabel: 'AI Inference Fresh',
      isStale: false,
      canSupportAction: true,
    },
    {
      id: `ev-unres-1`,
      type: exception.repeatContactCount > 0 ? 'MISSING' : 'UNRESOLVED',
      title: exception.repeatContactCount > 0 ? 'Customer Escalation Pending' : 'Counterparty Settlement Recon',
      source: exception.source === 'CUSTOMER' ? 'ALAT Support Ticket CRM' : 'NIBSS Settlement Batch',
      details: exception.source === 'CUSTOMER'
        ? `Customer logged ${exception.repeatContactCount} repeat inquiries. Awaiting proof of beneficiary non-credit.`
        : 'Counterparty receiving institution has not returned credit confirmation message.',
      verified: false,
      freshness: 'DEGRADED',
      freshnessLabel: 'Degraded Source (14m lag)',
      isStale: false,
      canSupportAction: false,
    },
  ];

  const handleNoteSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newNote.trim()) return;
    setIsAddingNote(true);
    try {
      await onAddNote(exception.id, newNote.trim());
      setNewNote('');
      addToast({
        type: 'success',
        title: 'Note Added',
        message: 'Investigation note logged to immutable audit trail.',
      });
    } catch {
      addToast({
        type: 'error',
        title: 'Failed to Add Note',
        message: 'Could not append note to exception record.',
      });
    } finally {
      setIsAddingNote(false);
    }
  };

  return (
    <aside
      role="region"
      aria-label={`Exception Detail Preview for ${exception.reference}`}
      className="w-[420px] shrink-0 bg-[var(--surface)] border-l border-[var(--surface-border)] flex flex-col h-full shadow-lg text-[13px] overflow-hidden animate-in slide-in-from-right duration-150 z-20"
    >
      {/* Drawer Header */}
      <div className="p-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-start justify-between gap-2">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span
              className={`text-[10px] font-bold px-2 py-0.5 rounded font-mono-financial border ${
                exception.severity === 'CRITICAL'
                  ? 'bg-rose-100 text-rose-900 border-rose-300'
                  : exception.severity === 'HIGH'
                  ? 'bg-amber-100 text-amber-900 border-amber-300'
                  : 'bg-slate-100 text-slate-800 border-slate-300'
              }`}
            >
              {exception.severity}
            </span>
            <span className="px-2 py-0.5 rounded font-mono-financial font-bold text-[11px] bg-sky-50 text-sky-900 border border-sky-300">
              {exception.classificationCode}
            </span>
            <span
              className={`inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded border font-medium ${
                exception.source === 'CUSTOMER'
                  ? 'bg-indigo-50 text-indigo-900 border-indigo-200'
                  : 'bg-slate-100 text-slate-700 border-slate-200'
              }`}
            >
              {exception.source === 'CUSTOMER' ? (
                <>
                  <User className="w-3 h-3 text-indigo-600" /> Customer
                </>
              ) : (
                <>
                  <Bot className="w-3 h-3 text-sky-600" /> Bot
                </>
              )}
            </span>
          </div>
          <h2 className="font-mono-financial font-bold text-[15px] text-[var(--text-primary)]">
            {exception.reference}
          </h2>
        </div>

        <button
          type="button"
          onClick={onClose}
          className="p-1.5 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
          aria-label="Close detail drawer (Esc)"
          title="Close (Esc)"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {/* Drawer Action Bar */}
      <div className="px-4 py-2 bg-[var(--surface-raised)] border-b border-[var(--surface-border-subtle)] flex flex-wrap items-center gap-1.5">
        {/* Claim Action */}
        <span
          title={
            !isMaker
              ? 'Claiming requires ANALYST_MAKER or ADMIN role'
              : isClaimedByMe
              ? 'Already claimed by you'
              : 'Claim this exception to your queue'
          }
        >
          <Button
            size="xs"
            variant={isClaimedByMe ? 'secondary' : 'primary'}
            disabled={!isMaker || isClaimedByMe}
            onClick={() => onClaim(exception)}
          >
            {isClaimedByMe ? 'Claimed by You' : 'Claim Case'}
          </Button>
        </span>

        {/* Assign Action */}
        <Button
          size="xs"
          variant="secondary"
          onClick={() => onOpenAssign(exception)}
        >
          Assign / Reassign
        </Button>

        {/* Change State Action */}
        <Button
          size="xs"
          variant="secondary"
          onClick={() => onOpenStateChange(exception)}
        >
          Change State
        </Button>

        {/* Propose Reversal */}
        <span
          title={
            !isMaker
              ? 'Only ANALYST_MAKER can draft reversal proposals'
              : 'Draft dual-control reversal proposal'
          }
        >
          <Button
            size="xs"
            variant="danger"
            disabled={!isMaker}
            onClick={() => onProposeReversal(exception)}
          >
            Propose Reversal
          </Button>
        </span>
      </div>

      {/* Drawer Body */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Dual SLA Timers Card */}
        <div className="p-3 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)] space-y-2">
          <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center justify-between">
            <span>SLA & Resolution Countdowns</span>
            <StatusBadge status={exception.status} size="sm" />
          </div>

          <div className="grid grid-cols-2 gap-2 pt-1">
            <div className="space-y-1">
              <span className="text-[10px] text-[var(--text-secondary)] block">Internal SLA Target</span>
              <CountdownTimer targetDate={exception.internalSlaTarget} label="Int" />
            </div>
            <div className="space-y-1">
              <span className="text-[10px] text-[var(--text-secondary)] block">CBN Regulatory SLA</span>
              <CountdownTimer
                targetDate={exception.regulatoryDeadline}
                label="Reg"
                type="regulatory"
              />
            </div>
          </div>
        </div>

        {/* Priority & Financial Exposure */}
        <div className="grid grid-cols-2 gap-3 p-3 rounded-md bg-[var(--surface)] border border-[var(--surface-border-subtle)] shadow-xs">
          <div>
            <span className="text-[11px] text-[var(--text-muted)] block">Disputed Amount</span>
            <div className="font-mono-financial font-bold text-[16px] text-[var(--text-primary)]">
              {exception.currency}{' '}
              {Number(exception.amount).toLocaleString('en-US', {
                minimumFractionDigits: 2,
              })}
            </div>
            <span className="text-[10px] font-medium text-[var(--text-muted)]">
              Channel: {exception.channel}
            </span>
          </div>

          <div className="space-y-1">
            <span className="text-[11px] text-[var(--text-muted)] block">Priority Ranking</span>
            <PriorityWhyPopover
              score={exception.priorityScore}
              factors={exception.priorityFactors}
              exceptionRef={exception.reference}
            />
            <div className="text-[10px] text-[var(--text-secondary)] font-mono-financial">
              Exposure: {exception.currency} {Number(exception.exposure).toLocaleString('en-US')}
            </div>
          </div>
        </div>

        {/* Incident Badge (if linked) */}
        {exception.incidentId && (
          <div className="p-2.5 rounded bg-amber-50/70 border border-amber-300 text-amber-950 text-[12px] flex items-start gap-2">
            <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold font-mono-financial text-amber-900">
                {exception.incidentId}
              </span>
              <p className="text-[11px] opacity-90 mt-0.5">
                {exception.incidentTitle || 'Major payment gateway incident linked.'}
              </p>
            </div>
          </div>
        )}

        {/* Masked Customer Account & Identity */}
        <div className="p-3 rounded-md bg-[var(--surface)] border border-[var(--surface-border-subtle)] space-y-2">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
            <User className="w-3.5 h-3.5 text-slate-500" /> Customer & Account Provenance
          </span>

          <div className="space-y-2 text-[12px]">
            <div className="flex items-center justify-between">
              <span className="text-[var(--text-muted)]">Customer Name:</span>
              <span className="font-semibold text-[var(--text-primary)]">
                {exception.customerName || 'Confidential Banking Customer'}
              </span>
            </div>

            <div className="flex items-center justify-between">
              <span className="text-[var(--text-muted)]">Debit Account:</span>
              <MaskedField
                maskedValue={
                  exception.accountNumber
                    ? `${exception.accountNumber.slice(0, 3)}••••${exception.accountNumber.slice(-3)}`
                    : '012••••789'
                }
                revealedValue={exception.accountNumber || '0123456789'}
                fieldLabel="Customer Account Number"
                entityId={exception.reference}
              />
            </div>

            {exception.maskedPan && (
              <div className="flex items-center justify-between">
                <span className="text-[var(--text-muted)]">Card PAN:</span>
                <MaskedField
                  maskedValue={exception.maskedPan}
                  revealedValue="5061 9901 8841 7711"
                  fieldLabel="Customer Card PAN"
                  entityId={exception.reference}
                />
              </div>
            )}

            <div className="flex items-center justify-between">
              <span className="text-[var(--text-muted)]">STAN / RRN:</span>
              <span className="font-mono-financial text-[11px] text-[var(--text-primary)]">
                {exception.stan || '009901'} / {exception.rrn || '9901847120'}
              </span>
            </div>
          </div>
        </div>

        {/* 4-Quadrant Evidence Dossier */}
        <div className="space-y-2">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
            <FileSpreadsheet className="w-3.5 h-3.5 text-sky-600" /> Evidence Dossier
          </span>
          <EvidencePanel evidence={evidenceList} reference={exception.reference} />
        </div>

        {/* Investigation Notes & Audit Log */}
        <div className="space-y-2">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
            <MessageSquare className="w-3.5 h-3.5 text-indigo-600" /> Investigation Notes ({exception.notes.length})
          </span>

          <div className="space-y-2 max-h-48 overflow-y-auto pr-1">
            {exception.notes.length === 0 ? (
              <p className="text-[11px] text-[var(--text-muted)] italic p-2 bg-[var(--canvas-subdued)] rounded text-center">
                No analyst notes recorded yet.
              </p>
            ) : (
              exception.notes.map((note) => (
                <div
                  key={note.id}
                  className="p-2.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] text-[12px] space-y-1"
                >
                  <div className="flex items-center justify-between text-[10px] text-[var(--text-muted)] font-mono-financial">
                    <span className="font-bold text-[var(--text-secondary)]">{note.author}</span>
                    <span>{new Date(note.timestamp).toLocaleTimeString()}</span>
                  </div>
                  <p className="text-[var(--text-primary)] text-[12px] leading-relaxed">
                    {note.text}
                  </p>
                </div>
              ))
            )}
          </div>

          {/* Add Note Form */}
          <form onSubmit={handleNoteSubmit} className="flex items-center gap-1.5 pt-1">
            <input
              type="text"
              value={newNote}
              onChange={(e) => setNewNote(e.target.value)}
              placeholder="Add investigation finding or ledger note..."
              className="flex-1 px-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
            <Button
              type="submit"
              size="xs"
              variant="primary"
              disabled={isAddingNote || !newNote.trim()}
              leftIcon={<Send className="w-3 h-3" />}
            >
              Add
            </Button>
          </form>
        </div>
      </div>
    </aside>
  );
};
