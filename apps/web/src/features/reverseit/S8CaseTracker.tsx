import React, { useState } from 'react';
import {
  Clock,
  CheckCircle2,
  ArrowLeft,
  ShieldCheck,
  ExternalLink,
  HelpCircle,
  MessageSquare,
  FileCheck2,
  Zap,
  Star,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { CustomerDisputeCase } from './reverseitTypes';
import { PizzaTrackerStepper } from './PizzaTrackerStepper';
import { CustomerNotificationHub } from './CustomerNotificationHub';
import { CreditAdviceReceiptModal } from './CreditAdviceReceiptModal';

export interface S8CaseTrackerProps {
  disputeCase: CustomerDisputeCase;
  onBack: () => void;
  onViewEscalation: () => void;
}

export const S8CaseTracker: React.FC<S8CaseTrackerProps> = ({
  disputeCase,
  onBack,
  onViewEscalation,
}) => {
  const [isNotificationHubOpen, setIsNotificationHubOpen] = useState(false);
  const [isReceiptModalOpen, setIsReceiptModalOpen] = useState(false);
  const [userRating, setUserRating] = useState<number>(5);
  const [ratingSubmitted, setRatingSubmitted] = useState<boolean>(false);

  return (
    <div className="max-w-4xl mx-auto py-2 px-2 sm:px-4 space-y-6 animate-in fade-in duration-200">
      {/* Back button and quick actions */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1.5 text-[13px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-medium transition-colors cursor-pointer"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>Back to My Cases</span>
        </button>

        {/* Omnichannel and Receipt Action Triggers */}
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            leftIcon={<MessageSquare className="w-3.5 h-3.5 text-emerald-600" />}
            onClick={() => setIsNotificationHubOpen(true)}
          >
            Notification Hub (WhatsApp/SMS)
          </Button>
          <Button
            size="sm"
            variant="primary"
            leftIcon={<FileCheck2 className="w-3.5 h-3.5" />}
            onClick={() => setIsReceiptModalOpen(true)}
          >
            Credit Advice Slip
          </Button>
        </div>
      </div>

      {/* Case Header Card */}
      <div className="p-5 sm:p-6 bg-[var(--surface)] border border-[var(--surface-border)] rounded-3xl shadow-xs space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <h1 className="text-[22px] font-extrabold text-[var(--text-primary)] font-mono-financial">
                {disputeCase.caseNumber}
              </h1>
              <span className="px-2.5 py-0.5 rounded-full text-[11px] font-bold uppercase tracking-wider bg-sky-100 text-sky-800 border border-sky-300">
                {disputeCase.channel} Network
              </span>
            </div>
            <p className="text-[12px] text-[var(--text-muted)] font-mono-financial">
              Switch RRN: <span className="text-[var(--text-secondary)] font-bold">{disputeCase.reference}</span> &bull; Reported {new Date(disputeCase.reportedAt).toLocaleDateString()}
            </p>
          </div>

          <div className="text-right">
            <span className="text-[26px] font-black font-mono-financial text-[var(--text-primary)] block">
              {disputeCase.currency} {disputeCase.amount}
            </span>
            <span className="text-[11px] font-bold text-emerald-800 bg-emerald-50 px-3 py-0.5 rounded-full border border-emerald-300 inline-block mt-0.5">
              {disputeCase.statusLabel}
            </span>
          </div>
        </div>

        {/* Instant Provisional Credit Banner */}
        <div className="p-4 rounded-2xl bg-gradient-to-r from-emerald-500/10 via-teal-500/10 to-sky-500/10 border border-emerald-500/30 text-[12px] flex items-start gap-3.5">
          <div className="w-8 h-8 rounded-xl bg-emerald-600 text-white flex items-center justify-center shrink-0 shadow-xs">
            <Zap className="w-4 h-4 fill-white" />
          </div>
          <div className="space-y-0.5 flex-1">
            <div className="flex items-center justify-between">
              <strong className="font-extrabold text-emerald-950 dark:text-emerald-300 text-[13px]">
                Instant Provisional Credit Protection Active
              </strong>
              <span className="text-[10px] font-bold uppercase tracking-wider text-emerald-700 bg-emerald-100 dark:bg-emerald-900/50 px-2 py-0.5 rounded-full">
                Zero Customer Liability
              </span>
            </div>
            <p className="text-emerald-900/90 dark:text-emerald-200 leading-relaxed text-[12px]">
              {disputeCase.customerSafeStatusText} Switch error confirmed. Under Central Bank Consumer Protection Regulations, your funds are fully guaranteed and protected.
            </p>
          </div>
        </div>
      </div>

      {/* Real-Time "Pizza-Tracker" Stepper */}
      <PizzaTrackerStepper disputeCase={disputeCase} />

      {/* SLA Windows Card */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-[12px]">
        <div className="p-5 rounded-3xl bg-[var(--surface)] border border-[var(--surface-border)] space-y-1.5 shadow-2xs">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
            <Clock className="w-3.5 h-3.5 text-sky-600" />
            <span>Bank Resolution Commitment</span>
          </span>
          <span className="text-[18px] font-extrabold font-mono-financial text-[var(--text-primary)] block">
            {disputeCase.timeRemainingText}
          </span>
          <p className="text-[11px] text-[var(--text-muted)]">
            Guaranteed Target: {new Date(disputeCase.bankTargetDeadline).toLocaleTimeString()} WAT
          </p>
        </div>

        <div className="p-5 rounded-3xl bg-[var(--surface)] border border-[var(--surface-border)] space-y-1.5 shadow-2xs">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-600" />
            <span>CBN Statutory Maximum</span>
          </span>
          <span className="text-[18px] font-extrabold font-mono-financial text-emerald-700 block">
            Statutory T+1 Protection
          </span>
          <p className="text-[11px] text-[var(--text-muted)]">
            Mandate Deadline: {new Date(disputeCase.regulatoryDeadline).toLocaleTimeString()} WAT
          </p>
        </div>
      </div>

      {/* Investigation Audit Trail */}
      <div className="p-5 sm:p-6 bg-[var(--surface)] border border-[var(--surface-border)] rounded-3xl space-y-4 shadow-xs">
        <h2 className="text-[15px] font-bold text-[var(--text-primary)] flex items-center gap-2">
          <span>Case Investigation Audit Trail</span>
          <span className="text-[11px] text-[var(--text-muted)] font-normal">
            (Cryptographically Sealed Logs)
          </span>
        </h2>

        <div className="space-y-6 relative before:absolute before:inset-0 before:left-3.5 before:w-0.5 before:bg-[var(--canvas-subdued)]">
          {disputeCase.history.map((hist, i) => (
            <div key={hist.stage} className="relative flex items-start gap-4">
              <span
                className={`w-7 h-7 rounded-full flex items-center justify-center shrink-0 z-10 font-bold text-[11px] transition-all ${
                  hist.verified
                    ? 'bg-emerald-600 text-white shadow-xs'
                    : 'bg-slate-100 text-slate-400 border border-slate-300'
                }`}
              >
                {hist.verified ? <CheckCircle2 className="w-4 h-4 stroke-[2.5]" /> : i + 1}
              </span>
              <div className="space-y-0.5 pt-0.5 flex-1">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="font-bold text-[13px] text-[var(--text-primary)]">
                    {hist.label}
                  </span>
                  <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                    {hist.timestamp}
                  </span>
                </div>
                <p className="text-[12px] text-[var(--text-secondary)] leading-relaxed">
                  {hist.details}
                </p>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Customer Satisfaction Feedback */}
      <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-3xl flex flex-wrap items-center justify-between gap-4 shadow-xs">
        <div className="space-y-1">
          <span className="font-bold text-[13px] text-[var(--text-primary)] block">
            How satisfied are you with this dispute resolution?
          </span>
          <p className="text-[11px] text-[var(--text-muted)]">
            Your feedback helps us continuously improve instant reversal response times.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1">
            {[1, 2, 3, 4, 5].map((star) => (
              <button
                key={star}
                type="button"
                onClick={() => {
                  setUserRating(star);
                  setRatingSubmitted(true);
                }}
                className="p-1 cursor-pointer transition-transform hover:scale-110"
                aria-label={`Rate ${star} stars`}
              >
                <Star
                  className={`w-5 h-5 ${
                    star <= userRating
                      ? 'text-amber-400 fill-amber-400'
                      : 'text-slate-300 dark:text-slate-600'
                  }`}
                />
              </button>
            ))}
          </div>
          {ratingSubmitted && (
            <span className="text-[11px] font-bold text-emerald-600 bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-200">
              Thank you!
            </span>
          )}
        </div>
      </div>

      {/* Escalation Ladder Action */}
      <div className="p-4 sm:p-5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-3xl flex flex-wrap items-center justify-between gap-3 shadow-2xs">
        <div className="space-y-0.5">
          <span className="text-[13px] font-bold text-[var(--text-primary)] flex items-center gap-1.5">
            <HelpCircle className="w-4 h-4 text-sky-600" />
            <span>Resolution Past Target or Stalled?</span>
          </span>
          <p className="text-[11px] text-[var(--text-muted)]">
            View the official 3-step escalation ladder (Bank Ombudsman & CBN Consumer Protection).
          </p>
        </div>
        <Button
          size="sm"
          variant="secondary"
          onClick={onViewEscalation}
          rightIcon={<ExternalLink className="w-3.5 h-3.5" />}
        >
          View Escalation Ladder &rarr;
        </Button>
      </div>

      {/* Omnichannel Notification Hub Modal */}
      <CustomerNotificationHub
        disputeCase={disputeCase}
        isOpen={isNotificationHubOpen}
        onClose={() => setIsNotificationHubOpen(false)}
      />

      {/* Electronic Credit Advice Slip Receipt Modal */}
      <CreditAdviceReceiptModal
        disputeCase={disputeCase}
        isOpen={isReceiptModalOpen}
        onClose={() => setIsReceiptModalOpen(false)}
      />
    </div>
  );
};
