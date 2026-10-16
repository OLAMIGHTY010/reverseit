import React from 'react';
import {
  MessageSquare,
  CheckCircle2,
  XCircle,
  Copy,
} from 'lucide-react';
import type { CustomerReportedCaseDetails } from '../../types/domain';

interface CustomerReportedAlertCardProps {
  details: CustomerReportedCaseDetails;
}

export const CustomerReportedAlertCard: React.FC<CustomerReportedAlertCardProps> = ({ details }) => {
  return (
    <div className="border border-indigo-200 rounded-md bg-indigo-50/30 shadow-xs divide-y divide-indigo-100 text-[12px]">
      {/* Header */}
      <div className="p-3 bg-indigo-50/70 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <MessageSquare className="w-4 h-4 text-indigo-700" />
          <h4 className="font-bold text-[13px] text-indigo-950">
            Customer-Reported Dispute Dossier (Scope v2)
          </h4>
        </div>
        <span className="font-mono-financial font-extrabold text-[10px] px-2 py-0.5 rounded bg-rose-100 text-rose-800 border border-rose-300">
          {details.repeatContactCount}x Repeat Inquiries
        </span>
      </div>

      {/* Customer Complaint Text */}
      <div className="p-3 space-y-1">
        <span className="text-[10px] font-bold uppercase tracking-wider text-indigo-900/80">
          Customer Stated Dispute Narrative
        </span>
        <blockquote className="p-2.5 rounded bg-white/80 border border-indigo-200 text-indigo-950 italic text-[12px] leading-relaxed shadow-2xs">
          "{details.complaintText}"
        </blockquote>
      </div>

      {/* Raw Masked Alert Payload */}
      <div className="p-3 space-y-1.5 font-mono-financial">
        <div className="flex items-center justify-between text-[10px] text-indigo-900 uppercase font-bold tracking-wider">
          <span>Raw Inbound Alert (Masked)</span>
          <button
            type="button"
            onClick={() => navigator.clipboard.writeText(details.rawAlertMasked)}
            className="text-indigo-600 hover:text-indigo-900 flex items-center gap-0.5"
          >
            <Copy className="w-3 h-3" />
            <span>Copy</span>
          </button>
        </div>
        <div className="p-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[11px] text-[var(--text-primary)] whitespace-pre-wrap">
          {details.rawAlertMasked}
        </div>
      </div>

      {/* Parsed Fields with Confidence & Tamper Flags */}
      <div className="p-3 grid grid-cols-1 md:grid-cols-2 gap-3">
        {/* Parsed Fields */}
        <div className="space-y-1.5">
          <span className="text-[10px] font-bold uppercase tracking-wider text-indigo-900">
            NLP Parsed Entities
          </span>
          <div className="space-y-1">
            {details.parsedFields.map((field) => (
              <div
                key={field.field}
                className="p-1.5 rounded bg-white/70 border border-indigo-100 flex items-center justify-between text-[11px]"
              >
                <span className="text-indigo-950 font-medium">{field.field}:</span>
                <div className="flex items-center gap-1.5 font-mono-financial">
                  <span className="font-bold text-[var(--text-primary)]">{field.value}</span>
                  <span className="text-[9px] px-1 py-0.2 rounded bg-sky-100 text-sky-800 border border-sky-200">
                    {Math.round(field.confidence * 100)}%
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Cryptographic Tamper Flags */}
        <div className="space-y-1.5">
          <span className="text-[10px] font-bold uppercase tracking-wider text-indigo-900">
            Tamper Integrity Verification
          </span>
          <div className="space-y-1">
            {details.tamperFlags.map((flag) => (
              <div
                key={flag.check}
                className="p-1.5 rounded bg-white/70 border border-indigo-100 space-y-0.5 text-[11px]"
              >
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-indigo-950 flex items-center gap-1">
                    {flag.passed ? (
                      <CheckCircle2 className="w-3 h-3 text-emerald-600" />
                    ) : (
                      <XCircle className="w-3 h-3 text-rose-600" />
                    )}
                    {flag.check}
                  </span>
                  <span
                    className={`text-[9px] font-bold font-mono-financial px-1 py-0.2 rounded ${
                      flag.passed
                        ? 'bg-emerald-100 text-emerald-900'
                        : 'bg-rose-100 text-rose-900'
                    }`}
                  >
                    {flag.passed ? 'PASSED' : 'FLAGGED'}
                  </span>
                </div>
                <p className="text-[10px] text-[var(--text-muted)] leading-tight pl-4">
                  {flag.note}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Match Candidates */}
      <div className="p-3 space-y-1.5">
        <span className="text-[10px] font-bold uppercase tracking-wider text-indigo-900">
          Ledger Match Candidates
        </span>
        <div className="space-y-1">
          {details.matchCandidates.map((cand) => (
            <div
              key={cand.reference}
              className="p-2 rounded bg-white border border-indigo-200 flex items-center justify-between font-mono-financial text-[11px]"
            >
              <div>
                <strong className="text-indigo-950">{cand.reference}</strong>
                <span className="text-[var(--text-muted)] ml-2">{cand.timestamp}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-[var(--text-primary)]">{cand.amount}</span>
                <span className="text-[10px] font-extrabold px-1.5 py-0.2 rounded bg-emerald-100 text-emerald-900 border border-emerald-300">
                  {Math.round(cand.matchConfidence * 100)}% Match
                </span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
