import React from 'react';
import { Keyboard, X } from 'lucide-react';
import { Button } from '../../components/Button';

interface KeyboardShortcutsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

const SHORTCUTS = [
  { key: 'j / ↓', description: 'Move focus to next exception row' },
  { key: 'k / ↑', description: 'Move focus to previous exception row' },
  { key: 'Enter', description: 'Open or close detail preview drawer for focused row' },
  { key: 'Space', description: 'Toggle row selection for bulk actions' },
  { key: 'a', description: 'Claim or reassign focused exception' },
  { key: 's', description: 'Transition lifecycle status with reason code' },
  { key: 'c', description: 'Toggle Customer-Reported only filter' },
  { key: 'Ctrl + K', description: 'Open global search across all transactions & cases' },
  { key: 'Esc', description: 'Close active drawer, modal, or popover' },
  { key: '?', description: 'Toggle this keyboard shortcuts cheat sheet' },
];

export const KeyboardShortcutsModal: React.FC<KeyboardShortcutsModalProps> = ({
  isOpen,
  onClose,
}) => {
  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="shortcuts-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-md w-full overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <Keyboard className="w-4 h-4 text-sky-600" />
            <h3 id="shortcuts-title" className="text-[14px] font-bold text-[var(--text-primary)]">
              Operations Keyboard Shortcuts
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            aria-label="Close shortcuts dialog"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-3">
          <p className="text-[12px] text-[var(--text-muted)]">
            High-density bank operations console supports full non-mouse keyboard triage:
          </p>

          <div className="divide-y divide-[var(--surface-border-subtle)] border border-[var(--surface-border)] rounded-md overflow-hidden bg-[var(--surface)]">
            {SHORTCUTS.map((s) => (
              <div
                key={s.key}
                className="flex items-center justify-between px-3 py-2 text-[12px]"
              >
                <span className="text-[var(--text-secondary)]">{s.description}</span>
                <kbd className="px-2 py-0.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] font-mono-financial font-bold text-[11px] text-[var(--text-primary)] shadow-xs">
                  {s.key}
                </kbd>
              </div>
            ))}
          </div>

          <div className="flex justify-end pt-2">
            <Button variant="secondary" size="sm" onClick={onClose}>
              Dismiss
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
