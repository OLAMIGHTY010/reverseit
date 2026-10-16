import React, { useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  Edit2,
  Building,
  CheckCircle2,
  Check,
  Receipt,
  ShieldCheck,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { ParsedAlertDetails } from './reverseitTypes';

export interface S3CheckDetailsProps {
  parsed: ParsedAlertDetails;
  onBack: () => void;
  onConfirmDetails: (confirmed: ParsedAlertDetails) => void;
}

export const S3CheckDetails: React.FC<S3CheckDetailsProps> = ({
  parsed,
  onBack,
  onConfirmDetails,
}) => {
  const [details, setDetails] = useState<ParsedAlertDetails>(parsed);
  const [isEditing, setIsEditing] = useState(false);
  const [isConfirmedByUser, setIsConfirmedByUser] = useState(true);

  const handleFieldChange = (field: keyof ParsedAlertDetails, value: string) => {
    setDetails((prev) => ({
      ...prev,
      [field]: value,
    }));
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
        <span>Back to alert input</span>
      </button>

      {/* Screen Title & Subtitle */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-sky-100 text-sky-800">
              Step 2 of 5
            </span>
            <span className="text-[12px] text-[var(--text-muted)]">Verification</span>
          </div>
          <h1 className="text-[22px] font-bold text-[var(--text-primary)]">
            Check Extracted Transaction Details
          </h1>
          <p className="text-[13px] text-[var(--text-secondary)]">
            Please verify the amounts and references extracted from your alert before we search bank switch records.
          </p>
        </div>

        <Button
          size="sm"
          variant="secondary"
          onClick={() => setIsEditing(!isEditing)}
          leftIcon={isEditing ? <Check className="w-3.5 h-3.5" /> : <Edit2 className="w-3.5 h-3.5" />}
        >
          {isEditing ? 'Save Changes' : 'Edit Fields'}
        </Button>
      </div>

      {/* Non-Wema External Bank Notice */}
      {details.isNonWemaAlert && (
        <div className="p-3.5 rounded-xl bg-blue-50 border border-blue-200 text-blue-950 text-[12px] flex items-start gap-3 shadow-2xs">
          <Building className="w-4 h-4 text-blue-700 shrink-0 mt-0.5" />
          <div>
            <strong className="font-bold">Issuing Bank: {details.issuingBank}</strong>
            <p className="text-[11px] text-blue-800 mt-0.5 leading-relaxed">
              We parsed this from an external bank alert. Smart Reversal enforces standard 
              Central Bank of Nigeria (CBN) regulatory countdowns and creates your formal complaint letter regardless of the bank.
            </p>
          </div>
        </div>
      )}

      {/* Digital Receipt Card */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-2xl overflow-hidden shadow-sm divide-y divide-[var(--surface-border-subtle)] text-[13px]">
        {/* Receipt Header Badge */}
        <div className="p-4 bg-gradient-to-r from-sky-50 to-indigo-50 border-b border-[var(--surface-border-subtle)] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Receipt className="w-4 h-4 text-sky-700" />
            <span className="font-bold text-[13px] text-sky-950">Verified Alert Receipt</span>
          </div>
          <span className="text-[11px] font-semibold text-emerald-800 bg-emerald-100 px-2.5 py-0.5 rounded-full border border-emerald-300 flex items-center gap-1">
            <CheckCircle2 className="w-3 h-3 text-emerald-600" />
            <span>High Confidence</span>
          </span>
        </div>

        {/* Debited Amount Highlight */}
        <div className="p-5 flex items-center justify-between bg-[var(--surface)]">
          <div>
            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
              Debited Amount
            </span>
            {isEditing ? (
              <div className="flex items-center gap-2 mt-1">
                <span className="font-bold text-[16px] text-[var(--text-primary)]">{details.currency}</span>
                <input
                  type="text"
                  value={details.amount}
                  onChange={(e) => handleFieldChange('amount', e.target.value)}
                  className="px-3 py-1.5 border rounded-lg text-[16px] font-mono-financial font-bold bg-[var(--canvas-subdued)] focus:outline-none focus:ring-2 focus:ring-sky-500"
                />
              </div>
            ) : (
              <span className="text-[26px] font-extrabold font-mono-financial text-[var(--text-primary)] tracking-tight">
                {details.currency} {details.amount}
              </span>
            )}
          </div>
          <span className="text-[11px] font-mono-financial px-2.5 py-1 rounded-full bg-emerald-50 text-emerald-800 border border-emerald-200 font-semibold">
            {details.confidence.amount}% Confidence
          </span>
        </div>

        {/* Account Number */}
        <div className="p-4 flex items-center justify-between">
          <div>
            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
              Source Account
            </span>
            {isEditing ? (
              <input
                type="text"
                value={details.accountTail}
                onChange={(e) => handleFieldChange('accountTail', e.target.value)}
                className="mt-1 px-3 py-1 border rounded-lg font-mono-financial text-[13px] bg-[var(--canvas-subdued)]"
                placeholder="Last 4 digits"
              />
            ) : (
              <span className="font-mono-financial text-[14px] font-semibold text-[var(--text-primary)]">
                •••• {details.accountTail}
              </span>
            )}
          </div>
          <span className="text-[11px] font-medium text-emerald-700 flex items-center gap-1">
            <ShieldCheck className="w-3.5 h-3.5" />
            <span>Masked for Privacy</span>
          </span>
        </div>

        {/* Date & Time */}
        <div className="p-4 flex items-center justify-between">
          <div>
            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
              Debit Timestamp
            </span>
            {isEditing ? (
              <input
                type="text"
                value={details.dateTime}
                onChange={(e) => handleFieldChange('dateTime', e.target.value)}
                className="mt-1 px-3 py-1 border rounded-lg font-mono-financial text-[13px] w-64 bg-[var(--canvas-subdued)]"
              />
            ) : (
              <span className="font-mono-financial text-[13px] text-[var(--text-primary)] font-medium">
                {details.dateTime}
              </span>
            )}
          </div>
          <span className="text-[11px] text-[var(--text-muted)]">West Africa Time (WAT)</span>
        </div>

        {/* Channel & Reference */}
        <div className="p-4 flex items-center justify-between">
          <div>
            <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
              Channel & Reference (RRN / Session ID)
            </span>
            {isEditing ? (
              <input
                type="text"
                value={details.reference}
                onChange={(e) => handleFieldChange('reference', e.target.value)}
                className="mt-1 px-3 py-1 border rounded-lg font-mono-financial text-[13px] w-64 bg-[var(--canvas-subdued)]"
              />
            ) : (
              <div className="flex items-center gap-2 mt-0.5">
                <span className="px-2 py-0.5 rounded-md bg-sky-100 text-sky-800 font-bold text-[11px]">
                  {details.channel}
                </span>
                <span className="font-mono-financial text-[13px] font-semibold text-[var(--text-primary)]">
                  {details.reference}
                </span>
              </div>
            )}
          </div>
          <span className="text-[11px] font-mono-financial px-2 py-0.5 rounded-full bg-emerald-50 text-emerald-800 border border-emerald-200">
            {details.confidence.reference}% Match
          </span>
        </div>

        {/* Narration */}
        <div className="p-4">
          <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
            Transaction Narration
          </span>
          {isEditing ? (
            <textarea
              rows={2}
              value={details.narration}
              onChange={(e) => handleFieldChange('narration', e.target.value)}
              className="mt-1 w-full px-3 py-1.5 border rounded-lg font-mono-financial text-[12px] bg-[var(--canvas-subdued)]"
            />
          ) : (
            <p className="font-mono-financial text-[12px] text-[var(--text-secondary)] mt-1 break-all bg-[var(--canvas-subdued)] p-2.5 rounded-lg border border-[var(--surface-border-subtle)]">
              {details.narration}
            </p>
          )}
        </div>
      </div>

      {/* Confirmation Checkbox */}
      <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl shadow-2xs">
        <label className="flex items-center gap-3 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={isConfirmedByUser}
            onChange={(e) => setIsConfirmedByUser(e.target.checked)}
            className="w-4 h-4 rounded text-sky-600 focus:ring-sky-500 border-slate-300 cursor-pointer shrink-0"
          />
          <span className="text-[13px] font-medium text-[var(--text-primary)]">
            I confirm that these extracted details accurately reflect the debit alert I received.
          </span>
        </label>
      </div>

      {/* Navigation Actions */}
      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" size="md" onClick={onBack}>
          Back
        </Button>
        <Button
          variant="primary"
          size="md"
          disabled={!isConfirmedByUser}
          onClick={() => onConfirmDetails(details)}
          rightIcon={<ArrowRight className="w-4 h-4" />}
        >
          Search Bank Switch Records &rarr;
        </Button>
      </div>
    </div>
  );
};
