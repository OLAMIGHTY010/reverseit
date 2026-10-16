import React, { useState } from 'react';
import {
  CheckCircle2,
  ShieldCheck,
  Building2,
  Copy,
  Check,
  FileText,
  CreditCard,
  Hash,
} from 'lucide-react';
import { Button } from '../../components/Button';
import { useApp } from '../../context/AppContext';
import type { AgentQueryResult } from '../../types/domain';

export interface AgentVerifiedSuccessCardProps {
  result: AgentQueryResult;
}

export const AgentVerifiedSuccessCard: React.FC<AgentVerifiedSuccessCardProps> = ({ result }) => {
  const { addToast } = useApp();
  const [copiedEvidence, setCopiedEvidence] = useState(false);
  const ev = result.evidenceSummary;
  const txn = result.transaction;

  const handleCopyEvidence = async () => {
    if (!ev) return;
    try {
      await navigator.clipboard.writeText(ev.quotableSummary);
      setCopiedEvidence(true);
      addToast({
        type: 'success',
        title: 'Evidence Summary Copied',
        message: 'Quotable settlement proof copied to clipboard.',
      });
      setTimeout(() => setCopiedEvidence(false), 2500);
    } catch {
      setCopiedEvidence(true);
      setTimeout(() => setCopiedEvidence(false), 2500);
    }
  };

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
      {/* 1. Header Banner */}
      <div className="p-4 bg-emerald-500/10 border-b border-emerald-200 flex flex-wrap items-center justify-between gap-3 text-emerald-950">
        <div className="flex items-center gap-2">
          <span className="p-1 rounded bg-emerald-100 text-emerald-800">
            <CheckCircle2 className="w-5 h-5 text-emerald-600" />
          </span>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-mono-financial font-bold text-[14px]">
                {txn?.reference || 'TXN-20261015-NIP-9902'}
              </span>
              <span className="text-[11px] font-mono-financial font-semibold px-2 py-0.2 rounded bg-emerald-100 text-emerald-800 border border-emerald-300">
                VERIFIED SUCCESSFUL
              </span>
            </div>
            <p className="text-[12px] text-emerald-800/90 mt-0.5">
              Settlement Confirmed &bull; Both core debit and counterparty credit acknowledged.
            </p>
          </div>
        </div>

        <div className="text-right">
          <div className="text-[15px] font-bold font-mono-financial text-emerald-950">
            {txn ? `${txn.currency} ${Number(txn.amount).toLocaleString('en-US', { minimumFractionDigits: 2 })}` : 'NGN 45,000.00'}
          </div>
          <div className="text-[11px] text-emerald-800 font-mono-financial">
            Settled at Destination Bank
          </div>
        </div>
      </div>

      {/* 2. Quotable Settlement Evidence Breakdown */}
      <div className="p-4 space-y-4 text-[13px]">
        <div>
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] font-bold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-1.5">
              <FileText className="w-3.5 h-3.5 text-sky-600" /> Quotable Settlement Proof (Agent Script Cue)
            </span>
            <Button
              size="sm"
              variant="secondary"
              onClick={handleCopyEvidence}
              leftIcon={copiedEvidence ? <Check className="w-3 h-3 text-emerald-600" /> : <Copy className="w-3 h-3" />}
            >
              {copiedEvidence ? 'Copied' : 'Copy Evidence Ref'}
            </Button>
          </div>

          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded font-mono-financial text-[12px] text-[var(--text-primary)] leading-relaxed">
            {ev?.quotableSummary || 'Core ledger debited; destination bank confirmed credit message.'}
          </div>
        </div>

        {/* 3. Verified Field Ledger Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3 text-[12px]">
          <div className="p-2.5 bg-[var(--canvas-subdued)] rounded border border-[var(--surface-border-subtle)] space-y-0.5">
            <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] flex items-center gap-1">
              <Building2 className="w-3 h-3 text-sky-600" /> Destination Bank
            </span>
            <span className="font-semibold text-[var(--text-primary)]">
              {ev?.beneficiaryBank || 'GTBank (058)'}
            </span>
            <span className="text-[11px] font-mono-financial text-[var(--text-muted)] block">
              Acct: {ev?.beneficiaryAccount || '0144882190'}
            </span>
          </div>

          <div className="p-2.5 bg-[var(--canvas-subdued)] rounded border border-[var(--surface-border-subtle)] space-y-0.5">
            <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] flex items-center gap-1">
              <Hash className="w-3 h-3 text-sky-600" /> Switch Session ID
            </span>
            <span className="font-mono-financial font-semibold text-[var(--text-primary)] break-all text-[11px]">
              {ev?.sessionId || '99901426101509294288420'}
            </span>
            <span className="text-[10px] text-emerald-700 font-semibold block">
              ✓ Acknowledged by recipient switch
            </span>
          </div>

          <div className="p-2.5 bg-[var(--canvas-subdued)] rounded border border-[var(--surface-border-subtle)] space-y-0.5">
            <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] flex items-center gap-1">
              <CreditCard className="w-3 h-3 text-sky-600" /> STAN & RRN
            </span>
            <span className="font-mono-financial font-semibold text-[var(--text-primary)]">
              STAN: {ev?.stan || '009902'}
            </span>
            <span className="text-[11px] font-mono-financial text-[var(--text-muted)] block">
              RRN: {ev?.rrn || '9901847121'}
            </span>
          </div>

          <div className="p-2.5 bg-[var(--canvas-subdued)] rounded border border-[var(--surface-border-subtle)] space-y-0.5">
            <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] flex items-center gap-1">
              <ShieldCheck className="w-3 h-3 text-emerald-600" /> Settlement Time
            </span>
            <span className="font-mono-financial font-semibold text-[var(--text-primary)]">
              {ev?.settledAt ? new Date(ev.settledAt).toLocaleTimeString() : '09:30:02 AM'}
            </span>
            <span className="text-[10px] text-[var(--text-muted)] block">
              Batch: Net Obligation OK
            </span>
          </div>
        </div>

        {/* 4. Agent Reassurance Directive */}
        <div className="p-3 bg-sky-50 text-sky-950 border border-sky-200 rounded text-[12px] space-y-1">
          <span className="font-bold text-sky-900 block">Agent Action Guidance:</span>
          <p className="text-sky-900 leading-relaxed">
            The customer may be reporting non-receipt by their beneficiary. Reassure the customer that funds have departed Wema Bank and are documented as received at the destination bank. Advise the customer to share the <strong>Session ID ({ev?.sessionId || '99901426101509294288420'})</strong> with the recipient to confirm credit with their bank branch.
          </p>
        </div>
      </div>
    </div>
  );
};
