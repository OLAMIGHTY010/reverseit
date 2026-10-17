import React, { useState } from 'react';
import {
  Clock,
  AlertTriangle,
  Flame,
  PhoneCall,
  Building2,
  CheckCircle2,
  MessageSquarePlus,
  Send,
  X,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import { MaskedField } from '../../components/MaskedField';
import { useApp } from '../../context/AppContext';
import { apiClient } from '../../services/apiClient';
import { calculateDisputeAbuseProfile } from '../fraud/DisputeAbuseEngine';
import { DisputeAbuseBadge } from '../fraud/DisputeAbuseBadge';
import { DisputeAbuseModal } from '../fraud/DisputeAbuseModal';
import type { AgentQueryResult } from '../../types/domain';

export interface AgentActiveExceptionCardProps {
  result: AgentQueryResult;
  onRefresh: () => void;
}

export const AgentActiveExceptionCard: React.FC<AgentActiveExceptionCardProps> = ({
  result,
  onRefresh,
}) => {
  const { addToast } = useApp();
  const exc = result.exception;
  const [isLogCallOpen, setIsLogCallOpen] = useState(false);
  const [isAbuseModalOpen, setIsAbuseModalOpen] = useState(false);
  const [callerPhone, setCallerPhone] = useState(exc?.customerPhone || '+234 ');
  const [callNotes, setCallNotes] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!exc) return null;

  const handleLogCall = async (escalated: boolean) => {
    if (!callerPhone.trim()) {
      addToast({
        type: 'error',
        title: 'Validation Error',
        message: 'Please provide customer caller phone number.',
      });
      return;
    }

    setIsSubmitting(true);
    try {
      await apiClient.logAgentContact(
        exc.id,
        callerPhone,
        callNotes || 'Customer called regarding debit inquiry.',
        escalated,
        'Agent (Contact Centre Desk)'
      );

      addToast({
        type: escalated ? 'warning' : 'success',
        title: escalated ? 'Supervisor Escalation Queued' : 'Contact Logged to Dossier',
        message: escalated
          ? `Case ${exc.reference} escalated to Tier-2 Supervisor queue. Priority upgraded to CRITICAL.`
          : `Call linked to case ${exc.reference}. Repeat inquiry counter incremented.`,
      });

      setIsLogCallOpen(false);
      setCallNotes('');
      onRefresh();
    } catch {
      addToast({
        type: 'error',
        title: 'Logging Error',
        message: 'Could not log customer call. Please retry.',
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
      {/* 1. Header Banner */}
      <div className="p-4 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="p-1 rounded bg-amber-100 text-amber-800">
            <AlertTriangle className="w-4 h-4" />
          </span>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-mono-financial font-bold text-[14px] text-[var(--text-primary)]">
                {exc.reference}
              </span>
              <span className="text-[11px] font-mono-financial font-semibold px-1.5 py-0.2 rounded bg-amber-100 text-amber-900 border border-amber-300">
                {exc.classificationCode}
              </span>
              <span className="text-[11px] font-mono-financial font-semibold px-1.5 py-0.2 rounded bg-slate-100 text-slate-800 border border-slate-300">
                {exc.channel}
              </span>
            </div>
            <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
              Active Recon Case &bull; Status: <strong className="text-[var(--text-primary)] uppercase">{exc.status}</strong>
            </p>
          </div>
        </div>

        {/* Amount & Repeat Inquiry Counter */}
        <div className="flex items-center gap-2">
          <div className="text-right">
            <div className="text-[15px] font-bold font-mono-financial text-[var(--text-primary)]">
              {exc.currency} {Number(exc.amount).toLocaleString('en-US', { minimumFractionDigits: 2 })}
            </div>
            <div className="text-[11px] text-[var(--text-muted)] font-mono-financial">
              Originating Account: <span className="font-bold">{exc.accountNumber ? `${exc.accountNumber.slice(0, 3)}••••${exc.accountNumber.slice(-3)}` : '012••••789'}</span>
            </div>
          </div>

          <div
            className={clsx(
              'px-2.5 py-1 rounded border text-[11px] font-mono-financial font-semibold flex items-center gap-1.5',
              result.isThresholdReached || exc.repeatContactCount >= 3
                ? 'bg-rose-100 text-rose-900 border-rose-300 ring-2 ring-rose-400 animate-pulse'
                : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border)]'
            )}
          >
            <PhoneCall className="w-3.5 h-3.5" />
            <span>{exc.repeatContactCount || 1} Inquiries</span>
          </div>

          <DisputeAbuseBadge
            profile={
              exc.disputeAbuseProfile ||
              calculateDisputeAbuseProfile({
                amount: exc.amount,
                channel: exc.channel,
                repeatContactCount: exc.repeatContactCount,
              })
            }
            onClick={() => setIsAbuseModalOpen(true)}
            compact
          />
        </div>
      </div>

      {/* 2. Major Incident Banner if Applicable */}
      {result.incidentBanner && (
        <div className="p-3 bg-amber-500/10 border-l-4 border-amber-500 text-[12px] text-amber-950 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Flame className="w-4 h-4 text-amber-600 shrink-0" />
            <div>
              <span className="font-bold font-mono-financial mr-1">
                [{result.incidentBanner.incidentId}] Major Network Incident:
              </span>
              <span>{result.incidentBanner.title}</span>
            </div>
          </div>
          <div className="text-[11px] font-mono-financial text-amber-800 bg-amber-100 px-2 py-0.5 rounded border border-amber-300">
            {result.incidentBanner.eta}
          </div>
        </div>
      )}

      {/* 3. Customer-Safe Status & SLA Countdown Panel */}
      <div className="p-4 space-y-4 text-[13px]">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Safe Status */}
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded space-y-1">
            <span className="text-[11px] font-bold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-1">
              <CheckCircle2 className="w-3.5 h-3.5 text-sky-600" /> Customer-Safe Situation
            </span>
            <p className="text-[13px] text-[var(--text-primary)] font-medium leading-relaxed">
              {result.customerSafeStatusText}
            </p>
          </div>

          {/* Expected Next Step & SLA */}
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded space-y-1">
            <span className="text-[11px] font-bold text-[var(--text-muted)] uppercase tracking-wider flex items-center gap-1">
              <Clock className="w-3.5 h-3.5 text-amber-600" /> Next Safe Action & SLA Window
            </span>
            <p className="text-[13px] text-[var(--text-primary)] font-medium leading-relaxed">
              {result.expectedNextStep || 'Awaiting counterparty batch settlement acknowledgment.'}
            </p>
            <p className="text-[11px] text-[var(--text-muted)] font-mono-financial pt-1 border-t border-[var(--surface-border-subtle)]">
              {result.timeEstimate || 'Target turnaround: within CBN 72h window.'}
            </p>
          </div>
        </div>

        {/* 4. Masked Customer Data with Audited Reveal */}
        <div className="p-3 border border-[var(--surface-border-subtle)] rounded bg-[var(--surface)] space-y-2">
          <div className="flex items-center justify-between text-[11px] text-[var(--text-muted)] font-bold uppercase tracking-wider">
            <span>Customer Attributes (Masked for Privacy)</span>
            <span>Audit Level: STRICT</span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-[12px]">
            <div>
              <span className="text-[11px] text-[var(--text-muted)] block">Customer Name</span>
              <span className="font-semibold text-[var(--text-primary)]">
                {exc.customerName || 'Amina Bello'}
              </span>
            </div>

            <div>
              <span className="text-[11px] text-[var(--text-muted)] block">Account Number</span>
              <MaskedField
                fieldLabel="Account Number"
                maskedValue={exc.accountNumber ? `${exc.accountNumber.slice(0, 3)}••••${exc.accountNumber.slice(-3)}` : '012••••789'}
                revealedValue={exc.accountNumber || '0123456789'}
                entityId={exc.id}
              />
            </div>

            <div>
              <span className="text-[11px] text-[var(--text-muted)] block">Phone Number</span>
              <MaskedField
                fieldLabel="Customer Phone"
                maskedValue={exc.customerPhone ? `${exc.customerPhone.slice(0, 4)} •••• ${exc.customerPhone.slice(-4)}` : '+234 •••• 2233'}
                revealedValue={exc.customerPhone || '+234 809 111 2233'}
                entityId={exc.id}
              />
            </div>
          </div>
        </div>

        {/* 5. Repeat Inquiry & Escalation Actions Bar */}
        <div className="pt-2 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--surface-border-subtle)]">
          <div className="text-[12px] text-[var(--text-muted)] flex items-center gap-1.5">
            <Building2 className="w-3.5 h-3.5 text-sky-600" />
            <span>Assigned Team: <strong className="text-[var(--text-primary)]">{exc.team}</strong></span>
          </div>

          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="secondary"
              onClick={() => setIsLogCallOpen(true)}
              leftIcon={<MessageSquarePlus className="w-3.5 h-3.5" />}
            >
              Log Customer Call
            </Button>

            {/* Supervisor Escalation Button: Enabled if repeat inquiry threshold is reached */}
            {(result.isThresholdReached || exc.repeatContactCount >= 3) && (
              <Button
                size="sm"
                variant="danger"
                onClick={() => handleLogCall(true)}
                disabled={isSubmitting}
                leftIcon={<Flame className="w-3.5 h-3.5" />}
              >
                Escalate to Supervisor Queue
              </Button>
            )}
          </div>
        </div>
      </div>

      {/* 6. Quick Modal to Log Call */}
      {isLogCallOpen && (
        <div className="p-4 bg-[var(--canvas-subdued)] border-t border-[var(--surface-border)] space-y-3">
          <div className="flex items-center justify-between font-bold text-[13px] text-[var(--text-primary)]">
            <span className="flex items-center gap-1.5">
              <PhoneCall className="w-4 h-4 text-sky-600" /> Log Customer Contact
            </span>
            <button
              type="button"
              onClick={() => setIsLogCallOpen(false)}
              className="text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-[12px]">
            <div>
              <label htmlFor="caller-phone-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                Caller Phone Number
              </label>
              <input
                id="caller-phone-input"
                type="text"
                value={callerPhone}
                onChange={(e) => setCallerPhone(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            <div>
              <label htmlFor="caller-notes-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                Call Remarks / Customer Feedback
              </label>
              <input
                id="caller-notes-input"
                type="text"
                value={callNotes}
                onChange={(e) => setCallNotes(e.target.value)}
                placeholder="e.g. Caller inquired on delay, read approved script"
                className="w-full px-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-1">
            <Button size="sm" variant="secondary" onClick={() => setIsLogCallOpen(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              variant="primary"
              disabled={isSubmitting}
              onClick={() => handleLogCall(false)}
              leftIcon={<Send className="w-3.5 h-3.5" />}
            >
              {isSubmitting ? 'Logging...' : 'Save & Link Call to Case'}
            </Button>
          </div>
        </div>
      )}

      {/* Part 2: Customer Dispute Abuse Dossier Modal */}
      {isAbuseModalOpen && (
        <DisputeAbuseModal
          isOpen={isAbuseModalOpen}
          onClose={() => setIsAbuseModalOpen(false)}
          exception={exc}
          profile={
            exc.disputeAbuseProfile ||
            calculateDisputeAbuseProfile({
              amount: exc.amount,
              channel: exc.channel,
              repeatContactCount: exc.repeatContactCount,
            })
          }
        />
      )}
    </div>
  );
};
