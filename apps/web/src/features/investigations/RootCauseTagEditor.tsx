import React, { useState } from 'react';
import { Sparkles, Check, Edit2, ShieldCheck } from 'lucide-react';
import { Button } from '../../components/Button';
import type { RootCause } from '../../types/domain';

interface RootCauseTagEditorProps {
  aiSuggestedRootCause: RootCause;
  confirmedRootCause?: RootCause;
  isConfirmed: boolean;
  onConfirm: (cause: RootCause) => Promise<void>;
}

const ROOT_CAUSE_OPTIONS: Array<{ code: RootCause; label: string; desc: string }> = [
  { code: 'SWITCH_TIMEOUT', label: 'Switch Timeout (Error 91)', desc: 'Interswitch/NIBSS counterparty network timed out' },
  { code: 'LEDGER_GLITCH', label: 'Ledger Post Error', desc: 'Core banking post failure without debit synchronization' },
  { code: 'NETWORK_SPLIT', label: 'Network Split-Brain', desc: 'Intermittent socket disconnect during two-phase commit' },
  { code: 'MERCHANT_DROP', label: 'Merchant POS Drop', desc: 'Acquirer terminal dropped communication post-PIN' },
  { code: 'CUSTOMER_DOUBLE_TAP', label: 'Customer Double-Tap', desc: 'User tapped contactless pad or clicked transfer button twice' },
  { code: 'SUSPENSE_MISMATCH', label: 'GL Suspense Imbalance', desc: 'Reconciliation batch omitted credit settlement entry' },
];

export const RootCauseTagEditor: React.FC<RootCauseTagEditorProps> = ({
  aiSuggestedRootCause,
  confirmedRootCause,
  isConfirmed,
  onConfirm,
}) => {
  const [isEditing, setIsEditing] = useState(false);
  const [selectedCause, setSelectedCause] = useState<RootCause>(
    confirmedRootCause || aiSuggestedRootCause
  );
  const [isSaving, setIsSaving] = useState(false);

  const activeCause = confirmedRootCause || selectedCause;

  const handleConfirmClick = async () => {
    setIsSaving(true);
    try {
      await onConfirm(selectedCause);
      setIsEditing(false);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="p-3.5 rounded-md bg-[var(--surface)] border border-[var(--surface-border)] shadow-xs space-y-2 text-[12px]">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] flex items-center gap-1.5">
          <Sparkles className="w-3.5 h-3.5 text-sky-600" />
          <span>Root-Cause Classification (Scope v2)</span>
        </span>

        {isConfirmed ? (
          <span className="inline-flex items-center gap-1 text-[10px] font-bold px-2 py-0.5 rounded bg-emerald-50 text-emerald-800 border border-emerald-300">
            <ShieldCheck className="w-3 h-3 text-emerald-600" /> Analyst Confirmed
          </span>
        ) : (
          <span className="text-[10px] font-mono-financial text-amber-700 bg-amber-50 px-1.5 py-0.5 rounded border border-amber-200">
            AI Proposed
          </span>
        )}
      </div>

      {!isEditing ? (
        <div className="flex items-center justify-between pt-1">
          <div>
            <div className="font-mono-financial font-extrabold text-[13px] text-[var(--text-primary)]">
              {activeCause.replace(/_/g, ' ')}
            </div>
            <p className="text-[11px] text-[var(--text-muted)] mt-0.5">
              {ROOT_CAUSE_OPTIONS.find((r) => r.code === activeCause)?.desc}
            </p>
          </div>

          <div className="flex items-center gap-1.5">
            {!isConfirmed && (
              <Button
                size="xs"
                variant="primary"
                onClick={handleConfirmClick}
                isLoading={isSaving}
                leftIcon={<Check className="w-3 h-3" />}
              >
                Confirm AI Tag
              </Button>
            )}
            <Button
              size="xs"
              variant="secondary"
              onClick={() => setIsEditing(true)}
              leftIcon={<Edit2 className="w-3 h-3" />}
            >
              Change
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-2 pt-1 animate-in fade-in">
          <select
            value={selectedCause}
            onChange={(e) => setSelectedCause(e.target.value as RootCause)}
            className="w-full px-2.5 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500 font-mono-financial font-medium"
          >
            {ROOT_CAUSE_OPTIONS.map((opt) => (
              <option key={opt.code} value={opt.code}>
                {opt.label}
              </option>
            ))}
          </select>

          <div className="flex items-center justify-end gap-1.5">
            <Button
              size="xs"
              variant="ghost"
              onClick={() => setIsEditing(false)}
            >
              Cancel
            </Button>
            <Button
              size="xs"
              variant="primary"
              onClick={handleConfirmClick}
              isLoading={isSaving}
            >
              Save Confirmed Tag
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};
