import React, { useState } from 'react';
import { Search, Hash, Calendar, Phone, Sparkles, AlertCircle } from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import type { AgentSearchIdentifierType, Channel, CustomerIntakeQuery } from '../../types/domain';

export interface AgentSearchPanelProps {
  onSearch: (query: CustomerIntakeQuery) => void;
  isLoading: boolean;
}

export const AgentSearchPanel: React.FC<AgentSearchPanelProps> = ({ onSearch, isLoading }) => {
  const [activeTab, setActiveTab] = useState<AgentSearchIdentifierType>('REFERENCE');

  // Form states
  const [reference, setReference] = useState('');
  const [amount, setAmount] = useState('');
  const [date, setDate] = useState('2026-10-15');
  const [channel, setChannel] = useState<Channel>('POS');
  const [customerToken, setCustomerToken] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);

  const handleSearchSubmit = (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    setValidationError(null);

    if (activeTab === 'REFERENCE') {
      if (!reference.trim()) {
        setValidationError('Please enter a Transaction Reference, RRN, STAN, or Session ID.');
        return;
      }
      onSearch({
        queryType: 'REFERENCE',
        reference: reference.trim(),
      });
    } else if (activeTab === 'AMOUNT_DATE_CHANNEL') {
      if (!amount.trim()) {
        setValidationError('Please enter the transaction amount.');
        return;
      }
      onSearch({
        queryType: 'AMOUNT_DATE_CHANNEL',
        amount: amount.trim(),
        date,
        channel,
      });
    } else if (activeTab === 'CUSTOMER_TOKEN') {
      if (!customerToken.trim()) {
        setValidationError('Please enter an account number or customer phone number.');
        return;
      }
      onSearch({
        queryType: 'CUSTOMER_TOKEN',
        customerToken: customerToken.trim(),
      });
    }
  };

  const handleQuickPreset = (preset: {
    tab: AgentSearchIdentifierType;
    ref?: string;
    amt?: string;
    dt?: string;
    ch?: Channel;
    token?: string;
  }) => {
    setActiveTab(preset.tab);
    setValidationError(null);

    if (preset.tab === 'REFERENCE' && preset.ref) {
      setReference(preset.ref);
      onSearch({ queryType: 'REFERENCE', reference: preset.ref });
    } else if (preset.tab === 'AMOUNT_DATE_CHANNEL' && preset.amt) {
      setAmount(preset.amt);
      setDate(preset.dt || '2026-10-15');
      setChannel(preset.ch || 'POS');
      onSearch({
        queryType: 'AMOUNT_DATE_CHANNEL',
        amount: preset.amt,
        date: preset.dt || '2026-10-15',
        channel: preset.ch || 'POS',
      });
    } else if (preset.tab === 'CUSTOMER_TOKEN' && preset.token) {
      setCustomerToken(preset.token);
      onSearch({ queryType: 'CUSTOMER_TOKEN', customerToken: preset.token });
    }
  };

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
      {/* Search Identifier Mode Selector */}
      <div className="px-4 py-3 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="p-1 rounded bg-sky-100 text-sky-800">
            <Search className="w-4 h-4" />
          </span>
          <div>
            <h3 className="text-[13px] font-bold text-[var(--text-primary)]">
              Customer Inquiry Identifier Search
            </h3>
            <p className="text-[11px] text-[var(--text-muted)]">
              Authorized identifiers only. Sensitive customer attributes remain masked.
            </p>
          </div>
        </div>

        {/* Tab Buttons */}
        <div className="flex items-center gap-1 bg-[var(--surface)] p-0.5 border border-[var(--surface-border)] rounded text-[12px]">
          <button
            type="button"
            onClick={() => {
              setActiveTab('REFERENCE');
              setValidationError(null);
            }}
            className={clsx(
              'px-2.5 py-1 rounded font-medium transition-colors flex items-center gap-1.5',
              activeTab === 'REFERENCE'
                ? 'bg-sky-600 text-white font-semibold'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            )}
          >
            <Hash className="w-3.5 h-3.5" /> Reference / Session ID
          </button>
          <button
            type="button"
            onClick={() => {
              setActiveTab('AMOUNT_DATE_CHANNEL');
              setValidationError(null);
            }}
            className={clsx(
              'px-2.5 py-1 rounded font-medium transition-colors flex items-center gap-1.5',
              activeTab === 'AMOUNT_DATE_CHANNEL'
                ? 'bg-sky-600 text-white font-semibold'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            )}
          >
            <Calendar className="w-3.5 h-3.5" /> Date + Amount + Channel
          </button>
          <button
            type="button"
            onClick={() => {
              setActiveTab('CUSTOMER_TOKEN');
              setValidationError(null);
            }}
            className={clsx(
              'px-2.5 py-1 rounded font-medium transition-colors flex items-center gap-1.5',
              activeTab === 'CUSTOMER_TOKEN'
                ? 'bg-sky-600 text-white font-semibold'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            )}
          >
            <Phone className="w-3.5 h-3.5" /> Customer Account / Phone
          </button>
        </div>
      </div>

      {/* Search Input Form */}
      <form onSubmit={handleSearchSubmit} className="p-4 space-y-3">
        {/* TAB 1: REFERENCE */}
        {activeTab === 'REFERENCE' && (
          <div className="flex flex-col sm:flex-row gap-2">
            <div className="relative flex-1">
              <input
                id="agent-search-reference-input"
                type="text"
                value={reference}
                onChange={(e) => setReference(e.target.value)}
                placeholder="e.g. TXN-20261015-NIP-9901, 9901847120 (RRN), 009901 (STAN)"
                className="w-full px-3 py-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500 placeholder:text-[var(--text-muted)]"
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={isLoading}
              leftIcon={<Search className="w-4 h-4" />}
            >
              {isLoading ? 'Querying Journals...' : 'Search Switch'}
            </Button>
          </div>
        )}

        {/* TAB 2: AMOUNT + DATE + CHANNEL */}
        {activeTab === 'AMOUNT_DATE_CHANNEL' && (
          <div className="grid grid-cols-1 sm:grid-cols-4 gap-2">
            <div>
              <label htmlFor="agent-search-amount-input" className="sr-only">Amount</label>
              <input
                id="agent-search-amount-input"
                type="text"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                placeholder="Amount (e.g. 45000.00)"
                className="w-full px-3 py-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
            <div>
              <label htmlFor="agent-search-date-input" className="sr-only">Date</label>
              <input
                id="agent-search-date-input"
                type="date"
                value={date}
                onChange={(e) => setDate(e.target.value)}
                className="w-full px-3 py-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
            <div>
              <label htmlFor="agent-search-channel-select" className="sr-only">Channel</label>
              <select
                id="agent-search-channel-select"
                value={channel}
                onChange={(e) => setChannel(e.target.value as Channel)}
                className="w-full px-3 py-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              >
                <option value="POS">POS Terminal</option>
                <option value="NIP">NIP Instant Payment</option>
                <option value="CARD">Debit / Credit Card</option>
                <option value="WEBHOOK">Online Gateway / Webhook</option>
                <option value="DIRECT_DEBIT">Direct Debit / NAPS</option>
              </select>
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={isLoading}
              leftIcon={<Search className="w-4 h-4" />}
            >
              {isLoading ? 'Querying...' : 'Find Matches'}
            </Button>
          </div>
        )}

        {/* TAB 3: CUSTOMER TOKEN */}
        {activeTab === 'CUSTOMER_TOKEN' && (
          <div className="flex flex-col sm:flex-row gap-2">
            <div className="relative flex-1">
              <input
                id="agent-search-customer-token-input"
                type="text"
                value={customerToken}
                onChange={(e) => setCustomerToken(e.target.value)}
                placeholder="e.g. Account Number 0123456789 or Phone +234 809 111 2233"
                className="w-full px-3 py-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500 placeholder:text-[var(--text-muted)]"
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={isLoading}
              leftIcon={<Search className="w-4 h-4" />}
            >
              {isLoading ? 'Searching...' : 'Search Account'}
            </Button>
          </div>
        )}

        {validationError && (
          <div className="flex items-center gap-1.5 text-[12px] text-rose-700 font-medium">
            <AlertCircle className="w-3.5 h-3.5" />
            <span>{validationError}</span>
          </div>
        )}

        {/* Rapid Test Presets Bar */}
        <div className="pt-2 flex flex-wrap items-center gap-1.5 text-[11px] text-[var(--text-muted)] border-t border-[var(--surface-border-subtle)]">
          <span className="font-semibold flex items-center gap-1 text-[var(--text-secondary)]">
            <Sparkles className="w-3 h-3 text-sky-600" /> Presets:
          </span>
          <button
            type="button"
            onClick={() => handleQuickPreset({ tab: 'REFERENCE', ref: 'TXN-20261015-NIP-9901' })}
            className="px-2 py-0.5 rounded bg-[var(--canvas-subdued)] hover:bg-[var(--surface-raised)] border border-[var(--surface-border)] font-mono-financial text-[11px] cursor-pointer"
          >
            State A: Active Exception (NIP-9901)
          </button>
          <button
            type="button"
            onClick={() => handleQuickPreset({ tab: 'REFERENCE', ref: 'TXN-20261015-NIP-9902' })}
            className="px-2 py-0.5 rounded bg-[var(--canvas-subdued)] hover:bg-[var(--surface-raised)] border border-[var(--surface-border)] font-mono-financial text-[11px] cursor-pointer"
          >
            State B: Verified Success (NIP-9902)
          </button>
          <button
            type="button"
            onClick={() => handleQuickPreset({ tab: 'AMOUNT_DATE_CHANNEL', amt: '45000', ch: 'POS' })}
            className="px-2 py-0.5 rounded bg-[var(--canvas-subdued)] hover:bg-[var(--surface-raised)] border border-[var(--surface-border)] font-mono-financial text-[11px] cursor-pointer"
          >
            State D: Disambiguation (₦45k POS)
          </button>
          <button
            type="button"
            onClick={() => handleQuickPreset({ tab: 'CUSTOMER_TOKEN', token: '+234 809 111 2233' })}
            className="px-2 py-0.5 rounded bg-rose-50 text-rose-900 border border-rose-300 font-mono-financial text-[11px] cursor-pointer font-semibold"
          >
            🚨 Repeat Contact &gt;= 3 (Escalation)
          </button>
          <button
            type="button"
            onClick={() => handleQuickPreset({ tab: 'REFERENCE', ref: 'TXN-UNKNOWN-9999' })}
            className="px-2 py-0.5 rounded bg-[var(--canvas-subdued)] hover:bg-[var(--surface-raised)] border border-[var(--surface-border)] font-mono-financial text-[11px] cursor-pointer"
          >
            State C: No Match (Intake Form)
          </button>
        </div>
      </form>
    </div>
  );
};
