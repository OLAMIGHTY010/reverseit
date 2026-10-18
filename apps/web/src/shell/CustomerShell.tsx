import React from 'react';
import { Sun, Moon, Lock, ShieldCheck } from 'lucide-react';
import { useApp } from '../context/AppContext';
import { ToastContainer } from '../components/Toast';

export interface CustomerShellProps {
  children: React.ReactNode;
}

export const CustomerShell: React.FC<CustomerShellProps> = ({ children }) => {
  const { theme, setTheme, setPortalMode } = useApp();

  return (
    <div className="min-h-screen bg-[var(--canvas)] text-[var(--text-primary)] flex flex-col selection:bg-[var(--primary-subtle)] selection:text-[var(--primary)] transition-colors">
      {/* Top Customer Navigation Bar */}
      <header className="sticky top-0 z-30 bg-[var(--surface)] border-b border-[var(--surface-border)] shadow-xs">
        <div className="max-w-7xl mx-auto px-3 sm:px-4 min-h-[56px] sm:h-16 py-2 sm:py-0 flex items-center justify-between gap-2 sm:gap-4">
          {/* Logo & Brand Identity */}
          <div className="flex items-center gap-2.5 sm:gap-3 min-w-0">
            <img src="/logo.png" alt="Smart Reversal Logo" className="w-7 h-7 sm:w-8 sm:h-8 object-contain shrink-0" />
            <div className="min-w-0">
              <div className="flex items-center gap-1.5 sm:gap-2">
                <span className="font-extrabold text-[15px] sm:text-[16px] tracking-tight text-[var(--text-primary)] whitespace-nowrap">
                  Smart Reversal
                </span>
                <span className="hidden sm:inline-flex text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-sky-100 text-sky-800 border border-sky-300 dark:bg-sky-950/60 dark:text-sky-300 dark:border-sky-800 shrink-0">
                  Customer Portal
                </span>
              </div>
              <p className="text-[10px] sm:text-[11px] text-[var(--text-muted)] flex items-center gap-1 sm:gap-1.5 truncate">
                <span className="hidden md:inline">Self-Service Dispute & Resolution</span>
                <span className="hidden md:inline">&bull;</span>
                <span className="inline-flex items-center gap-1 text-emerald-700 dark:text-emerald-400 font-medium shrink-0">
                  <ShieldCheck className="w-3 h-3 text-emerald-600 dark:text-emerald-400 shrink-0" />
                  <span className="truncate">CBN Regulatory Protection</span>
                </span>
              </p>
            </div>
          </div>

          {/* Right Controls: Theme Toggle & Bank Staff Access */}
          <div className="flex items-center gap-1.5 sm:gap-2.5 text-[12px] shrink-0">
            {/* Theme Toggle */}
            <button
              type="button"
              onClick={() => setTheme(theme === 'daylight' ? 'soft-dim' : 'daylight')}
              className="p-1.5 sm:p-2 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] transition-colors cursor-pointer shrink-0"
              title={`Switch theme (currently ${theme})`}
              aria-label="Toggle Theme Mode"
            >
              {theme === 'daylight' ? (
                <Moon className="w-4 h-4 text-slate-700" aria-hidden="true" />
              ) : (
                <Sun className="w-4 h-4 text-amber-400" aria-hidden="true" />
              )}
            </button>

            {/* Staff Portal Gateway */}
            <button
              type="button"
              onClick={() => setPortalMode('staff')}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 sm:px-3 text-xs font-semibold rounded-lg bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-primary)] hover:border-[var(--primary)] hover:text-[var(--primary)] transition-all cursor-pointer shadow-2xs shrink-0 whitespace-nowrap"
              title="Switch to Bank Staff Portal"
            >
              <Lock className="w-3.5 h-3.5 text-[var(--text-muted)] shrink-0" />
              <span className="hidden sm:inline">Bank Staff Portal</span>
              <span className="sm:hidden font-medium">Staff Portal</span>
            </button>
          </div>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-3 sm:p-4 md:p-6" role="main">
        {children}
      </main>

      {/* Customer Footer */}
      <footer className="border-t border-[var(--surface-border)] py-6 bg-[var(--surface)] text-[12px] text-[var(--text-muted)] mt-auto">
        <div className="max-w-7xl mx-auto px-4 flex flex-col sm:flex-row items-center justify-between gap-3 text-center sm:text-left">
          <div className="flex items-center gap-2">
            <span className="font-semibold text-[var(--text-primary)]">Smart Reversal</span>
            <span>&bull;</span>
            <span>Central Bank of Nigeria (CBN) Electronic Payment Dispute Guidelines Compliant</span>
          </div>
          <div className="flex items-center gap-4 text-[11px]">
            <span>Bank-Grade Encryption</span>
            <span>&bull;</span>
            <span>Local PII Redaction</span>
            <span>&bull;</span>
            <span>Zero Hallucinations Verified</span>
          </div>
        </div>
      </footer>

      <ToastContainer />
    </div>
  );
};
