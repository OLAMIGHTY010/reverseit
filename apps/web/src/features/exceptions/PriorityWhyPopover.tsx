import React, { useState, useRef, useEffect } from 'react';
import { HelpCircle, Info, TrendingUp, AlertCircle, Users, Sparkles, X } from 'lucide-react';
import clsx from 'clsx';
import type { PriorityFactors } from '../../types/domain';

interface PriorityWhyPopoverProps {
  score: number;
  factors: PriorityFactors;
  exceptionRef: string;
}

export const PriorityWhyPopover: React.FC<PriorityWhyPopoverProps> = ({
  score,
  factors,
  exceptionRef,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        popoverRef.current &&
        !popoverRef.current.contains(event.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(event.target as Node)
      ) {
        setIsOpen(false);
      }
    }

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen]);

  const scoreColor =
    score >= 90
      ? 'bg-rose-50 text-rose-800 border-rose-300 hover:bg-rose-100'
      : score >= 75
      ? 'bg-amber-50 text-amber-900 border-amber-300 hover:bg-amber-100'
      : score >= 50
      ? 'bg-sky-50 text-sky-800 border-sky-300 hover:bg-sky-100'
      : 'bg-slate-50 text-slate-700 border-slate-300 hover:bg-slate-100';

  return (
    <div className="relative inline-block text-left">
      <button
        ref={triggerRef}
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          setIsOpen(!isOpen);
        }}
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        aria-label={`Priority score ${score} out of 100 for ${exceptionRef}. Click for algorithmic scoring breakdown.`}
        className={clsx(
          'inline-flex items-center gap-1.5 px-2 py-0.5 rounded border font-mono-financial text-[12px] font-bold transition-colors cursor-pointer',
          scoreColor
        )}
      >
        <span>{score}</span>
        <span className="text-[10px] opacity-60">/100</span>
        <HelpCircle className="w-3 h-3 opacity-70 ml-0.5" />
      </button>

      {isOpen && (
        <div
          ref={popoverRef}
          role="dialog"
          aria-modal="false"
          aria-label="Priority Score Rationale"
          className="absolute z-50 left-0 mt-1.5 w-80 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-lg p-3 text-[12px] text-[var(--text-primary)] animate-in fade-in zoom-in-95 duration-100"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between pb-2 border-b border-[var(--surface-border-subtle)]">
            <div className="flex items-center gap-1.5 font-semibold text-[13px] text-[var(--text-primary)]">
              <TrendingUp className="w-4 h-4 text-sky-600" />
              <span>Priority Algorithm: {score}/100</span>
            </div>
            <button
              type="button"
              onClick={() => setIsOpen(false)}
              className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
              aria-label="Close why popover"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>

          <div className="py-2 text-[12px] text-[var(--text-secondary)] leading-relaxed">
            {factors.rationale}
          </div>

          <div className="space-y-2 pt-2 border-t border-[var(--surface-border-subtle)]">
            <div className="text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">
              Scoring Weights Breakdown
            </div>

            <div className="grid grid-cols-2 gap-2 text-[11px]">
              <div className="p-1.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)]">
                <div className="flex items-center justify-between text-[var(--text-secondary)]">
                  <span className="flex items-center gap-1">
                    <AlertCircle className="w-3 h-3 text-rose-600" /> Exposure
                  </span>
                  <span className="font-mono-financial font-bold">
                    {factors.amountWeight} pts
                  </span>
                </div>
              </div>

              <div className="p-1.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)]">
                <div className="flex items-center justify-between text-[var(--text-secondary)]">
                  <span className="flex items-center gap-1">
                    <Info className="w-3 h-3 text-amber-600" /> SLA Urgency
                  </span>
                  <span className="font-mono-financial font-bold">
                    {factors.slaWeight} pts
                  </span>
                </div>
              </div>

              <div className="p-1.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)]">
                <div className="flex items-center justify-between text-[var(--text-secondary)]">
                  <span className="flex items-center gap-1">
                    <Users className="w-3 h-3 text-indigo-600" /> Customer Impact
                  </span>
                  <span className="font-mono-financial font-bold">
                    {factors.customerWeight} pts
                  </span>
                </div>
              </div>

              <div className="p-1.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)]">
                <div className="flex items-center justify-between text-[var(--text-secondary)]">
                  <span className="flex items-center gap-1">
                    <Sparkles className="w-3 h-3 text-sky-600" /> ML Anomaly
                  </span>
                  <span className="font-mono-financial font-bold">
                    {factors.anomalyWeight} pts
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div className="mt-3 pt-2 border-t border-[var(--surface-border-subtle)] text-[10px] text-[var(--text-muted)] flex justify-between items-center font-mono-financial">
            <span>Ref: {exceptionRef}</span>
            <span>Model: SR-Triager-v2.1</span>
          </div>
        </div>
      )}
    </div>
  );
};
