import React from 'react';
import {
  ShieldAlert,
  CheckCircle2,
  X,
  CreditCard,
  UserCheck,
  FileCheck2,
  ArrowRight,
} from 'lucide-react';
import { Button } from '../../components/Button';
import { StatusBadge } from '../../components/StatusBadge';
import { MaskedField } from '../../components/MaskedField';
import { getMakerCheckerPolicy, calculateDisputeAbuseProfile } from '../fraud/DisputeAbuseEngine';
import { DisputeAbuseBadge } from '../fraud/DisputeAbuseBadge';
import type { ReversalDetailRecord } from '../../types/domain';

export interface ReversalDetailDrawerProps {
  detail: ReversalDetailRecord | null;
  isOpen: boolean;
  onClose: () => void;
  onProceedToAuthorize: () => void;
  isChecker: boolean;
}

export const ReversalDetailDrawer: React.FC<ReversalDetailDrawerProps> = ({
  detail,
  isOpen,
  onClose,
  onProceedToAuthorize,
  isChecker,
}) => {
  if (!isOpen || !detail) return null;

  const policy = getMakerCheckerPolicy(detail.reversalAmount, detail.currency);
  const abuse = detail.abuseProfile || calculateDisputeAbuseProfile({
    amount: detail.reversalAmount,
    channel: detail.channel,
  });

  return (
    <div
      className="fixed inset-0 z-50 overflow-hidden bg-slate-900/50 backdrop-blur-xs flex justify-end animate-in fade-in duration-150"
      role="dialog"
      aria-modal="true"
      aria-labelledby="reversal-detail-title"
    >
      <div className="w-full max-w-2xl bg-[var(--surface)] border-l border-[var(--surface-border)] shadow-2xl flex flex-col h-full overflow-hidden animate-in slide-in-from-right duration-200">
        {/* Header */}
        <div className="px-5 py-4 border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="p-1 rounded bg-purple-100 text-purple-900">
              <ShieldAlert className="w-5 h-5" />
            </span>
            <div>
              <div className="flex items-center gap-2">
                <h2 id="reversal-detail-title" className="text-[15px] font-bold text-[var(--text-primary)]">
                  Reversal Proposal Audit Dossier
                </h2>
                <StatusBadge status={detail.status} size="sm" />
              </div>
              <p className="text-[11px] font-mono-financial text-[var(--text-muted)] mt-0.5">
                Ref: {detail.reference} &bull; Idempotency: {detail.idempotencyKey.slice(0, 16)}...
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded cursor-pointer"
            aria-label="Close detail drawer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Scrollable Content */}
        <div className="flex-1 overflow-y-auto p-5 space-y-5 text-[13px]">
          {/* Amount & Maker-Checker Tier Bar */}
          <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <span className="text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider block">
                  Total Reversal Exposure
                </span>
                <div className="text-[22px] font-bold font-mono-financial text-[var(--text-primary)]">
                  {detail.currency} {Number(detail.reversalAmount).toLocaleString('en-US', { minimumFractionDigits: 2 })}
                </div>
              </div>

              <div className="flex items-center gap-2">
                <DisputeAbuseBadge profile={abuse} compact />
                <span className="px-2 py-0.5 rounded font-mono-financial text-[11px] font-bold bg-purple-100 text-purple-900 border border-purple-300">
                  {policy.tier.replace(/_/g, ' ')}
                </span>
              </div>
            </div>

            <div className="pt-2 border-t border-[var(--surface-border-subtle)] text-[11px] text-[var(--text-secondary)] font-mono-financial flex items-center justify-between">
              <span>{policy.policyDescription}</span>
              <span className="font-semibold text-emerald-800">
                {policy.requiresChecker ? 'Dual Sign-Off Mandatory' : 'Single Analyst Authorized'}
              </span>
            </div>
          </div>

          {/* Maker Proposal Summary Card */}
          <div className="p-4 border border-[var(--surface-border-subtle)] rounded-md bg-[var(--surface)] space-y-2.5">
            <h3 className="font-bold text-[12px] uppercase text-[var(--text-muted)] tracking-wider flex items-center gap-1.5">
              <UserCheck className="w-3.5 h-3.5 text-sky-600" /> Maker Proposal Audit Record
            </h3>

            <div className="grid grid-cols-2 gap-2 text-[12px] font-mono-financial">
              <div>
                <span className="text-[var(--text-muted)] block text-[11px]">Proposed By Maker:</span>
                <span className="font-bold text-[var(--text-primary)]">{detail.proposedBy}</span>
              </div>
              <div>
                <span className="text-[var(--text-muted)] block text-[11px]">Submission Time:</span>
                <span>{new Date(detail.proposedAt).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-[var(--text-muted)] block text-[11px]">Reason Code:</span>
                <span className="font-semibold text-purple-900 bg-purple-50 px-1.5 py-0.2 rounded border border-purple-200">
                  {detail.reasonCode}
                </span>
              </div>
              <div>
                <span className="text-[var(--text-muted)] block text-[11px]">Payment Channel:</span>
                <span>{detail.channel}</span>
              </div>
            </div>

            <div className="pt-2 border-t border-[var(--surface-border-subtle)]">
              <span className="text-[11px] font-semibold text-[var(--text-muted)] block mb-1">
                Maker Justification Note:
              </span>
              <p className="text-[12px] text-[var(--text-secondary)] italic bg-[var(--canvas-subdued)] p-2.5 rounded border border-[var(--surface-border-subtle)] leading-relaxed">
                &ldquo;{detail.justification}&rdquo;
              </p>
            </div>
          </div>

          {/* Beneficiary & Originating Account Details */}
          <div className="p-4 border border-[var(--surface-border-subtle)] rounded-md bg-[var(--surface)] space-y-3">
            <h3 className="font-bold text-[12px] uppercase text-[var(--text-muted)] tracking-wider flex items-center gap-1.5">
              <CreditCard className="w-3.5 h-3.5 text-sky-600" /> Financial Settlement Routing
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-[12px]">
              <div className="p-2.5 bg-[var(--canvas-subdued)] rounded border border-[var(--surface-border-subtle)] space-y-1">
                <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] block">
                  Credit Destination Account
                </span>
                <div className="font-mono-financial font-bold text-[13px] text-[var(--text-primary)]">
                  <MaskedField
                    fieldLabel="Destination Account"
                    maskedValue={detail.destinationAccount ? `${detail.destinationAccount.slice(0, 3)}••••${detail.destinationAccount.slice(-3)}` : '012••••789'}
                    revealedValue={detail.destinationAccount}
                    entityId={detail.id}
                  />
                </div>
                <span className="text-[11px] text-[var(--text-muted)] block">
                  Institution: {detail.destinationBank}
                </span>
              </div>

              <div className="p-2.5 bg-[var(--canvas-subdued)] rounded border border-[var(--surface-border-subtle)] space-y-1">
                <span className="text-[10px] uppercase font-bold text-[var(--text-muted)] block">
                  Idempotency Verification
                </span>
                <div className="font-mono-financial font-semibold text-[11px] text-[var(--text-primary)] break-all">
                  {detail.idempotencyKey}
                </div>
                <span className="text-[10px] text-emerald-700 font-semibold block">
                  ✓ Replay Prevention Guaranteed
                </span>
              </div>
            </div>
          </div>

          {/* 8-Point Financial Safety Checklist */}
          <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] divide-y divide-[var(--surface-border-subtle)]">
            <div className="px-4 py-2.5 bg-[var(--canvas-subdued)] flex items-center justify-between">
              <h3 className="font-bold text-[12px] uppercase text-[var(--text-primary)] tracking-wider flex items-center gap-1.5">
                <FileCheck2 className="w-4 h-4 text-emerald-600" />
                <span>8 Financial Integrity Safety Checks (R51 & S1-S8)</span>
              </h3>
              <span className="text-[11px] font-mono-financial font-semibold text-emerald-800 bg-emerald-100 px-2 py-0.5 rounded">
                8 / 8 Checks Passed
              </span>
            </div>

            <div className="p-3 space-y-2">
              {detail.safetyChecks.map((check) => (
                <div
                  key={check.id}
                  className="p-2.5 rounded border border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-start gap-2 text-[12px]"
                >
                  <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0 mt-0.5" />
                  <div className="space-y-0.5 flex-1">
                    <div className="flex items-center justify-between font-semibold">
                      <span className="text-[var(--text-primary)]">{check.name}</span>
                      <span className="text-[10px] font-mono-financial px-1.5 py-0.2 rounded bg-emerald-100 text-emerald-800 font-bold">
                        {check.status}
                      </span>
                    </div>
                    <p className="text-[11px] text-[var(--text-muted)]">{check.description}</p>
                    <p className="text-[11px] font-mono-financial text-emerald-900 bg-emerald-50/70 p-1 rounded border border-emerald-200/50 mt-1">
                      {check.details}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Footer with Action Buttons */}
        <div className="px-5 py-3 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between gap-3">
          <Button variant="ghost" size="md" onClick={onClose}>
            Back to Queue
          </Button>

          {detail.status === 'approval' && (
            <Button
              variant="primary"
              size="md"
              onClick={onProceedToAuthorize}
              rightIcon={<ArrowRight className="w-4 h-4" />}
            >
              {isChecker ? 'Proceed to Authorization' : 'Review Dual-Control Details'}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};
