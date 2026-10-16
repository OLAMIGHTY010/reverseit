import React, { useState, useEffect } from 'react';
import {
  Clock,
  ArrowLeft,
  ArrowRight,
  ShieldCheck,
  CheckCircle2,
  FileCheck2,
} from 'lucide-react';
import { Button } from '../../components/Button';
import { REGULATORY_CLOCKS } from './reverseitData';
import type { DisputeType, RegulatoryClockInfo } from './reverseitTypes';

export interface S6YourDeadlineProps {
  disputeType: DisputeType;
  onBack: () => void;
  onProceedToComplaint: (clock: RegulatoryClockInfo) => void;
}

export const S6YourDeadline: React.FC<S6YourDeadlineProps> = ({
  disputeType,
  onBack,
  onProceedToComplaint,
}) => {
  const clock = REGULATORY_CLOCKS[disputeType];

  // Live countdown state
  const [secondsRemaining, setSecondsRemaining] = useState(
    clock.bankTargetHours * 3600 - 1450
  );

  useEffect(() => {
    const timer = setInterval(() => {
      setSecondsRemaining((prev) => Math.max(0, prev - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  const hours = Math.floor(secondsRemaining / 3600);
  const minutes = Math.floor((secondsRemaining % 3600) / 60);
  const seconds = secondsRemaining % 60;

  return (
    <div className="max-w-2xl mx-auto py-2 px-2 sm:px-4 space-y-6 animate-in fade-in duration-200">
      {/* Back button */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1.5 text-[13px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-medium transition-colors cursor-pointer"
      >
        <ArrowLeft className="w-4 h-4" />
        <span>Change problem type</span>
      </button>

      {/* Screen Title & Subtitle */}
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-emerald-100 text-emerald-800">
            Step 5 of 5
          </span>
          <span className="text-[12px] text-[var(--text-muted)]">Official Statutory Clock</span>
        </div>
        <h1 className="text-[22px] font-bold text-[var(--text-primary)]">
          Your Guaranteed Resolution Deadline
        </h1>
        <p className="text-[13px] text-[var(--text-secondary)]">
          Under Central Bank of Nigeria (CBN) directives, your bank is mandated to resolve this transaction within the legal timeframe below.
        </p>
      </div>

      {/* Large Live Countdown Display */}
      <div className="p-6 sm:p-8 rounded-3xl bg-gradient-to-br from-slate-900 via-slate-850 to-indigo-950 text-white text-center shadow-xl border border-slate-700/60 space-y-5">
        <div className="inline-flex items-center gap-2 px-3.5 py-1 rounded-full bg-sky-500/20 text-sky-300 text-[12px] font-bold border border-sky-400/30">
          <Clock className="w-4 h-4 animate-pulse text-sky-400" />
          <span>Active Resolution Countdown (WAT)</span>
        </div>

        <div>
          <div className="text-[44px] sm:text-[56px] font-extrabold font-mono-financial tracking-tight text-sky-300 drop-shadow-sm">
            {hours.toString().padStart(2, '0')}h : {minutes.toString().padStart(2, '0')}m :{' '}
            {seconds.toString().padStart(2, '0')}s
          </div>
          <p className="text-[13px] text-slate-300 font-medium mt-1">
            Bank Accelerated Resolution Target
          </p>
        </div>

        {/* Dual Deadline Comparison Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-4 border-t border-slate-800 text-left text-[12px]">
          <div className="p-4 rounded-xl bg-slate-800/70 border border-slate-700/70 space-y-1">
            <span className="text-[10px] font-bold uppercase tracking-wider text-slate-400 block">
              1. Accelerated Bank Target
            </span>
            <span className="text-[15px] font-bold text-white font-mono-financial block">
              {clock.bankTargetLabel}
            </span>
            <span className="text-[11px] text-sky-400 block">
              Automated status check ladder running
            </span>
          </div>

          <div className="p-4 rounded-xl bg-slate-800/70 border border-slate-700/70 space-y-1">
            <span className="text-[10px] font-bold uppercase tracking-wider text-slate-400 block">
              2. CBN Maximum Legal Clock
            </span>
            <span className="text-[15px] font-bold text-emerald-400 font-mono-financial block">
              {clock.regulatoryLabel}
            </span>
            <span className="text-[11px] text-slate-300 block">
              Mandatory statutory refund limit
            </span>
          </div>
        </div>
      </div>

      {/* Statutory Source Transparency */}
      <div className="p-4 sm:p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-2xl space-y-2 text-[12px] shadow-2xs">
        <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
          <span className="flex items-center gap-1.5">
            <ShieldCheck className="w-4 h-4 text-emerald-600" />
            <span>Central Bank Directive Lineage</span>
          </span>
          <span className="text-emerald-800 bg-emerald-50 px-2.5 py-0.5 rounded-full border border-emerald-200 font-semibold">
            {clock.status} Policy
          </span>
        </div>
        <p className="text-[13px] text-[var(--text-primary)] font-semibold leading-relaxed">
          {clock.sourceAuthority}
        </p>
        <p className="text-[11px] text-[var(--text-muted)]">
          Effective Date: {clock.effectiveDate} &bull; Clock starts from debit detection time in West Africa Time.
        </p>
      </div>

      {/* What Happens Next Guidance */}
      <div className="p-5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-2xl space-y-3 text-[12px]">
        <h2 className="font-bold text-[13px] text-[var(--text-primary)] flex items-center gap-1.5">
          <FileCheck2 className="w-4 h-4 text-sky-600" />
          <span>What happens next?</span>
        </h2>
        <ul className="space-y-2 text-[var(--text-secondary)] pl-1 leading-relaxed">
          <li className="flex items-start gap-2">
            <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0 mt-0.5" />
            <span>We automatically compile a verified formal complaint citing your exact switch reference IDs.</span>
          </li>
          <li className="flex items-start gap-2">
            <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0 mt-0.5" />
            <span>Your complaint lands with priority as <strong>CUS001</strong> directly in the bank's exception workbench.</span>
          </li>
          <li className="flex items-start gap-2">
            <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0 mt-0.5" />
            <span>Once the payment switch confirms failure, your refund posts back to your account automatically.</span>
          </li>
        </ul>
      </div>

      {/* Navigation Buttons */}
      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" size="md" onClick={onBack}>
          Back
        </Button>
        <Button
          variant="primary"
          size="md"
          onClick={() => onProceedToComplaint(clock)}
          rightIcon={<ArrowRight className="w-4 h-4" />}
        >
          Review & Send Formal Complaint &rarr;
        </Button>
      </div>
    </div>
  );
};
