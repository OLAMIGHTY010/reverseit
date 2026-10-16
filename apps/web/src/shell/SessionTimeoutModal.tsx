import React, { useState, useEffect } from 'react';
import { Clock, LogOut } from 'lucide-react';
import { Button } from '../components/Button';
import { useApp } from '../context/AppContext';

export const SessionTimeoutModal: React.FC = () => {
  const { sessionTimeoutOpen, setSessionTimeoutOpen, addToast } = useApp();
  const [secondsRemaining, setSecondsRemaining] = useState(120);

  useEffect(() => {
    if (!sessionTimeoutOpen) return;

    const interval = setInterval(() => {
      setSecondsRemaining((prev) => {
        if (prev <= 1) {
          clearInterval(interval);
          setSessionTimeoutOpen(false);
          addToast({
            type: 'error',
            title: 'Session Expired',
            message: 'You have been logged out due to inactivity in accordance with banking security policy.',
          });
          return 120;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [sessionTimeoutOpen, addToast, setSessionTimeoutOpen]);

  if (!sessionTimeoutOpen) return null;

  const handleExtend = () => {
    setSessionTimeoutOpen(false);
    addToast({
      type: 'success',
      title: 'Session Extended',
      message: 'Your authenticated operations session has been renewed for 30 minutes.',
    });
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-xs"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="timeout-dialog-title"
    >
      <div className="w-full max-w-sm bg-[var(--surface)] border border-amber-300 rounded-lg shadow-2xl overflow-hidden p-5 space-y-4 animate-in fade-in zoom-in-95">
        <div className="flex items-center gap-2 text-amber-900">
          <span className="p-1.5 rounded-full bg-amber-100">
            <Clock className="w-5 h-5 text-amber-700 animate-pulse" />
          </span>
          <h2 id="timeout-dialog-title" className="text-[15px] font-bold">
            Session Timeout Warning
          </h2>
        </div>

        <p className="text-[13px] text-[var(--text-secondary)] leading-relaxed">
          For financial security compliance, your session will automatically terminate in{' '}
          <strong className="font-mono-financial text-amber-800 text-[15px]">{secondsRemaining}s</strong> due to idle time.
        </p>

        <div className="flex items-center justify-end gap-2 pt-2 border-t border-[var(--surface-border)]">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSessionTimeoutOpen(false)}
            leftIcon={<LogOut className="w-3.5 h-3.5" />}
          >
            Logout
          </Button>
          <Button variant="primary" size="sm" onClick={handleExtend}>
            Extend Session
          </Button>
        </div>
      </div>
    </div>
  );
};
