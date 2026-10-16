import React from 'react';
import {
  CheckCircle2,
  AlertTriangle,
  AlertOctagon,
  Info,
  X,
} from 'lucide-react';
import clsx from 'clsx';
import { useApp } from '../context/AppContext';
import type { ToastItem } from '../context/AppContextTypes';

export const ToastContainer: React.FC = () => {
  const { toasts, dismissToast } = useApp();

  if (toasts.length === 0) return null;

  return (
    <div
      className="fixed bottom-5 right-5 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none"
      role="region"
      aria-label="System notifications"
    >
      {toasts.map((toast: ToastItem) => {
        const isAlert = toast.type === 'error' || toast.type === 'warning';
        return (
          <div
            key={toast.id}
            role={isAlert ? 'alert' : 'status'}
            aria-live={isAlert ? 'assertive' : 'polite'}
            className={clsx(
              'pointer-events-auto p-3.5 rounded-md border shadow-lg transition-all duration-200 flex items-start gap-3 text-[13px] bg-[var(--surface)]',
              toast.type === 'success' && 'border-emerald-300 text-emerald-950',
              toast.type === 'error' && 'border-rose-300 text-rose-950',
              toast.type === 'warning' && 'border-amber-300 text-amber-950',
              toast.type === 'info' && 'border-sky-300 text-sky-950'
            )}
          >
            <div className="shrink-0 mt-0.5">
              {toast.type === 'success' && <CheckCircle2 className="w-4 h-4 text-emerald-600" />}
              {toast.type === 'error' && <AlertOctagon className="w-4 h-4 text-rose-600" />}
              {toast.type === 'warning' && <AlertTriangle className="w-4 h-4 text-amber-600" />}
              {toast.type === 'info' && <Info className="w-4 h-4 text-sky-600" />}
            </div>

            <div className="flex-1 space-y-1">
              <div className="font-semibold">{toast.title}</div>
              {toast.message && (
                <div className="text-[12px] opacity-90 leading-relaxed">{toast.message}</div>
              )}
              {toast.action && (
                <button
                  type="button"
                  onClick={toast.action.onClick}
                  className="text-[12px] font-semibold underline hover:opacity-80 cursor-pointer pt-1 block"
                >
                  {toast.action.label}
                </button>
              )}
            </div>

            <button
              type="button"
              onClick={() => dismissToast(toast.id)}
              className="shrink-0 p-1 opacity-60 hover:opacity-100 rounded cursor-pointer"
              aria-label="Dismiss notification"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
        );
      })}
    </div>
  );
};
