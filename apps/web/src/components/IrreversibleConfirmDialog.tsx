import React, { useState } from 'react';
import { AlertOctagon, X, ShieldAlert } from 'lucide-react';
import { Button } from './Button';

export interface IrreversibleConfirmDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void>;
  title: string;
  description: string;
  confirmKeyword?: string;
  actionButtonText?: string;
}

export const IrreversibleConfirmDialog: React.FC<IrreversibleConfirmDialogProps> = ({
  isOpen,
  onClose,
  onConfirm,
  title,
  description,
  confirmKeyword = 'CONFIRM REVERSAL',
  actionButtonText = 'Permanently Execute Reversal',
}) => {
  const [typedInput, setTypedInput] = useState('');
  const [isExecuting, setIsExecuting] = useState(false);

  if (!isOpen) return null;

  const isConfirmed = typedInput.trim() === confirmKeyword;

  const handleExecute = async () => {
    if (!isConfirmed) return;
    setIsExecuting(true);
    try {
      await onConfirm();
      setTypedInput('');
      onClose();
    } finally {
      setIsExecuting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-xs"
      role="dialog"
      aria-modal="true"
      aria-labelledby="irreversible-dialog-title"
    >
      <div className="w-full max-w-md bg-[var(--surface)] border border-rose-400 rounded-lg shadow-2xl overflow-hidden animate-in fade-in zoom-in-95">
        {/* Warning Banner Header */}
        <div className="px-5 py-3.5 bg-rose-50 border-b border-rose-200 text-rose-950 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="p-1 rounded bg-rose-600 text-white">
              <AlertOctagon className="w-4 h-4" />
            </span>
            <h2 id="irreversible-dialog-title" className="text-[14px] font-bold">
              {title}
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 text-rose-700 hover:text-rose-900 rounded cursor-pointer"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content */}
        <div className="p-5 space-y-4 text-[13px]">
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-secondary)] leading-relaxed">
            {description}
          </div>

          <div className="space-y-1.5">
            <label htmlFor="confirm-keyword-input" className="block text-[12px] font-semibold text-[var(--text-primary)]">
              To proceed, please type <span className="font-mono-financial text-rose-700 font-bold select-all bg-rose-50 px-1.5 py-0.5 rounded border border-rose-200">{confirmKeyword}</span> below:
            </label>
            <input
              id="confirm-keyword-input"
              type="text"
              value={typedInput}
              onChange={(e) => setTypedInput(e.target.value)}
              placeholder={`Type "${confirmKeyword}"`}
              className="w-full px-3 py-1.5 text-[13px] font-mono-financial bg-[var(--surface)] text-[var(--text-primary)] border border-[var(--surface-border)] rounded focus:outline-2 focus:outline-rose-500"
              autoFocus
            />
          </div>
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-[var(--surface-border)] bg-[var(--canvas-subdued)] flex items-center justify-end gap-2.5">
          <Button variant="ghost" size="md" onClick={onClose} disabled={isExecuting}>
            Abort Action
          </Button>
          <Button
            variant="danger"
            size="md"
            disabled={!isConfirmed}
            isLoading={isExecuting}
            leftIcon={<ShieldAlert className="w-4 h-4" />}
            onClick={handleExecute}
          >
            {actionButtonText}
          </Button>
        </div>
      </div>
    </div>
  );
};
