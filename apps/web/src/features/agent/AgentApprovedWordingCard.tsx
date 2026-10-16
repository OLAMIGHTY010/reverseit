import React, { useState } from 'react';
import { Copy, Check, MessageSquareQuote, ShieldAlert, ShieldCheck } from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import { useApp } from '../../context/AppContext';

export interface AgentApprovedWordingCardProps {
  wording: string;
  canQuoteRefund: boolean;
  stateLabel: string;
}

export const AgentApprovedWordingCard: React.FC<AgentApprovedWordingCardProps> = ({
  wording,
  canQuoteRefund,
  stateLabel,
}) => {
  const { addToast } = useApp();
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(wording);
      setCopied(true);
      addToast({
        type: 'success',
        title: 'Script Copied to Clipboard',
        message: 'Approved customer wording copied. Ready to paste or read to caller.',
      });
      setTimeout(() => setCopied(false), 2500);
    } catch {
      // Fallback
      setCopied(true);
      setTimeout(() => setCopied(false), 2500);
    }
  };

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs overflow-hidden">
      {/* Header bar */}
      <div className="px-4 py-2.5 bg-[var(--canvas-subdued)] border-b border-[var(--surface-border)] flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="p-1 rounded bg-sky-100 text-sky-800">
            <MessageSquareQuote className="w-4 h-4" />
          </span>
          <span className="text-[13px] font-bold text-[var(--text-primary)]">
            Approved Customer Speech Script ({stateLabel})
          </span>
        </div>

        {/* Regulatory Compliance Guard Badge */}
        <div
          className={clsx(
            'flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-mono-financial font-semibold border',
            canQuoteRefund
              ? 'bg-emerald-50 text-emerald-900 border-emerald-300'
              : 'bg-amber-50 text-amber-900 border-amber-300'
          )}
        >
          {canQuoteRefund ? (
            <>
              <ShieldCheck className="w-3.5 h-3.5 text-emerald-700" />
              <span>REFUND AUTHORIZED TO QUOTE</span>
            </>
          ) : (
            <>
              <ShieldAlert className="w-3.5 h-3.5 text-amber-700" />
              <span>STRICT: DO NOT CLAIM REFUND</span>
            </>
          )}
        </div>
      </div>

      {/* Script Body */}
      <div className="p-4 space-y-3">
        <div className="relative p-3.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded text-[13px] text-[var(--text-primary)] leading-relaxed italic font-serif selection:bg-sky-200">
          &ldquo;{wording}&rdquo;
        </div>

        {/* Footnote Warning */}
        <div className="flex flex-wrap items-center justify-between gap-3 pt-1 text-[11px] text-[var(--text-muted)]">
          <p>
            {canQuoteRefund ? (
              <span className="text-emerald-800 font-semibold">
                ✓ Reversal verified settled. You are permitted to state that funds are credited back to customer.
              </span>
            ) : (
              <span className="text-amber-800 font-semibold">
                ⚠️ Compliance Notice: You must never promise or imply that money has been refunded while automated reconciliation is pending.
              </span>
            )}
          </p>

          <Button
            size="sm"
            variant={copied ? 'secondary' : 'primary'}
            onClick={handleCopy}
            leftIcon={copied ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
          >
            {copied ? 'Copied' : 'Copy Approved Script'}
          </Button>
        </div>
      </div>
    </div>
  );
};
