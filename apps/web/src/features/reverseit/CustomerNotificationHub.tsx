import React, { useState } from 'react';
import { MessageSquare, Smartphone, Bell, CheckCheck, Copy, Check, ShieldCheck, X } from 'lucide-react';
import type { CustomerDisputeCase } from './reverseitTypes';
import { Button } from '../../components/Button';

export interface CustomerNotificationHubProps {
  disputeCase: CustomerDisputeCase;
  isOpen: boolean;
  onClose: () => void;
}

type NotificationChannel = 'whatsapp' | 'sms' | 'push';

export const CustomerNotificationHub: React.FC<CustomerNotificationHubProps> = ({
  disputeCase,
  isOpen,
  onClose,
}) => {
  const [activeChannel, setActiveChannel] = useState<NotificationChannel>('whatsapp');
  const [copied, setCopied] = useState(false);

  if (!isOpen) return null;

  const now = new Date();
  const timeStr = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const dateStr = now.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' });

  // Notification Copy Texts
  const whatsappText = `🔔 *Smart Reversal Notification*\n\nHello *${disputeCase.customerName || 'Customer'}*,\n\nYour dispute for *${disputeCase.currency} ${disputeCase.amount}* regarding reference *${disputeCase.reference}* has been processed.\n\nStatus: *${disputeCase.statusLabel}*\nNotice: ${disputeCase.customerSafeStatusText}\n\nCase ID: ${disputeCase.caseNumber}\nBank Target: Within 4 Hours\nCBN Statutory Mandate: Protected under T+1 SLA.\n\nTrack your case in real time: https://smart-reversal.vercel.app?ref=${disputeCase.caseNumber}\n\n_Thank you for banking with us._`;

  const smsText = `Bank Alert: Reversal Case ${disputeCase.caseNumber} update. Amount: ${disputeCase.currency} ${disputeCase.amount}. Status: ${disputeCase.statusLabel}. Funds protected under CBN guidelines. View status at smart-reversal.vercel.app. Help: 0700-BANK-HELP`;

  const pushTitle = `Dispute Update: ${disputeCase.caseNumber}`;
  const pushBody = `${disputeCase.currency} ${disputeCase.amount} dispute status changed to: ${disputeCase.statusLabel}.`;

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 bg-slate-900/60 backdrop-blur-xs flex items-center justify-center p-3 sm:p-4 animate-in fade-in duration-200">
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-3xl shadow-2xl max-w-lg w-full overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="p-4 sm:p-5 border-b border-[var(--surface-border)] flex items-center justify-between bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2.5">
            <div className="w-9 h-9 rounded-xl bg-sky-600 text-white flex items-center justify-center shadow-xs">
              <Bell className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-extrabold text-[15px] text-[var(--text-primary)]">
                Omnichannel Notification Hub
              </h3>
              <p className="text-[11px] text-[var(--text-muted)]">
                Live simulated customer notifications sent by the bank
              </p>
            </div>
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

        {/* Channel Selector Tabs */}
        <div className="grid grid-cols-3 border-b border-[var(--surface-border)] p-1.5 bg-[var(--canvas-subdued)] gap-1 text-[12px] font-bold">
          <button
            type="button"
            onClick={() => setActiveChannel('whatsapp')}
            className={`py-2 px-3 rounded-xl flex items-center justify-center gap-1.5 transition-all cursor-pointer ${
              activeChannel === 'whatsapp'
                ? 'bg-emerald-600 text-white shadow-xs'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            }`}
          >
            <MessageSquare className="w-4 h-4" />
            <span>WhatsApp</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveChannel('sms')}
            className={`py-2 px-3 rounded-xl flex items-center justify-center gap-1.5 transition-all cursor-pointer ${
              activeChannel === 'sms'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            }`}
          >
            <Smartphone className="w-4 h-4" />
            <span>SMS Alert</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveChannel('push')}
            className={`py-2 px-3 rounded-xl flex items-center justify-center gap-1.5 transition-all cursor-pointer ${
              activeChannel === 'push'
                ? 'bg-purple-600 text-white shadow-xs'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            }`}
          >
            <Bell className="w-4 h-4" />
            <span>Mobile Push</span>
          </button>
        </div>

        {/* Channel Preview Body */}
        <div className="p-4 sm:p-6 overflow-y-auto space-y-4 flex-1">
          {activeChannel === 'whatsapp' && (
            <div className="space-y-3">
              {/* WhatsApp Interface Wrapper */}
              <div className="rounded-2xl bg-[#0b141a] text-slate-100 p-4 border border-emerald-950/40 shadow-inner font-sans space-y-3">
                {/* Chat Top bar */}
                <div className="flex items-center gap-2.5 pb-3 border-b border-slate-800">
                  <div className="w-8 h-8 rounded-full bg-emerald-700 flex items-center justify-center font-bold text-white text-[12px]">
                    SR
                  </div>
                  <div className="flex-1 min-w-0">
                    <span className="font-bold text-[13px] block text-slate-100">
                      Smart Reversal Bank Bot
                    </span>
                    <span className="text-[10px] text-emerald-400 block">Official Verified Business Account</span>
                  </div>
                  <span className="text-[10px] text-slate-400">{timeStr}</span>
                </div>

                {/* WhatsApp Message Bubble */}
                <div className="bg-[#1f2c34] rounded-2xl rounded-tl-xs p-3.5 space-y-2 text-[12px] leading-relaxed shadow-xs text-slate-200">
                  <div className="flex items-center gap-1.5 text-emerald-400 font-bold text-[11px]">
                    <ShieldCheck className="w-4 h-4" />
                    <span>Bank Dispute Update &bull; Ref {disputeCase.caseNumber}</span>
                  </div>
                  <p>
                    Hello <span className="font-bold text-white">{disputeCase.customerName || 'Customer'}</span>,
                  </p>
                  <p>
                    Your dispute report for transaction reference <code className="bg-slate-900/60 px-1 py-0.5 rounded text-sky-300 font-mono text-[11px]">{disputeCase.reference}</code> has been investigated.
                  </p>
                  <div className="p-2.5 rounded-xl bg-slate-900/50 border border-slate-700/60 space-y-1">
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-400">Disputed Amount:</span>
                      <strong className="text-white font-mono">{disputeCase.currency} {disputeCase.amount}</strong>
                    </div>
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-400">Current Status:</span>
                      <span className="text-emerald-400 font-bold">{disputeCase.statusLabel}</span>
                    </div>
                    <div className="flex justify-between text-[11px]">
                      <span className="text-slate-400">SLA Mandate:</span>
                      <span className="text-sky-300">Protected by CBN T+1 SLA</span>
                    </div>
                  </div>
                  <p className="text-[11px] text-slate-300">
                    {disputeCase.customerSafeStatusText}
                  </p>
                  <div className="flex items-center justify-end gap-1 text-[10px] text-slate-400 pt-1">
                    <span>{timeStr}</span>
                    <CheckCheck className="w-3.5 h-3.5 text-emerald-400" />
                  </div>
                </div>

                {/* Action quick buttons */}
                <div className="grid grid-cols-2 gap-2 pt-1 text-[11px] font-bold">
                  <div className="p-2 rounded-xl bg-[#1f2c34] text-center text-emerald-400 border border-slate-700 cursor-pointer hover:bg-slate-800 transition-colors">
                    🔍 Track Full Case
                  </div>
                  <div className="p-2 rounded-xl bg-[#1f2c34] text-center text-emerald-400 border border-slate-700 cursor-pointer hover:bg-slate-800 transition-colors">
                    🧾 Download Credit Slip
                  </div>
                </div>
              </div>

              <div className="flex justify-end">
                <Button
                  size="sm"
                  variant="secondary"
                  leftIcon={copied ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                  onClick={() => handleCopy(whatsappText)}
                >
                  {copied ? 'Copied Text!' : 'Copy WhatsApp Message'}
                </Button>
              </div>
            </div>
          )}

          {activeChannel === 'sms' && (
            <div className="space-y-3">
              <div className="p-4 rounded-2xl bg-slate-900 text-slate-100 border border-slate-800 font-mono text-[12px] space-y-3 shadow-inner">
                <div className="flex items-center justify-between text-[10px] text-slate-400 border-b border-slate-800 pb-2">
                  <span>Sender: <strong>BANK-ALERT</strong></span>
                  <span>{timeStr} &bull; {dateStr}</span>
                </div>
                <p className="leading-relaxed text-slate-200">
                  {smsText}
                </p>
                <div className="text-[10px] text-slate-400 pt-1">
                  Network: MTN / Airtel / Glo Delivery Verified (00-Delivered)
                </div>
              </div>

              <div className="flex justify-end">
                <Button
                  size="sm"
                  variant="secondary"
                  leftIcon={copied ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                  onClick={() => handleCopy(smsText)}
                >
                  {copied ? 'Copied SMS!' : 'Copy SMS Text'}
                </Button>
              </div>
            </div>
          )}

          {activeChannel === 'push' && (
            <div className="space-y-3">
              <div className="p-4 rounded-2xl bg-slate-950 text-white border border-slate-800 shadow-xl space-y-2">
                <div className="flex items-center justify-between text-[11px] text-slate-400">
                  <div className="flex items-center gap-1.5">
                    <div className="w-4 h-4 rounded-md bg-sky-500 text-white flex items-center justify-center text-[9px] font-bold">
                      SR
                    </div>
                    <span className="font-semibold text-slate-300">Bank Mobile App</span>
                  </div>
                  <span>now</span>
                </div>
                <div className="space-y-0.5">
                  <strong className="text-[13px] text-white block">{pushTitle}</strong>
                  <p className="text-[12px] text-slate-300 leading-normal">{pushBody}</p>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex justify-between items-center text-[12px]">
          <span className="text-[11px] text-[var(--text-muted)] flex items-center gap-1.5">
            <ShieldCheck className="w-4 h-4 text-emerald-600" />
            <span>Complies with Central Bank Disclosure Standards</span>
          </span>
          <Button size="sm" variant="secondary" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </div>
  );
};
