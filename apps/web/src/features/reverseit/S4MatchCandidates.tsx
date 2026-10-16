import React, { useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  AlertCircle,
  HelpCircle,
  Sparkles,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { ParsedAlertDetails, MatchedTransactionCandidate } from './reverseitTypes';

export interface S4MatchCandidatesProps {
  parsed: ParsedAlertDetails;
  onBack: () => void;
  onCandidateSelected: (candidate: MatchedTransactionCandidate | null) => void;
}

export const S4MatchCandidates: React.FC<S4MatchCandidatesProps> = ({
  parsed,
  onBack,
  onCandidateSelected,
}) => {
  // Generate realistic candidates based on parsed alert
  const [candidates] = useState<MatchedTransactionCandidate[]>([
    {
      id: 'tx-cand-01',
      reference: parsed.reference || 'NIP-DEMO-7781002',
      amount: parsed.amount,
      currency: parsed.currency,
      channel: parsed.channel || 'NIP',
      dateTime: parsed.dateTime,
      beneficiaryMasked: 'CHINEDU OKONKWO (GTB •••• 9921)',
      matchScore: 98,
      matchConfidence: 'HIGH',
      matchReason: 'Exact match on reference, amount, and account tail within 2 minutes of debit.',
      status: 'FAILED',
    },
    {
      id: 'tx-cand-02',
      reference: 'NIP-DEMO-7780998',
      amount: parsed.amount,
      currency: parsed.currency,
      channel: parsed.channel || 'NIP',
      dateTime: '14-Oct-2026 09:10:02 WAT',
      beneficiaryMasked: 'CHINEDU OKONKWO (GTB •••• 9921)',
      matchScore: 82,
      matchConfidence: 'MEDIUM',
      matchReason: 'Same beneficiary and amount 2 minutes prior; suspected retry candidate.',
      status: 'PENDING',
    },
  ]);

  const [selectedId, setSelectedId] = useState<string | 'NONE'>(candidates[0]?.id || 'NONE');

  const handleProceed = () => {
    if (selectedId === 'NONE') {
      onCandidateSelected(null);
    } else {
      const selected = candidates.find((c) => c.id === selectedId) || null;
      onCandidateSelected(selected);
    }
  };

  return (
    <div className="max-w-2xl mx-auto py-2 px-2 sm:px-4 space-y-6 animate-in fade-in duration-200">
      {/* Back button */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1.5 text-[13px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-medium transition-colors cursor-pointer"
      >
        <ArrowLeft className="w-4 h-4" />
        <span>Back to extracted details</span>
      </button>

      {/* Screen Title & Subtitle */}
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-sky-100 text-sky-800">
            Step 3 of 5
          </span>
          <span className="text-[12px] text-[var(--text-muted)]">Switch Correlation</span>
        </div>
        <h1 className="text-[22px] font-bold text-[var(--text-primary)]">
          We Found Matching Transactions on the Switch
        </h1>
        <p className="text-[13px] text-[var(--text-secondary)]">
          Our automated reconciliation engine cross-checked your alert against interbank records. 
          Please select the exact transaction you wish to dispute.
        </p>
      </div>

      {/* Candidate Cards List */}
      <div className="space-y-3">
        {candidates.map((cand, idx) => {
          const isSelected = selectedId === cand.id;

          return (
            <div
              key={cand.id}
              onClick={() => setSelectedId(cand.id)}
              className={`p-5 rounded-2xl border-2 transition-all cursor-pointer relative shadow-2xs ${
                isSelected
                  ? 'bg-sky-50/70 border-sky-600 shadow-md ring-1 ring-sky-500'
                  : 'bg-[var(--surface)] border-[var(--surface-border)] hover:border-slate-400'
              }`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-extrabold text-[18px] font-mono-financial text-[var(--text-primary)]">
                      {cand.currency} {cand.amount}
                    </span>
                    <span className="px-2 py-0.5 rounded-md text-[10px] font-bold uppercase tracking-wider bg-sky-100 text-sky-800">
                      {cand.channel} Channel
                    </span>
                    {idx === 0 && (
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-100 text-emerald-800 border border-emerald-300">
                        <Sparkles className="w-3 h-3 text-emerald-600" />
                        <span>Recommended Match</span>
                      </span>
                    )}
                  </div>

                  <div className="text-[12px] font-mono-financial text-[var(--text-muted)]">
                    Ref: <span className="font-semibold text-[var(--text-secondary)]">{cand.reference}</span> &bull; {cand.dateTime}
                  </div>

                  <div className="text-[13px] font-medium text-[var(--text-primary)] pt-0.5">
                    Beneficiary: <span className="font-semibold">{cand.beneficiaryMasked}</span>
                  </div>
                </div>

                <div className="text-right shrink-0">
                  <span className="px-2.5 py-1 rounded-full text-[11px] font-bold font-mono-financial bg-emerald-100 text-emerald-800 inline-block">
                    {cand.matchScore}% Match
                  </span>
                  <div className="mt-1 flex items-center justify-end gap-1 text-[11px] text-rose-700 font-bold">
                    <AlertCircle className="w-3.5 h-3.5 text-rose-600" />
                    <span>Switch: {cand.status}</span>
                  </div>
                </div>
              </div>

              {/* Match rationale */}
              <div className="mt-3 pt-2.5 border-t border-slate-200/80 flex items-center justify-between text-[11px]">
                <span className="text-[var(--text-secondary)] flex items-center gap-1.5">
                  <CheckCircle2 className="w-3.5 h-3.5 text-sky-600 shrink-0" />
                  <span>{cand.matchReason}</span>
                </span>
                <span className={`font-semibold ${isSelected ? 'text-sky-700' : 'text-[var(--text-muted)]'}`}>
                  {isSelected ? 'Selected' : 'Click to select'}
                </span>
              </div>
            </div>
          );
        })}

        {/* None of these match option */}
        <div
          onClick={() => setSelectedId('NONE')}
          className={`p-4 rounded-2xl border-2 transition-all cursor-pointer shadow-2xs ${
            selectedId === 'NONE'
              ? 'bg-amber-50/70 border-amber-500 shadow-md ring-1 ring-amber-400'
              : 'bg-[var(--surface)] border-[var(--surface-border)] hover:border-slate-400'
          }`}
        >
          <div className="flex items-start gap-3">
            <div className="p-2 rounded-lg bg-amber-100 text-amber-800 shrink-0 mt-0.5">
              <HelpCircle className="w-4 h-4" />
            </div>
            <div className="space-y-0.5 flex-1">
              <span className="text-[14px] font-bold text-[var(--text-primary)] block">
                None of these match my transaction
              </span>
              <p className="text-[12px] text-[var(--text-secondary)] leading-relaxed">
                If the switch has not yet published this session, we will file this as an <strong>Unmatched Exception Claim (CUS001)</strong>. 
                Our bank operations team will pull the internal core banking ledger directly.
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Navigation Buttons */}
      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" size="md" onClick={onBack}>
          Back
        </Button>
        <Button
          variant="primary"
          size="md"
          onClick={handleProceed}
          rightIcon={<ArrowRight className="w-4 h-4" />}
        >
          Confirm & Explain What Happened &rarr;
        </Button>
      </div>
    </div>
  );
};
