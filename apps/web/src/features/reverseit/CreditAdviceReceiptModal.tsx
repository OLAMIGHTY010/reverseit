import React from 'react';
import { ShieldCheck, Printer, CheckCircle, Download, X, QrCode } from 'lucide-react';
import type { CustomerDisputeCase } from './reverseitTypes';
import { Button } from '../../components/Button';

export interface CreditAdviceReceiptModalProps {
  disputeCase: CustomerDisputeCase;
  isOpen: boolean;
  onClose: () => void;
}

export const CreditAdviceReceiptModal: React.FC<CreditAdviceReceiptModalProps> = ({
  disputeCase,
  isOpen,
  onClose,
}) => {
  if (!isOpen) return null;

  const handlePrint = () => {
    window.print();
  };

  const formattedDate = new Date().toLocaleDateString('en-GB', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  });
  const formattedTime = new Date().toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });

  return (
    <div className="fixed inset-0 z-50 bg-slate-900/60 backdrop-blur-xs flex items-center justify-center p-3 sm:p-4 animate-in fade-in duration-200">
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-3xl shadow-2xl max-w-lg w-full overflow-hidden flex flex-col max-h-[95vh]">
        {/* Top Control Bar */}
        <div className="p-4 border-b border-[var(--surface-border)] flex items-center justify-between bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <span className="font-extrabold text-[14px] text-[var(--text-primary)]">
              Official Bank Credit Advice
            </span>
            <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-100 text-emerald-800 border border-emerald-300">
              Verified Settlement
            </span>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] cursor-pointer"
            aria-label="Close"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Printable Receipt Paper */}
        <div className="p-6 sm:p-8 overflow-y-auto space-y-6 flex-1 bg-white text-slate-900 font-sans print:m-0 print:p-0">
          {/* Bank Branding Header */}
          <div className="text-center space-y-2 border-b border-slate-200 pb-5">
            <div className="inline-flex items-center justify-center w-12 h-12 rounded-2xl bg-emerald-600 text-white shadow-sm mb-1">
              <CheckCircle className="w-7 h-7 stroke-[2.5]" />
            </div>
            <h2 className="text-[20px] font-extrabold tracking-tight text-slate-900 uppercase">
              Electronic Credit Advice
            </h2>
            <p className="text-[12px] text-slate-500 font-medium">
              Automated Reversal & Dispute Settlement System &bull; Interbank Operations
            </p>
          </div>

          {/* Amount Callout */}
          <div className="p-4 rounded-2xl bg-emerald-50 border border-emerald-200 text-center space-y-1">
            <span className="text-[11px] font-bold uppercase tracking-wider text-emerald-800">
              Total Amount Credited
            </span>
            <div className="text-[28px] font-black font-mono text-emerald-700 tracking-tight">
              {disputeCase.currency} {disputeCase.amount}
            </div>
            <span className="text-[11px] text-emerald-900 font-medium block">
              Provisional Credit / Instant Switch Reversal Approved
            </span>
          </div>

          {/* Key Transaction Metadata Grid */}
          <div className="space-y-3 text-[12px] border-b border-slate-200 pb-5">
            <div className="flex justify-between py-1 border-b border-slate-100">
              <span className="text-slate-500 font-medium">Case Reference:</span>
              <strong className="font-mono text-slate-900 font-bold">{disputeCase.caseNumber}</strong>
            </div>
            <div className="flex justify-between py-1 border-b border-slate-100">
              <span className="text-slate-500 font-medium">Switch Session RRN:</span>
              <span className="font-mono text-slate-900 font-semibold">{disputeCase.reference}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-slate-100">
              <span className="text-slate-500 font-medium">Channel / Rails:</span>
              <span className="font-semibold text-slate-800 uppercase">{disputeCase.channel} Network</span>
            </div>
            <div className="flex justify-between py-1 border-b border-slate-100">
              <span className="text-slate-500 font-medium">Customer Account:</span>
              <span className="font-mono text-slate-900 font-semibold">{disputeCase.maskedAccount || '*** 4092'}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-slate-100">
              <span className="text-slate-500 font-medium">Value Date:</span>
              <span className="font-mono text-slate-800">{formattedDate} at {formattedTime}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-slate-500 font-medium">Core Banking Posting:</span>
              <span className="font-mono text-emerald-700 font-semibold">POSTED_SUCCESS (GL-30291)</span>
            </div>
          </div>

          {/* QR Code and Cryptographic Stamp */}
          <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200 flex items-center justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-1.5 text-slate-900 font-bold text-[12px]">
                <ShieldCheck className="w-4 h-4 text-emerald-600" />
                <span>Digitally Sealed & Cryptographically Audited</span>
              </div>
              <p className="text-[11px] text-slate-500 leading-relaxed">
                Hash: <code className="text-slate-700 font-mono text-[10px]">sha256:7f9a2e...b38c</code>
              </p>
              <p className="text-[10px] text-slate-400">
                Authorized under Central Bank Dispute Resolution Mandate
              </p>
            </div>
            <div className="w-14 h-14 bg-white border border-slate-300 rounded-xl p-1.5 flex items-center justify-center shrink-0">
              <QrCode className="w-full h-full text-slate-800" />
            </div>
          </div>
        </div>

        {/* Footer Actions */}
        <div className="p-4 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-between gap-2">
          <Button
            size="sm"
            variant="secondary"
            onClick={onClose}
          >
            Close
          </Button>

          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="secondary"
              leftIcon={<Printer className="w-3.5 h-3.5" />}
              onClick={handlePrint}
            >
              Print Receipt
            </Button>
            <Button
              size="sm"
              variant="primary"
              leftIcon={<Download className="w-3.5 h-3.5" />}
              onClick={handlePrint}
            >
              Download PDF
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
