import React, { useState } from 'react';
import { X, Sliders, CheckCircle, Info } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ToleranceConfig } from './reconciliationTypes';

interface ToleranceSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  config: ToleranceConfig;
  onSave: (newConfig: ToleranceConfig) => void;
}

export const ToleranceSettingsModal: React.FC<ToleranceSettingsModalProps> = ({
  isOpen,
  onClose,
  config,
  onSave,
}) => {
  const [amountTolerance, setAmountTolerance] = useState(config.amountToleranceNgn);
  const [timeWindowMinutes, setTimeWindowMinutes] = useState(
    Math.round(config.timeWindowSeconds / 60)
  );
  const [autoPair, setAutoPair] = useState(config.autoPairToleranceMatches);
  const [requireChecker, setRequireChecker] = useState(config.requireCheckerSignoff);

  if (!isOpen) return null;

  const handleSave = () => {
    onSave({
      amountToleranceNgn: Number(amountTolerance),
      timeWindowSeconds: Number(timeWindowMinutes) * 60,
      autoPairToleranceMatches: autoPair,
      requireCheckerSignoff: requireChecker,
    });
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-200">
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xl w-full max-w-md overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <Sliders className="w-5 h-5 text-indigo-600" />
            <h3 className="text-[15px] font-bold text-[var(--text-primary)]">
              Reconciliation Tolerance Matching Rules
            </h3>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-4">
          <div className="p-3 bg-indigo-50/50 border border-indigo-200 rounded text-[12px] text-indigo-900 flex items-start gap-2.5">
            <Info className="w-4 h-4 text-indigo-600 shrink-0 mt-0.5" />
            <span>
              Tolerance matching permits automatic identification of cross-border FX rounding (±₦5.00)
              and timestamp cutoff jitter (±30 mins) across clearing boundaries.
            </span>
          </div>

          <div>
            <label className="block text-[12px] font-bold text-[var(--text-primary)] mb-1">
              Amount Tolerance Variance (₦)
            </label>
            <div className="flex items-center gap-3">
              <input
                type="number"
                step="0.5"
                min="0"
                max="100"
                value={amountTolerance}
                onChange={(e) => setAmountTolerance(Number(e.target.value))}
                className="w-32 px-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial text-[var(--text-primary)]"
              />
              <span className="text-[12px] text-[var(--text-muted)]">
                Default: ±₦5.00 (CBN FX Rounding limit)
              </span>
            </div>
          </div>

          <div>
            <label className="block text-[12px] font-bold text-[var(--text-primary)] mb-1">
              Timestamp Boundary Window (Minutes)
            </label>
            <div className="flex items-center gap-3">
              <input
                type="number"
                step="5"
                min="0"
                max="180"
                value={timeWindowMinutes}
                onChange={(e) => setTimeWindowMinutes(Number(e.target.value))}
                className="w-32 px-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial text-[var(--text-primary)]"
              />
              <span className="text-[12px] text-[var(--text-muted)]">
                Default: ±30 min (Settlement batch straddle)
              </span>
            </div>
          </div>

          <div className="pt-2 border-t border-[var(--surface-border-subtle)] space-y-2">
            <label className="flex items-center gap-2 cursor-pointer text-[12px] text-[var(--text-primary)] font-medium">
              <input
                type="checkbox"
                checked={autoPair}
                onChange={(e) => setAutoPair(e.target.checked)}
                className="w-4 h-4 text-indigo-600 rounded"
              />
              <span>Auto-tag candidate breaks as TOLERANCE_MATCHED</span>
            </label>

            <label className="flex items-center gap-2 cursor-pointer text-[12px] text-[var(--text-primary)] font-medium">
              <input
                type="checkbox"
                checked={requireChecker}
                onChange={(e) => setRequireChecker(e.target.checked)}
                className="w-4 h-4 text-indigo-600 rounded"
              />
              <span>Require Dual-Control (Checker) sign-off for paired breaks</span>
            </label>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleSave}
            leftIcon={<CheckCircle className="w-4 h-4" />}
          >
            Save & Apply Rules
          </Button>
        </div>
      </div>
    </div>
  );
};
