import React from 'react';
import { Layers, ArrowRight } from 'lucide-react';
import { Button } from '../../components/Button';
import type { Transaction } from '../../types/domain';

export interface AgentDisambiguationCardProps {
  candidates: Transaction[];
  onSelectCandidate: (txn: Transaction) => void;
}

export const AgentDisambiguationCard: React.FC<AgentDisambiguationCardProps> = ({
  candidates,
  onSelectCandidate,
}) => {
  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
      {/* Header Banner */}
      <div className="p-4 bg-sky-500/10 border-b border-sky-200 flex flex-wrap items-center justify-between gap-3 text-sky-950">
        <div className="flex items-center gap-2">
          <span className="p-1 rounded bg-sky-100 text-sky-800">
            <Layers className="w-5 h-5 text-sky-600" />
          </span>
          <div>
            <h3 className="text-[14px] font-bold">
              Ambiguous Matches: {candidates.length} Candidate Records Detected
            </h3>
            <p className="text-[12px] text-sky-800/90 mt-0.5">
              Please disambiguate with the customer by verifying the exact time or merchant terminal.
            </p>
          </div>
        </div>
      </div>

      {/* Candidate Cards Grid */}
      <div className="p-4 space-y-3">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {candidates.map((txn, index) => (
            <div
              key={txn.id}
              className="p-3.5 border border-[var(--surface-border)] rounded-md bg-[var(--canvas-subdued)] hover:bg-[var(--surface)] transition-all space-y-2.5 text-[12px]"
            >
              <div className="flex items-center justify-between font-bold">
                <span className="font-mono-financial text-[13px] text-[var(--text-primary)]">
                  Option #{index + 1}: {txn.reference}
                </span>
                <span className="text-[10px] font-mono-financial px-1.5 py-0.2 rounded bg-[var(--surface)] border border-[var(--surface-border)] uppercase">
                  {txn.status}
                </span>
              </div>

              <div className="space-y-1 text-[var(--text-secondary)] font-mono-financial text-[11px]">
                <div className="flex justify-between">
                  <span>Amount & Currency:</span>
                  <strong className="text-[var(--text-primary)]">{txn.currency} {Number(txn.amount).toLocaleString('en-US', { minimumFractionDigits: 2 })}</strong>
                </div>
                <div className="flex justify-between">
                  <span>Channel & STAN:</span>
                  <span>{txn.channel} &bull; STAN {txn.stan}</span>
                </div>
                <div className="flex justify-between">
                  <span>RRN:</span>
                  <span>{txn.rrn}</span>
                </div>
                <div className="flex justify-between">
                  <span>Timestamp:</span>
                  <span>{new Date(txn.createdAt).toLocaleTimeString()}</span>
                </div>
                <div className="flex justify-between">
                  <span>Counterparty Bank:</span>
                  <span>{txn.beneficiaryBank}</span>
                </div>
              </div>

              <div className="pt-2 border-t border-[var(--surface-border-subtle)] flex justify-end">
                <Button
                  size="sm"
                  variant="primary"
                  onClick={() => onSelectCandidate(txn)}
                  rightIcon={<ArrowRight className="w-3.5 h-3.5" />}
                >
                  Select this Transaction
                </Button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
