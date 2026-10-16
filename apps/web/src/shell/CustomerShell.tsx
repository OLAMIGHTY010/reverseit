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
        <div className="max-w-7xl mx-auto px-4 h-15 flex items-center justify-between gap-4">
          {/* Logo & Brand Identity */}
          <div className="flex items-center gap-3">
            <img src="/logo.png" alt="Smart Reversal Logo" className="w-8 h-8 object-contain" />
            <div>
              <div className="flex items-center gap-2">
                <span className="font-extrabold text-[16px] tracking-tight text-[var(--text-primary)]">
                  Smart Reversal
                </span>
                <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-sky-100 text-sky-800 border border-sky-300">
                  Customer Portal
                </span>
              </div>
              <p className="text-[11px] text-[var(--text-muted)] flex items-center gap-1">
                <span>Self-Service Dispute & Resolution</span>
                <span>&bull;</span>
                <span className="flex items-center gap-1 text-emerald-700 font-medium">
                  <ShieldCheck className="w-3 h-3 text-emerald-600" />
                  <span>CBN Regulatory Protection</span>
                </span>
              </p>
            </div>
          </div>

          {/* Right Controls: Theme Toggle & Bank Staff Access */}
          <div className="flex items-center gap-2.5 text-[12px]">
            {/* Theme Toggle */}
            <button
              type="button"
              onClick={() => setTheme(theme === 'daylight' ? 'soft-dim' : 'daylight')}
              className="p-2 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] transition-colors cursor-pointer"
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
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-lg bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-primary)] hover:border-[var(--primary)] hover:text-[var(--primary)] transition-all cursor-pointer shadow-2xs"
            >
              <Lock className="w-3.5 h-3.5 text-[var(--text-muted)]" />
              <span>Bank Staff Portal</span>
            </button>
          </div>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-4 md:p-6" role="main">
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
