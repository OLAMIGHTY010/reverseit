import React from 'react';
import { AlertTriangle, RefreshCw, X } from 'lucide-react';
import { Button } from '../../components/Button';

interface ConflictDialogProps {
  isOpen: boolean;
  exceptionRef: string;
  updatedBy: string;
  onReload: () => void;
  onDismiss: () => void;
}

export const ConflictDialog: React.FC<ConflictDialogProps> = ({
  isOpen,
  exceptionRef,
  updatedBy,
  onReload,
  onDismiss,
}) => {
  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="conflict-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/40 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-amber-300 rounded-lg shadow-xl max-w-md w-full p-5 space-y-4">
        <div className="flex items-start gap-3">
          <div className="p-2 rounded-full bg-amber-100 text-amber-800 shrink-0">
            <AlertTriangle className="w-5 h-5 text-amber-700" />
          </div>
          <div className="space-y-1">
            <h3 id="conflict-dialog-title" className="text-[15px] font-bold text-[var(--text-primary)]">
              Concurrent Modification Conflict
            </h3>
            <p className="text-[13px] text-[var(--text-secondary)] leading-relaxed">
              Another analyst (<span className="font-semibold text-[var(--text-primary)]">{updatedBy}</span>) modified{' '}
              <span className="font-mono-financial font-bold text-[var(--text-primary)]">{exceptionRef}</span> since you opened this view.
            </p>
          </div>
        </div>

        <div className="p-3 bg-amber-50/70 border border-amber-200 rounded text-[12px] text-amber-900 space-y-1">
          <p className="font-semibold">Optimistic Concurrency Protection Active</p>
          <p className="opacity-90">
            To prevent accidental overwrites of financial investigation notes and state transitions, your pending action was paused. Please reload to inspect the latest state.
          </p>
        </div>

        <div className="flex items-center justify-end gap-2 pt-2 border-t border-[var(--surface-border-subtle)]">
          <Button
            variant="ghost"
            size="sm"
            onClick={onDismiss}
            leftIcon={<X className="w-4 h-4" />}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={onReload}
            leftIcon={<RefreshCw className="w-4 h-4" />}
          >
            Reload Latest Record
          </Button>
        </div>
      </div>
    </div>
  );
};
