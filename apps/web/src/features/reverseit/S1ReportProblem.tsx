import React from 'react';
import {
  FileText,
  UploadCloud,
  Clock,
  ShieldCheck,
  CheckCircle2,
  ArrowRight,
  Zap,
} from 'lucide-react';
import type { IntakeMode } from './reverseitTypes';

export interface S1ReportProblemProps {
  onSelectMode: (mode: IntakeMode) => void;
  onViewMyCases: () => void;
  activeCaseCount: number;
}

export const S1ReportProblem: React.FC<S1ReportProblemProps> = ({
  onSelectMode,
  onViewMyCases,
  activeCaseCount,
}) => {
  return (
    <div className="max-w-3xl mx-auto py-4 px-2 sm:px-4 space-y-8 animate-in fade-in duration-300">
      {/* Primary Hero Header */}
      <div className="text-center space-y-3 pt-2">
        <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-sky-50 text-sky-800 text-[12px] font-semibold border border-sky-200">
          <Zap className="w-3.5 h-3.5 text-sky-600" />
          <span>Self-Service Dispute Assistant</span>
        </div>

        <h1 className="text-[28px] sm:text-[34px] font-extrabold text-[var(--text-primary)] tracking-tight leading-tight">
          Money left your account, but something went wrong?
        </h1>

        <p className="text-[14px] sm:text-[15px] text-[var(--text-secondary)] max-w-xl mx-auto leading-relaxed">
          Don't stress. Report failed transfers, POS timeouts, or duplicate debits in under 60 seconds. 
          We verify your transaction with live banking switch records and enforce your official Central Bank of Nigeria (CBN) refund rights.
        </p>
      </div>

      {/* Two Big, Inviting Action Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Choice 1: Paste Alert Text */}
        <button
          type="button"
          onClick={() => onSelectMode('PASTE_TEXT')}
          className="p-6 bg-[var(--surface)] border-2 border-[var(--surface-border)] hover:border-sky-500 rounded-2xl shadow-xs hover:shadow-lg transition-all text-left flex flex-col justify-between space-y-5 group cursor-pointer focus:outline-none focus:ring-2 focus:ring-sky-500 relative overflow-hidden"
        >
          <div className="absolute top-3 right-3 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider bg-sky-100 text-sky-800 border border-sky-200">
            Recommended
          </div>

          <div className="flex items-center gap-4">
            <div className="p-3.5 w-13 h-13 rounded-xl bg-sky-50 text-sky-700 flex items-center justify-center group-hover:scale-105 group-hover:bg-sky-100 transition-all shadow-xs">
              <FileText className="w-7 h-7" />
            </div>
            <div>
              <h2 className="text-[17px] font-bold text-[var(--text-primary)] group-hover:text-sky-700 transition-colors">
                Paste Debit Alert
              </h2>
              <span className="text-[11px] text-sky-600 font-medium">SMS or Email Notification</span>
            </div>
          </div>

          <p className="text-[13px] text-[var(--text-muted)] leading-relaxed">
            Copy and paste the debit alert message sent to your phone or email. Our smart parser instantly extracts the amount, reference, and timestamp.
          </p>

          <div className="pt-2 border-t border-[var(--surface-border-subtle)] flex items-center justify-between">
            <span className="text-[12px] font-bold text-sky-700 group-hover:translate-x-1 transition-transform flex items-center gap-1.5">
              <span>Start with Alert Text</span>
              <ArrowRight className="w-4 h-4" />
            </span>
            <span className="text-[11px] text-[var(--text-muted)]">Takes ~30 seconds</span>
          </div>
        </button>

        {/* Choice 2: Upload Screenshot */}
        <button
          type="button"
          onClick={() => onSelectMode('UPLOAD_SCREENSHOT')}
          className="p-6 bg-[var(--surface)] border-2 border-[var(--surface-border)] hover:border-indigo-400 rounded-2xl shadow-xs hover:shadow-lg transition-all text-left flex flex-col justify-between space-y-5 group cursor-pointer focus:outline-none focus:ring-2 focus:ring-indigo-400 relative overflow-hidden"
        >
          <div className="absolute top-3 right-3 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider bg-indigo-50 text-indigo-700 border border-indigo-200">
            Image / Receipt
          </div>

          <div className="flex items-center gap-4">
            <div className="p-3.5 w-13 h-13 rounded-xl bg-indigo-50 text-indigo-600 flex items-center justify-center group-hover:scale-105 group-hover:bg-indigo-100 transition-all shadow-xs">
              <UploadCloud className="w-7 h-7" />
            </div>
            <div>
              <h2 className="text-[17px] font-bold text-[var(--text-primary)] group-hover:text-indigo-700 transition-colors">
                Upload Screenshot
              </h2>
              <span className="text-[11px] text-indigo-600 font-medium">App Receipt or Photo</span>
            </div>
          </div>

          <p className="text-[13px] text-[var(--text-muted)] leading-relaxed">
            Upload a screenshot from your mobile banking app or POS receipt. Our OCR extracts transaction details with automated balance redaction.
          </p>

          <div className="pt-2 border-t border-[var(--surface-border-subtle)] flex items-center justify-between">
            <span className="text-[12px] font-bold text-indigo-700 group-hover:translate-x-1 transition-transform flex items-center gap-1.5">
              <span>Start with Image</span>
              <ArrowRight className="w-4 h-4" />
            </span>
            <span className="text-[11px] text-[var(--text-muted)]">PNG, JPG or PDF</span>
          </div>
        </button>
      </div>

      {/* Trust & Guarantee Highlights Bar */}
      <div className="p-5 rounded-2xl bg-[var(--surface)] border border-[var(--surface-border)] shadow-xs">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 text-left">
          <div className="flex items-start gap-3">
            <div className="p-2 rounded-lg bg-emerald-50 text-emerald-700 shrink-0">
              <ShieldCheck className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-[13px] font-bold text-[var(--text-primary)]">Strict Privacy Guard</h3>
              <p className="text-[11px] text-[var(--text-muted)] mt-0.5">
                Balances and full card numbers are masked on your device before processing.
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="p-2 rounded-lg bg-sky-50 text-sky-700 shrink-0">
              <Clock className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-[13px] font-bold text-[var(--text-primary)]">CBN Resolution Clocks</h3>
              <p className="text-[11px] text-[var(--text-muted)] mt-0.5">
                Enforces legal 24h (Transfer) and 72h (POS) Central Bank refund mandates.
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <div className="p-2 rounded-lg bg-purple-50 text-purple-700 shrink-0">
              <CheckCircle2 className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-[13px] font-bold text-[var(--text-primary)]">Switch Verification</h3>
              <p className="text-[11px] text-[var(--text-muted)] mt-0.5">
                Authoritative queries to NIBSS and payment switches verify failed credits.
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* How It Works in 3 Steps */}
      <div className="space-y-3 pt-2">
        <h2 className="text-[15px] font-bold text-[var(--text-primary)] text-center">
          How Smart Reversal resolves your transaction in 3 steps
        </h2>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-center">
          <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-xl space-y-1">
            <span className="w-6 h-6 rounded-full bg-slate-900 text-white inline-flex items-center justify-center font-bold text-[11px]">
              1
            </span>
            <h3 className="text-[13px] font-bold text-[var(--text-primary)]">Share Alert</h3>
            <p className="text-[11px] text-[var(--text-muted)]">
              Paste SMS or receipt; numbers and trace keys are automatically structured.
            </p>
          </div>

          <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-xl space-y-1">
            <span className="w-6 h-6 rounded-full bg-slate-900 text-white inline-flex items-center justify-center font-bold text-[11px]">
              2
            </span>
            <h3 className="text-[13px] font-bold text-[var(--text-primary)]">Match Records</h3>
            <p className="text-[11px] text-[var(--text-muted)]">
              System cross-checks bank records to confirm the exact session ID and failure status.
            </p>
          </div>

          <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-xl space-y-1">
            <span className="w-6 h-6 rounded-full bg-slate-900 text-white inline-flex items-center justify-center font-bold text-[11px]">
              3
            </span>
            <h3 className="text-[13px] font-bold text-[var(--text-primary)]">Track & Refund</h3>
            <p className="text-[11px] text-[var(--text-muted)]">
              Receive a tracking code with live regulatory deadline until funds return to your balance.
            </p>
          </div>
        </div>
      </div>

      {/* Supported Nigerian Banks Pills */}
      <div className="p-3 bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl text-center space-y-2">
        <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
          Compatible with all Nigerian Commercial Banks & Mobile FinTechs
        </span>
        <div className="flex flex-wrap items-center justify-center gap-2 text-[11px] font-medium text-[var(--text-secondary)]">
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">Wema Bank / ALAT</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">GTBank (Guaranty Trust)</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">Access Bank</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">Zenith Bank</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">United Bank for Africa (UBA)</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">First Bank of Nigeria</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">Kuda Bank</span>
          <span className="px-2.5 py-1 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">OPay / Moniepoint</span>
        </div>
      </div>

      {/* Active Case Banner */}
      {activeCaseCount > 0 && (
        <div className="p-4 bg-sky-50 border border-sky-200 rounded-xl flex flex-wrap items-center justify-between gap-3 text-sky-950">
          <div className="flex items-center gap-3">
            <Clock className="w-5 h-5 text-sky-700 shrink-0" />
            <div>
              <strong className="text-[13px] font-bold">You have {activeCaseCount} active case{activeCaseCount > 1 ? 's' : ''} in progress</strong>
              <p className="text-[12px] text-sky-800">Check live status updates or view your CBN regulatory countdown clocks.</p>
            </div>
          </div>
          <button
            type="button"
            onClick={onViewMyCases}
            className="px-3.5 py-1.5 rounded-lg bg-sky-700 text-white font-semibold text-[12px] hover:bg-sky-800 transition-colors cursor-pointer shadow-xs"
          >
            Open My Cases &rarr;
          </button>
        </div>
      )}
    </div>
  );
};
