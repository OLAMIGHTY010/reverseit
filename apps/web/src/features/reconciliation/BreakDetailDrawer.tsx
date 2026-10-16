import React from 'react';
import {
  X,
  ShieldAlert,
  Link2,
  Sliders,
  CheckCircle2,
  Clock,
  Building,
  Server,
  FileSpreadsheet,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { ReconciliationBreak } from './reconciliationTypes';

interface BreakDetailDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  reconciliationBreak: ReconciliationBreak | null;
  onOpenPairingModal: () => void;
  onApplyTolerance: (breakId: string) => void;
  onEscalateInvestigation: (breakId: string) => void;
}

export const BreakDetailDrawer: React.FC<BreakDetailDrawerProps> = ({
  isOpen,
  onClose,
  reconciliationBreak,
  onOpenPairingModal,
  onApplyTolerance,
  onEscalateInvestigation,
}) => {
  if (!isOpen || !reconciliationBreak) return null;

  const b = reconciliationBreak;

  return (
    <div className="fixed inset-y-0 right-0 z-50 w-full max-w-xl bg-[var(--surface)] border-l border-[var(--surface-border)] shadow-2xl flex flex-col animate-in slide-in-from-right duration-200">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
        <div className="space-y-0.5">
          <div className="flex items-center gap-2">
            <span
              className={`text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded font-mono-financial ${
                b.severity === 'CRITICAL'
                  ? 'bg-rose-100 text-rose-900 border border-rose-300'
                  : b.severity === 'HIGH'
                  ? 'bg-amber-100 text-amber-900 border border-amber-300'
                  : 'bg-blue-100 text-blue-900 border border-blue-300'
              }`}
            >
              {b.severity} BREAK
            </span>
            <span className="text-[12px] font-mono-financial text-[var(--text-muted)]">
              {b.id}
            </span>
          </div>
          <h2 className="text-[15px] font-bold text-[var(--text-primary)]">
            {b.breakLabel}
          </h2>
        </div>
        <button
          onClick={onClose}
          className="p-1.5 text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded-md hover:bg-[var(--canvas-subdued)]"
        >
          <X className="w-5 h-5" />
        </button>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto p-6 space-y-6">
        {/* Status & Session Meta */}
        <div className="grid grid-cols-2 gap-3 text-[12px] font-mono-financial">
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded space-y-1">
            <span className="text-[var(--text-muted)] text-[10px] uppercase tracking-wider font-sans">
              Settlement Session
            </span>
            <div className="font-bold text-[var(--text-primary)]">{b.settlementSession}</div>
            <div className="text-[11px] text-[var(--text-secondary)]">{b.sessionDate}</div>
          </div>

          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded space-y-1">
            <span className="text-[var(--text-muted)] text-[10px] uppercase tracking-wider font-sans">
              Reconciliation State
            </span>
            <div className="font-bold text-indigo-700">{b.status.replace(/_/g, ' ')}</div>
            <div className="text-[11px] text-[var(--text-secondary)]">Channel: {b.channel}</div>
          </div>
        </div>

        {/* Amount Delta Callout */}
        <div
          className={`p-4 rounded border flex items-center justify-between ${
            b.amountDifference > 0
              ? 'bg-rose-50/70 border-rose-200 text-rose-950'
              : 'bg-emerald-50/70 border-emerald-200 text-emerald-950'
          }`}
        >
          <div className="space-y-0.5">
            <div className="text-[11px] font-bold uppercase tracking-wider">
              {b.amountDifference > 0 ? 'Financial Variance (Break Delta)' : 'Balanced Amount Match'}
            </div>
            <div className="text-[20px] font-bold font-mono-financial">
              ₦{b.amountDifference.toLocaleString('en-NG', { minimumFractionDigits: 2 })}
            </div>
          </div>
          {b.timeDifferenceSeconds > 0 && (
            <div className="text-right font-mono-financial text-[11px] text-[var(--text-secondary)] flex items-center gap-1">
              <Clock className="w-3.5 h-3.5" />
              <span>Δ {b.timeDifferenceSeconds}s time drift</span>
            </div>
          )}
        </div>

        {/* Dual Sided Record Comparison */}
        <div className="space-y-3">
          <h3 className="text-[13px] font-bold text-[var(--text-primary)] flex items-center gap-1.5">
            <FileSpreadsheet className="w-4 h-4 text-indigo-600" />
            <span>Dual-Sided Source Verification</span>
          </h3>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {/* Core Ledger Column */}
            <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md space-y-3 font-mono-financial text-[11px]">
              <div className="flex items-center justify-between pb-2 border-b border-[var(--surface-border-subtle)]">
                <span className="font-bold text-[var(--text-primary)] font-sans flex items-center gap-1.5 text-[12px]">
                  <Building className="w-3.5 h-3.5 text-blue-600" />
                  <span>Internal Core Ledger</span>
                </span>
                <span
                  className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                    b.ledgerRecord
                      ? 'bg-emerald-100 text-emerald-800'
                      : 'bg-rose-100 text-rose-800'
                  }`}
                >
                  {b.ledgerRecord ? 'ENTRY PRESENT' : 'MISSING ENTRY'}
                </span>
              </div>

              {b.ledgerRecord ? (
                <div className="space-y-2">
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Core Platform</span>
                    <span className="font-bold text-[var(--text-primary)]">
                      {b.ledgerRecord.coreSystem}
                    </span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Txn Reference</span>
                    <span className="text-[var(--text-primary)]">{b.ledgerRecord.reference}</span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Account Debited</span>
                    <span className="text-[var(--text-primary)]">
                      {b.ledgerRecord.accountNumber}
                    </span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Amount Posted</span>
                    <span className="font-bold text-[var(--text-primary)] text-[13px]">
                      ₦{b.ledgerRecord.amount} {b.ledgerRecord.currency}
                    </span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Posting Timestamp</span>
                    <span className="text-[var(--text-secondary)]">{b.ledgerRecord.postedAt}</span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Narration</span>
                    <p className="text-[10px] text-[var(--text-muted)] font-sans leading-tight">
                      {b.ledgerRecord.narration}
                    </p>
                  </div>
                </div>
              ) : (
                <div className="p-4 text-center text-rose-700 bg-rose-50/50 rounded text-[11px] font-sans">
                  No corresponding debit entry found on core banking ledger.
                </div>
              )}
            </div>

            {/* Switch Clearing File Column */}
            <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md space-y-3 font-mono-financial text-[11px]">
              <div className="flex items-center justify-between pb-2 border-b border-[var(--surface-border-subtle)]">
                <span className="font-bold text-[var(--text-primary)] font-sans flex items-center gap-1.5 text-[12px]">
                  <Server className="w-3.5 h-3.5 text-purple-600" />
                  <span>Switch Clearing File</span>
                </span>
                <span
                  className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                    b.switchRecord
                      ? 'bg-purple-100 text-purple-800'
                      : 'bg-rose-100 text-rose-800'
                  }`}
                >
                  {b.switchRecord ? 'RECORD PRESENT' : 'MISSING RECORD'}
                </span>
              </div>

              {b.switchRecord ? (
                <div className="space-y-2">
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Switch Operator</span>
                    <span className="font-bold text-[var(--text-primary)]">
                      {b.switchRecord.switchPartner}
                    </span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">STAN / RRN</span>
                    <span className="text-[var(--text-primary)]">
                      STAN: {b.switchRecord.stan} | RRN: {b.switchRecord.rrn}
                    </span>
                  </div>
                  {b.switchRecord.terminalId && (
                    <div>
                      <span className="text-[var(--text-muted)] text-[10px] block">Terminal ID</span>
                      <span className="text-[var(--text-primary)]">
                        {b.switchRecord.terminalId}
                      </span>
                    </div>
                  )}
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Clearing Amount</span>
                    <span className="font-bold text-[var(--text-primary)] text-[13px]">
                      ₦{b.switchRecord.clearingAmount} {b.switchRecord.currency}
                    </span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Response Status</span>
                    <span className="text-amber-800 font-bold">
                      {b.switchRecord.switchResponseCode}
                    </span>
                  </div>
                  <div>
                    <span className="text-[var(--text-muted)] text-[10px] block">Clearing Timestamp</span>
                    <span className="text-[var(--text-secondary)]">{b.switchRecord.clearedAt}</span>
                  </div>
                </div>
              ) : (
                <div className="p-4 text-center text-rose-700 bg-rose-50/50 rounded text-[11px] font-sans">
                  Switch settlement file has no matching clearing session or STAN record.
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Diagnostic Narrative & Tolerance Rule */}
        {b.makerNotes && (
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[12px] space-y-1">
            <span className="font-bold text-[var(--text-primary)] text-[11px] uppercase tracking-wider block">
              Analyst Reconciliation Notes
            </span>
            <p className="text-[var(--text-secondary)] leading-relaxed">{b.makerNotes}</p>
          </div>
        )}

        {b.toleranceRule && (
          <div className="p-3 bg-indigo-50 border border-indigo-200 rounded text-[12px] text-indigo-950 flex items-start gap-2.5">
            <Sliders className="w-4 h-4 text-indigo-600 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold">Active Tolerance Rule: </span>
              {b.toleranceRule}
            </div>
          </div>
        )}

        {/* Checker Signoff status if present */}
        {b.checkerSignoff && (
          <div className="p-3 bg-emerald-50 border border-emerald-200 rounded text-[12px] text-emerald-950 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <CheckCircle2 className="w-4 h-4 text-emerald-600" />
              <div>
                <div className="font-bold">Dual-Control Checker Sign-Off Confirmed</div>
                <div className="text-[11px] text-emerald-800">
                  {b.checkerSignoff.approvedBy} at {b.checkerSignoff.approvedAt}
                </div>
              </div>
            </div>
            <span className="px-2 py-0.5 rounded bg-emerald-200 text-emerald-900 font-bold text-[10px]">
              {b.checkerSignoff.status}
            </span>
          </div>
        )}
      </div>

      {/* Action Footer */}
      <div className="p-4 border-t border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] space-y-2">
        <div className="grid grid-cols-2 gap-2">
          {b.toleranceApplicable && !b.isToleranceApplied ? (
            <Button
              variant="secondary"
              size="sm"
              leftIcon={<Sliders className="w-4 h-4" />}
              onClick={() => onApplyTolerance(b.id)}
            >
              Apply Tolerance
            </Button>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              leftIcon={<Link2 className="w-4 h-4" />}
              onClick={onOpenPairingModal}
            >
              Manual Force Match
            </Button>
          )}

          <Button
            variant="primary"
            size="sm"
            leftIcon={<ShieldAlert className="w-4 h-4" />}
            onClick={() => onEscalateInvestigation(b.id)}
          >
            Escalate to Case
          </Button>
        </div>
      </div>
    </div>
  );
};
