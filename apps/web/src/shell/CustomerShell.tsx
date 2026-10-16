import React from 'react';
import { Sun, Moon, Lock } from 'lucide-react';
import { useApp } from '../context/AppContext';
import { ToastContainer } from '../components/Toast';

export interface CustomerShellProps {
  children: React.ReactNode;
}

export const CustomerShell: React.FC<CustomerShellProps> = ({ children }) => {
  const { theme, setTheme, setPortalMode } = useApp();

  return (
    <div className="min-h-screen bg-[var(--canvas)] text-[var(--text-primary)] flex flex-col selection:bg-[var(--primary-subtle)] selection:text-[var(--primary)]">
      {/* Top Header - Clean, balanced, maintaining initial visual tone */}
      <header className="sticky top-0 z-30 bg-[var(--surface)] border-b border-[var(--surface-border)] shadow-xs">
        <div className="max-w-7xl mx-auto px-4 h-14 flex items-center justify-between gap-4">
          {/* Logo & Branding */}
          <div className="flex items-center gap-2.5">
            <img src="/logo.png" alt="SMARTREVERSAL Logo" className="w-8 h-8 object-contain" />
            <div>
              <div className="font-bold text-[15px] tracking-tight text-[var(--text-primary)] flex items-center gap-1.5">
                <span>SMARTREVERSAL</span>
                <span className="text-[10px] font-medium px-1.5 py-0.2 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-muted)]">
                  Customer Portal
                </span>
              </div>
              <div className="text-[11px] text-[var(--text-muted)] tracking-tight">
                Self-Service Transaction Dispute & Resolution
              </div>
            </div>
          </div>

          {/* Right Controls: Theme Toggle & Bank Staff Access */}
          <div className="flex items-center gap-2 text-[12px]">
            {/* Theme Toggle */}
            <button
              type="button"
              onClick={() => setTheme(theme === 'daylight' ? 'soft-dim' : 'daylight')}
              className="p-1.5 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer"
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
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-primary)] hover:border-[var(--primary)] hover:text-[var(--primary)] transition-colors cursor-pointer"
            >
              <Lock className="w-3.5 h-3.5 text-[var(--text-muted)]" />
              <span>Staff Login</span>
            </button>
          </div>
        </div>
      </header>

      {/* Main Content Area - Preserves initial layout & padding */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-4 md:p-6 space-y-6" role="main">
        {children}
      </main>

      <ToastContainer />
    </div>
  );
};
