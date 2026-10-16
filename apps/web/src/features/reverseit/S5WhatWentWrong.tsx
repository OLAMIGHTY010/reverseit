import React, { useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  AlertTriangle,
  Repeat,
  CreditCard,
  Building,
  HelpCircle,
  Sparkles,
  ArrowRightLeft,
  Clock,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { DisputeType, MatchedTransactionCandidate } from './reverseitTypes';

export interface S5WhatWentWrongProps {
  candidate: MatchedTransactionCandidate | null;
  onBack: () => void;
  onDisputeSelected: (type: DisputeType) => void;
}

export const S5WhatWentWrong: React.FC<S5WhatWentWrongProps> = ({
  candidate,
  onBack,
  onDisputeSelected,
}) => {
  // Preselect based on channel
  const initialType: DisputeType =
    candidate?.channel === 'POS'
      ? 'pos_web_failure'
      : candidate?.channel === 'ATM'
      ? 'atm_failure'
      : 'failed_transfer';

  const [selectedType, setSelectedType] = useState<DisputeType>(initialType);

  const options: Array<{
    type: DisputeType;
    title: string;
    description: string;
    icon: React.ReactNode;
    cbnSla: string;
    warning?: string;
  }> = [
    {
      type: 'failed_transfer',
      title: 'Failed Transfer (Debit Without Credit)',
      description: 'Money was deducted from your account, but the recipient confirms they never received value.',
      icon: <ArrowRightLeft className="w-5 h-5 text-sky-600" />,
      cbnSla: '24h CBN Mandate',
    },
    {
      type: 'duplicate_debit',
      title: 'Duplicate Debit',
      description: 'You were charged two or more times for a single payment, transfer, or checkout attempt.',
      icon: <Repeat className="w-5 h-5 text-indigo-600" />,
      cbnSla: '24h Core Correction',
    },
    {
      type: 'pos_web_failure',
      title: 'POS Terminal or Web Checkout Failure',
      description: 'The POS terminal printed "Declined" or timed out, but your account balance was still deducted.',
      icon: <CreditCard className="w-5 h-5 text-purple-600" />,
      cbnSla: '72h Acquirer Window',
    },
    {
      type: 'atm_failure',
      title: 'ATM Cash Not Dispensed / Incomplete Dispense',
      description: 'The ATM failed to dispense cash or dispensed less cash than debited from your balance.',
      icon: <Building className="w-5 h-5 text-emerald-600" />,
      cbnSla: '48h ATM Window',
    },
    {
      type: 'wrong_account_or_amount',
      title: 'Sent to Wrong Account or Transferred Wrong Amount',
      description: 'You mistakenly entered the wrong account number or transferred more than intended.',
      icon: <HelpCircle className="w-5 h-5 text-amber-600" />,
      cbnSla: 'Written Recall Protocol',
      warning:
        'Important: Recalling funds from a wrong recipient requires an interbank indemnity request and depends on available balance in the receiving account.',
    },
  ];

  return (
    <div className="max-w-2xl mx-auto py-2 px-2 sm:px-4 space-y-6 animate-in fade-in duration-200">
      {/* Back button */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1.5 text-[13px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-medium transition-colors cursor-pointer"
      >
        <ArrowLeft className="w-4 h-4" />
        <span>Back to candidates</span>
      </button>

      {/* Screen Title & Subtitle */}
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-sky-100 text-sky-800">
            Step 4 of 5
          </span>
          <span className="text-[12px] text-[var(--text-muted)]">Problem Classification</span>
        </div>
        <h1 className="text-[22px] font-bold text-[var(--text-primary)]">
          What Went Wrong with This Transaction?
        </h1>
        <p className="text-[13px] text-[var(--text-secondary)]">
          Select the exact issue you experienced. This assigns your statutory Central Bank of Nigeria (CBN) resolution clock.
        </p>
      </div>

      {/* AI Recommendation Pill */}
      <div className="p-3.5 bg-sky-50 border border-sky-200 rounded-xl flex items-center justify-between text-[12px] text-sky-950 shadow-2xs">
        <div className="flex items-center gap-2.5">
          <Sparkles className="w-4 h-4 text-sky-700 shrink-0" />
          <span>
            <strong>AI Suggestion: </strong>
            We recommend <strong>"{options.find((o) => o.type === initialType)?.title}"</strong> based on switch response telemetry.
          </span>
        </div>
        <span className="text-[10px] font-bold uppercase tracking-wider bg-sky-200/70 text-sky-800 px-2 py-0.5 rounded-full shrink-0">
          Auto-Detected
        </span>
      </div>

      {/* Dispute Options Radio List */}
      <div className="space-y-3">
        {options.map((opt) => {
          const isSelected = selectedType === opt.type;

          return (
            <div
              key={opt.type}
              onClick={() => setSelectedType(opt.type)}
              className={`p-4 sm:p-5 rounded-2xl border-2 transition-all cursor-pointer shadow-2xs ${
                isSelected
                  ? 'bg-[var(--surface)] border-sky-600 shadow-md ring-1 ring-sky-500'
                  : 'bg-[var(--surface)] border-[var(--surface-border)] hover:border-slate-400'
              }`}
            >
              <div className="flex items-start gap-3.5">
                <input
                  type="radio"
                  name="dispute-type"
                  checked={isSelected}
                  onChange={() => setSelectedType(opt.type)}
                  className="mt-1 w-4 h-4 text-sky-600 focus:ring-sky-500 cursor-pointer shrink-0"
                />

                <div className="space-y-1 flex-1">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <div className="p-1 rounded-md bg-[var(--canvas-subdued)]">
                        {opt.icon}
                      </div>
                      <span className="font-bold text-[14px] text-[var(--text-primary)]">
                        {opt.title}
                      </span>
                    </div>

                    <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-800 bg-emerald-50 px-2 py-0.5 rounded-md border border-emerald-200">
                      <Clock className="w-3 h-3 text-emerald-600" />
                      <span>{opt.cbnSla}</span>
                    </span>
                  </div>

                  <p className="text-[12px] text-[var(--text-muted)] leading-relaxed pl-8">
                    {opt.description}
                  </p>

                  {opt.warning && isSelected && (
                    <div className="mt-3 p-3 rounded-xl bg-amber-50 text-amber-950 border border-amber-200 text-[11px] flex items-start gap-2.5 ml-8">
                      <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
                      <span className="leading-relaxed">{opt.warning}</span>
                    </div>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Navigation Buttons */}
      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" size="md" onClick={onBack}>
          Back
        </Button>
        <Button
          variant="primary"
          size="md"
          onClick={() => onDisputeSelected(selectedType)}
          rightIcon={<ArrowRight className="w-4 h-4" />}
        >
          View Your Guaranteed Deadline &rarr;
        </Button>
      </div>
    </div>
  );
};
