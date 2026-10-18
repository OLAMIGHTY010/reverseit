import React, { useState, useEffect } from 'react';
import {
  Search,
  Bell,
  Sun,
  Moon,
  User,
  Sliders,
  ChevronRight,
  Clock,
  BookOpen,
  LogOut,
  ChevronDown,
  ArrowLeftRight,
  Shield,
  Menu,
  X,
} from 'lucide-react';
import clsx from 'clsx';
import { useApp } from '../context/AppContext';
import { GlobalSearchModal } from './GlobalSearchModal';
import { SessionTimeoutModal } from './SessionTimeoutModal';
import { ToastContainer } from '../components/Toast';
import type { Role } from '../types/domain';
import { apiClient, type ServiceTierStatus } from '../services/apiClient';

export interface AppShellProps {
  children: React.ReactNode;
}

export const AppShell: React.FC<AppShellProps> = ({ children }) => {
  const {
    theme,
    setTheme,
    density,
    setDensity,
    role,
    setRole,
    environment,
    setEnvironment,
    activeView,
    setActiveView,
    setGlobalSearchOpen,
    setSessionTimeoutOpen,
    addToast,
    currentUser,
    logoutStaff,
    setPortalMode,
  } = useApp();

  const [alertsOpen, setAlertsOpen] = useState(false);
  const [govDropdownOpen, setGovDropdownOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [serviceStatus, setServiceStatus] = useState<ServiceTierStatus | null>(null);

  useEffect(() => {
    let mounted = true;
    const checkServices = async () => {
      try {
        const statuses = await apiClient.getServiceStatuses();
        if (mounted) setServiceStatus(statuses);
      } catch {
        // silent
      }
    };

    checkServices();
    const interval = setInterval(checkServices, 10000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, []);

  // Primary operational tabs (Clean, core workflows)
  const primaryNavItems = [
    { id: 'dashboard', label: 'Dashboard', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'agent-intake', label: 'Agent Complaint Intake', roles: ['CONTACT_CENTRE_AGENT', 'BRANCH_AGENT', 'ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'ADMIN'], badge: 'Live' },
    { id: 'reverseit', label: 'ReverseIt Customer Portal', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN', 'CONTACT_CENTRE_AGENT', 'BRANCH_AGENT'], isSpecial: true },
    { id: 'transactions', label: 'Transactions', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'reconciliation', label: 'Reconciliation', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'exceptions', label: 'Exceptions', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'], badge: '4' },
    { id: 'investigations', label: 'Investigations', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'reversals', label: 'Maker-Checker Reversals', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'ADMIN'], badge: '2' },
    { id: 'audit', label: 'Immutable Audit Trail', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'rules-health', label: 'Rules & Integration Health', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'executive-reports', label: 'Executive Reports & Heatmap', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'model-drift', label: 'ML Drift & Shadow Matrix', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] },
    { id: 'storybook', label: 'Design System & Storybook', roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'], isSpecial: true },
  ];

  const allowedPrimaryItems = primaryNavItems.filter((item) => item.roles.includes(role));

  const governanceNavItems = [
    { id: 'rules-health', label: 'Rules & Integration Health', desc: 'Active rules, adapter circuits & switch uptime' },
    { id: 'executive-reports', label: 'Executive Reports & Heatmap', desc: 'Dispute exposure & SLA regulatory summaries' },
    { id: 'model-drift', label: 'ML Drift & Shadow Matrix', desc: 'Predictive precision, explainability & drift metrics' },
  ];

  const roleLabels: Record<Role, string> = {
    ANALYST_MAKER: 'Analyst Maker',
    SUPERVISOR_CHECKER: 'Supervisor Checker',
    AUDITOR: 'Auditor (Read-Only)',
    ADMIN: 'Platform Admin',
    CONTACT_CENTRE_AGENT: 'Contact Centre Agent (Tier 1)',
    BRANCH_AGENT: 'Branch Operations Agent',
  };

  const allNavItems = [
    ...primaryNavItems,
    ...governanceNavItems.map((g) => ({ ...g, roles: ['ANALYST_MAKER', 'SUPERVISOR_CHECKER', 'AUDITOR', 'ADMIN'] })),
    { id: 'storybook', label: 'Design System & Storybook', roles: [] },
  ];

  const getBreadcrumb = () => {
    const item = allNavItems.find((i) => i.id === activeView);
    return item ? item.label : 'Operations Console';
  };

  return (
    <div className="min-h-screen bg-[var(--canvas)] text-[var(--text-primary)] flex flex-col selection:bg-[var(--primary-subtle)] selection:text-[var(--primary)]">
      {/* 1. Global Environment Banner (Sandbox vs Production + 3-Tier Indicators) */}
      <div
        className={clsx(
          'w-full px-3 sm:px-4 py-1 text-[11px] font-semibold flex items-center justify-between border-b select-none transition-colors gap-2 overflow-hidden',
          environment === 'SIMULATED_SANDBOX'
            ? 'bg-amber-100/90 text-amber-950 border-amber-300'
            : 'bg-emerald-100/90 text-emerald-950 border-emerald-300'
        )}
      >
        <div className="flex items-center gap-1.5 sm:gap-2 min-w-0 truncate">
          <span className="w-2 h-2 rounded-full animate-ping bg-amber-600 shrink-0" />
          <span className="truncate">
            {environment === 'SIMULATED_SANDBOX' ? (
              <>
                <span className="sm:hidden font-bold">SANDBOX (Emulated)</span>
                <span className="hidden sm:inline lg:hidden">SIMULATED SANDBOX — Emulated Mode</span>
                <span className="hidden lg:inline">
                  SIMULATED SANDBOX — Core Banking & Switch Adapters Operating in Emulated Mode (sample_settlement_nip.csv)
                </span>
              </>
            ) : (
              <>
                <span className="sm:hidden font-bold">PRODUCTION LIVE</span>
                <span className="hidden sm:inline">PRODUCTION ENVIRONMENT — Live Settlement Grid Active</span>
              </>
            )}
          </span>
        </div>

        <div className="flex items-center gap-2 sm:gap-3 shrink-0 text-[10px] font-mono-financial">
          {/* 3-Tier Status Indicators */}
          <div className="flex items-center gap-1.5 sm:gap-2">
            {/* Go Backend Status */}
            <span
              className={clsx(
                'inline-flex items-center gap-1 px-1.5 py-0.5 rounded border',
                serviceStatus?.backend.isLive
                  ? 'bg-emerald-50 text-emerald-800 border-emerald-300'
                  : 'bg-amber-50 text-amber-900 border-amber-300'
              )}
              title={serviceStatus?.backend.isLive ? `Backend connected (port 8080, latency ${serviceStatus.backend.latencyMs}ms)` : 'Backend running in Dual-Mode simulation'}
            >
              <span className={clsx('w-1.5 h-1.5 rounded-full', serviceStatus?.backend.isLive ? 'bg-emerald-600' : 'bg-amber-600')} />
              <span className="hidden xs:inline">API: </span>
              <span>{serviceStatus?.backend.isLive ? 'Live :8080' : 'Sim'}</span>
            </span>

            {/* FastAPI AI Service Status */}
            <span
              className={clsx(
                'inline-flex items-center gap-1 px-1.5 py-0.5 rounded border',
                serviceStatus?.aiService.isLive
                  ? 'bg-purple-50 text-purple-800 border-purple-300'
                  : 'bg-sky-50 text-sky-900 border-sky-300'
              )}
              title={serviceStatus?.aiService.isLive ? `AI Service connected (port 8000, latency ${serviceStatus.aiService.latencyMs}ms)` : 'AI running in Dual-Mode client fallback'}
            >
              <span className={clsx('w-1.5 h-1.5 rounded-full', serviceStatus?.aiService.isLive ? 'bg-purple-600' : 'bg-sky-600')} />
              <span className="hidden xs:inline">AI: </span>
              <span>{serviceStatus?.aiService.isLive ? 'Live :8000' : 'Dual'}</span>
            </span>
          </div>

          <div className="hidden sm:flex items-center gap-2">
            <span className="opacity-40">|</span>
            <button
              type="button"
              onClick={() =>
                setEnvironment(environment === 'SIMULATED_SANDBOX' ? 'PRODUCTION' : 'SIMULATED_SANDBOX')
              }
              className="hover:underline cursor-pointer font-mono-financial"
            >
              Switch to {environment === 'SIMULATED_SANDBOX' ? 'Production' : 'Sandbox'}
            </button>
            <span className="opacity-40">|</span>
            <button
              type="button"
              onClick={() => setSessionTimeoutOpen(true)}
              className="hover:underline cursor-pointer flex items-center gap-1 font-mono-financial"
            >
              <Clock className="w-3 h-3" />
              <span>Simulate Timeout</span>
            </button>
          </div>
        </div>
      </div>

      {/* 2. Top Header Bar (Clean, bright, NOT dark navy or black) */}
      <header className="sticky top-0 z-30 bg-[var(--surface)] border-b border-[var(--surface-border)] shadow-xs">
        <div className="max-w-7xl mx-auto px-3 sm:px-4 min-h-[52px] sm:h-14 py-1.5 sm:py-0 flex items-center justify-between gap-2 sm:gap-4">
          {/* Logo & Platform Title */}
          <div className="flex items-center gap-2.5 sm:gap-3 min-w-0 shrink-0">
            <img src="/logo.png" alt="SMARTREVERSAL Emblem" className="w-7 h-7 sm:w-8 sm:h-8 object-contain shrink-0" />
            <div className="min-w-0">
              <div className="font-bold text-[14px] sm:text-[15px] tracking-tight text-[var(--text-primary)] flex items-center gap-1.5">
                <span className="whitespace-nowrap">SMARTREVERSAL</span>
                <span className="text-[9px] sm:text-[10px] px-1.5 py-0.2 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] font-mono-financial font-medium text-[var(--text-muted)] shrink-0">
                  v0.2.0
                </span>
              </div>
              <div className="hidden sm:block text-[10px] sm:text-[11px] text-[var(--text-muted)] tracking-tight truncate">
                Financial Operations & Reversal Console
              </div>
            </div>
          </div>

          {/* Center: Global Transaction Search Input with Ctrl+K shortcut (Desktop) */}
          <div className="flex-1 max-w-md hidden md:block">
            <button
              type="button"
              onClick={() => setGlobalSearchOpen(true)}
              className="w-full flex items-center justify-between px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] text-[var(--text-muted)] hover:border-[var(--text-muted)] transition-colors cursor-pointer"
              aria-label="Open global transaction search (Shortcut Ctrl+K)"
            >
              <div className="flex items-center gap-2">
                <Search className="w-4 h-4 text-[var(--text-muted)]" aria-hidden="true" />
                <span className="truncate">Search transactions, RRNs, STANs, cases...</span>
              </div>
              <kbd className="px-1.5 py-0.5 text-[10px] bg-[var(--surface)] border border-[var(--surface-border)] rounded font-mono-financial text-[var(--text-secondary)] shrink-0">
                Ctrl K
              </kbd>
            </button>
          </div>

          {/* Desktop Right Controls: Staff Profile, Role Switcher, Density, Theme, Portal Switcher, Logout */}
          <div className="hidden md:flex items-center gap-2 text-[12px] shrink-0">
            {/* Logged-in Staff Profile */}
            <div className="flex items-center gap-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded px-2.5 py-1">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse shrink-0" />
              <div className="flex flex-col text-left">
                <span className="text-[11px] font-bold text-[var(--text-primary)] leading-tight whitespace-nowrap">
                  {currentUser?.name || (role === 'SUPERVISOR_CHECKER' ? 'Chioma Eze' : 'Ade Bello')}
                </span>
                <span className="text-[10px] text-[var(--text-muted)] font-mono-financial leading-tight whitespace-nowrap">
                  {roleLabels[role]}
                </span>
              </div>
            </div>

            {/* Persona Switcher (Demo Convenience) */}
            <div className="flex items-center gap-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded px-2 py-1">
              <User className="w-3.5 h-3.5 text-[var(--text-muted)] shrink-0" aria-hidden="true" />
              <label htmlFor="role-select" className="sr-only">
                Role Persona
              </label>
              <select
                id="role-select"
                value={role}
                onChange={(e) => {
                  const newRole = e.target.value as Role;
                  setRole(newRole);
                  addToast({
                    type: 'info',
                    title: 'Persona Switched',
                    message: `Switched operational role to ${roleLabels[newRole]}`,
                  });
                }}
                className="bg-transparent text-[var(--text-primary)] font-medium focus:outline-none cursor-pointer text-xs"
              >
                <option value="ANALYST_MAKER">Maker: Analyst</option>
                <option value="SUPERVISOR_CHECKER">Checker: Supervisor</option>
                <option value="AUDITOR">Auditor (Read-Only)</option>
                <option value="ADMIN">Admin: Systems</option>
              </select>
            </div>

            {/* Switch to Customer Portal Button */}
            <button
              type="button"
              onClick={() => setPortalMode('customer')}
              className="p-1.5 px-2 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer flex items-center gap-1 text-xs font-medium shrink-0"
              title="Switch to Customer Dispute Portal"
            >
              <ArrowLeftRight className="w-3.5 h-3.5 text-[var(--primary)] shrink-0" />
              <span className="hidden lg:inline whitespace-nowrap">Customer Portal</span>
            </button>

            {/* Density Toggle (Comfortable vs Compact) */}
            <button
              type="button"
              onClick={() => setDensity(density === 'comfortable' ? 'compact' : 'comfortable')}
              className="p-1.5 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer shrink-0"
              title={`Switch density mode (currently ${density})`}
              aria-label="Toggle Table Density"
            >
              <Sliders className="w-4 h-4" aria-hidden="true" />
            </button>

            {/* Theme Toggle (Daylight vs Soft Dim) */}
            <button
              type="button"
              onClick={() => setTheme(theme === 'daylight' ? 'soft-dim' : 'daylight')}
              className="p-1.5 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer shrink-0"
              title={`Switch theme (currently ${theme})`}
              aria-label="Toggle Theme Mode"
            >
              {theme === 'daylight' ? (
                <Moon className="w-4 h-4 text-slate-700" aria-hidden="true" />
              ) : (
                <Sun className="w-4 h-4 text-amber-400" aria-hidden="true" />
              )}
            </button>

            {/* Action Items Bell Dropdown */}
            <div className="relative shrink-0">
              <button
                type="button"
                onClick={() => setAlertsOpen((prev) => !prev)}
                className="relative p-1.5 rounded text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer"
                aria-label="Alerts and Action Items (2 pending)"
              >
                <Bell className="w-4 h-4" aria-hidden="true" />
                <span className="absolute -top-1 -right-1 w-2.5 h-2.5 bg-rose-600 rounded-full ring-2 ring-[var(--surface)]" />
              </button>

              {alertsOpen && (
                <div className="absolute right-0 top-full mt-2 w-72 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xl p-3 z-40 space-y-2">
                  <div className="flex items-center justify-between pb-1.5 border-b border-[var(--surface-border)] font-semibold text-[13px]">
                    <span>Pending Action Items</span>
                    <span className="text-[11px] font-mono-financial bg-rose-100 text-rose-800 px-1.5 py-0.2 rounded">
                      2 Urgent
                    </span>
                  </div>
                  <div
                    onClick={() => {
                      setActiveView('reversals');
                      setAlertsOpen(false);
                    }}
                    className="p-2 rounded hover:bg-[var(--canvas-subdued)] cursor-pointer space-y-0.5 text-[12px]"
                  >
                    <div className="font-semibold text-[var(--text-primary)] flex items-center justify-between">
                      <span>Maker-Checker Signoff</span>
                      <span className="text-amber-700 font-mono-financial">₦150k</span>
                    </div>
                    <p className="text-[var(--text-muted)]">NIP-9901 requires supervisor authorization.</p>
                  </div>
                  <div
                    onClick={() => {
                      setActiveView('exceptions');
                      setAlertsOpen(false);
                    }}
                    className="p-2 rounded hover:bg-[var(--canvas-subdued)] cursor-pointer space-y-0.5 text-[12px]"
                  >
                    <div className="font-semibold text-[var(--text-primary)] flex items-center justify-between">
                      <span>POS Timeout Discrepancy</span>
                      <span className="text-rose-700 font-mono-financial">Code 91</span>
                    </div>
                    <p className="text-[var(--text-muted)]">Terminal TERM-8801 debit without dispense.</p>
                  </div>
                </div>
              )}
            </div>

            {/* Logout Button */}
            <button
              type="button"
              onClick={() => {
                logoutStaff();
                addToast({
                  type: 'info',
                  title: 'Logged Out',
                  message: 'Staff session terminated.',
                });
              }}
              className="p-1.5 px-2 rounded text-rose-700 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-950/40 border border-rose-200 dark:border-rose-900 cursor-pointer flex items-center gap-1 text-xs font-semibold shrink-0"
              title="Log Out of Operations Console"
            >
              <LogOut className="w-3.5 h-3.5" />
              <span>Sign Out</span>
            </button>
          </div>

          {/* Mobile Right Controls: Search, Customer Portal, Bell, Mobile Menu Hamburger */}
          <div className="flex md:hidden items-center gap-1 sm:gap-1.5 text-[12px] shrink-0">
            {/* Mobile Search Button */}
            <button
              type="button"
              onClick={() => setGlobalSearchOpen(true)}
              className="p-1.5 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer"
              title="Search transactions (Ctrl+K)"
              aria-label="Open transaction search"
            >
              <Search className="w-4 h-4" />
            </button>

            {/* Switch to Customer Portal */}
            <button
              type="button"
              onClick={() => setPortalMode('customer')}
              className="p-1.5 px-2 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer flex items-center gap-1 text-xs font-medium"
              title="Switch to Customer Dispute Portal"
            >
              <ArrowLeftRight className="w-3.5 h-3.5 text-[var(--primary)]" />
              <span className="text-[11px] font-semibold">Customer</span>
            </button>

            {/* Action Items Bell */}
            <div className="relative">
              <button
                type="button"
                onClick={() => setAlertsOpen((prev) => !prev)}
                className="relative p-1.5 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border border-[var(--surface-border)] cursor-pointer"
                aria-label="Alerts"
              >
                <Bell className="w-4 h-4" />
                <span className="absolute -top-1 -right-1 w-2.5 h-2.5 bg-rose-600 rounded-full ring-2 ring-[var(--surface)]" />
              </button>

              {alertsOpen && (
                <div className="absolute right-0 top-full mt-2 w-72 bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl shadow-2xl p-3 z-50 space-y-2">
                  <div className="flex items-center justify-between pb-1.5 border-b border-[var(--surface-border)] font-semibold text-[13px]">
                    <span>Pending Action Items</span>
                    <span className="text-[11px] font-mono-financial bg-rose-100 text-rose-800 px-1.5 py-0.2 rounded">
                      2 Urgent
                    </span>
                  </div>
                  <div
                    onClick={() => {
                      setActiveView('reversals');
                      setAlertsOpen(false);
                    }}
                    className="p-2 rounded hover:bg-[var(--canvas-subdued)] cursor-pointer space-y-0.5 text-[12px]"
                  >
                    <div className="font-semibold text-[var(--text-primary)] flex items-center justify-between">
                      <span>Maker-Checker Signoff</span>
                      <span className="text-amber-700 font-mono-financial">₦150k</span>
                    </div>
                    <p className="text-[var(--text-muted)]">NIP-9901 requires supervisor authorization.</p>
                  </div>
                  <div
                    onClick={() => {
                      setActiveView('exceptions');
                      setAlertsOpen(false);
                    }}
                    className="p-2 rounded hover:bg-[var(--canvas-subdued)] cursor-pointer space-y-0.5 text-[12px]"
                  >
                    <div className="font-semibold text-[var(--text-primary)] flex items-center justify-between">
                      <span>POS Timeout Discrepancy</span>
                      <span className="text-rose-700 font-mono-financial">Code 91</span>
                    </div>
                    <p className="text-[var(--text-muted)]">Terminal TERM-8801 debit without dispense.</p>
                  </div>
                </div>
              )}
            </div>

            {/* Mobile Hamburger Menu Toggle */}
            <button
              type="button"
              onClick={() => setMobileMenuOpen((prev) => !prev)}
              className={clsx(
                'p-1.5 rounded-lg border transition-colors cursor-pointer',
                mobileMenuOpen
                  ? 'bg-[var(--primary-subtle)] text-[var(--primary)] border-[var(--primary)]'
                  : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--canvas-subdued)] border-[var(--surface-border)]'
              )}
              aria-label={mobileMenuOpen ? 'Close staff menu' : 'Open staff menu'}
            >
              {mobileMenuOpen ? <X className="w-4 h-4" /> : <Menu className="w-4 h-4" />}
            </button>
          </div>
        </div>

        {/* Mobile Slide-down Console Drawer */}
        {mobileMenuOpen && (
          <div className="md:hidden border-t border-[var(--surface-border)] bg-[var(--surface)] p-3 space-y-3 animate-in slide-in-from-top-2 duration-150 shadow-xl">
            {/* Active Staff Card */}
            <div className="p-2.5 rounded-xl bg-[var(--canvas-subdued)] border border-[var(--surface-border)] flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse shrink-0" />
                <div>
                  <div className="text-[12px] font-bold text-[var(--text-primary)]">
                    {currentUser?.name || (role === 'SUPERVISOR_CHECKER' ? 'Chioma Eze' : 'Ade Bello')}
                  </div>
                  <div className="text-[10px] text-[var(--text-muted)] font-mono-financial">
                    {roleLabels[role]}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-1.5">
                <User className="w-3.5 h-3.5 text-[var(--text-muted)]" />
                <select
                  value={role}
                  onChange={(e) => {
                    const newRole = e.target.value as Role;
                    setRole(newRole);
                    addToast({
                      type: 'info',
                      title: 'Persona Switched',
                      message: `Switched operational role to ${roleLabels[newRole]}`,
                    });
                  }}
                  className="bg-[var(--surface)] border border-[var(--surface-border)] rounded px-1.5 py-0.5 text-xs font-semibold text-[var(--text-primary)] focus:outline-none"
                >
                  <option value="ANALYST_MAKER">Maker: Analyst</option>
                  <option value="SUPERVISOR_CHECKER">Checker: Supervisor</option>
                  <option value="AUDITOR">Auditor</option>
                  <option value="ADMIN">Admin</option>
                </select>
              </div>
            </div>

            {/* Quick Toggle Controls Grid */}
            <div className="grid grid-cols-2 gap-2 text-[11px]">
              {/* Theme Toggle */}
              <button
                type="button"
                onClick={() => setTheme(theme === 'daylight' ? 'soft-dim' : 'daylight')}
                className="p-2 rounded-lg bg-[var(--canvas-subdued)] border border-[var(--surface-border)] flex items-center justify-between text-[var(--text-secondary)] font-medium cursor-pointer"
              >
                <span>Theme</span>
                <span className="flex items-center gap-1 font-semibold text-[var(--text-primary)]">
                  {theme === 'daylight' ? <Moon className="w-3.5 h-3.5" /> : <Sun className="w-3.5 h-3.5 text-amber-400" />}
                  {theme === 'daylight' ? 'Daylight' : 'Soft Dim'}
                </span>
              </button>

              {/* Table Density */}
              <button
                type="button"
                onClick={() => setDensity(density === 'comfortable' ? 'compact' : 'comfortable')}
                className="p-2 rounded-lg bg-[var(--canvas-subdued)] border border-[var(--surface-border)] flex items-center justify-between text-[var(--text-secondary)] font-medium cursor-pointer"
              >
                <span>Density</span>
                <span className="flex items-center gap-1 font-semibold text-[var(--text-primary)] capitalize">
                  <Sliders className="w-3.5 h-3.5" />
                  {density}
                </span>
              </button>

              {/* Environment Switcher */}
              <button
                type="button"
                onClick={() =>
                  setEnvironment(environment === 'SIMULATED_SANDBOX' ? 'PRODUCTION' : 'SIMULATED_SANDBOX')
                }
                className="p-2 rounded-lg bg-[var(--canvas-subdued)] border border-[var(--surface-border)] flex items-center justify-between text-[var(--text-secondary)] font-medium cursor-pointer"
              >
                <span>Env</span>
                <span className="font-semibold text-amber-700 dark:text-amber-400 truncate">
                  {environment === 'SIMULATED_SANDBOX' ? 'Sandbox' : 'Production'}
                </span>
              </button>

              {/* Sign Out */}
              <button
                type="button"
                onClick={() => {
                  logoutStaff();
                  setMobileMenuOpen(false);
                  addToast({
                    type: 'info',
                    title: 'Logged Out',
                    message: 'Staff session terminated.',
                  });
                }}
                className="p-2 rounded-lg bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900 text-rose-700 dark:text-rose-400 flex items-center justify-between font-semibold cursor-pointer"
              >
                <span>Session</span>
                <span className="flex items-center gap-1">
                  <LogOut className="w-3.5 h-3.5" />
                  Sign Out
                </span>
              </button>
            </div>
          </div>
        )}

        {/* 3. Primary Navigation Bar (Smooth touch-scrollable) */}
        <nav
          className="bg-[var(--surface)] border-t border-[var(--surface-border-subtle)] overflow-x-auto scrollbar-none"
          aria-label="Primary Application Navigation"
        >
          <div className="max-w-7xl mx-auto px-2 sm:px-4 flex items-center gap-0.5 sm:gap-1 flex-nowrap min-w-max">
            {/* Primary Core Operational Items */}
            {allowedPrimaryItems.map((item) => {
              const isActive = activeView === item.id;
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => {
                    setActiveView(item.id);
                    setMobileMenuOpen(false);
                  }}
                  className={clsx(
                    'px-2.5 sm:px-3.5 py-2 sm:py-2.5 text-[12px] sm:text-[13px] font-medium border-b-2 transition-colors cursor-pointer flex items-center gap-1 sm:gap-1.5 whitespace-nowrap shrink-0',
                    isActive
                      ? 'border-[var(--primary)] text-[var(--primary)] font-semibold'
                      : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--surface-border)]'
                  )}
                  aria-current={isActive ? 'page' : undefined}
                >
                  <span>{item.label}</span>
                  {item.badge && (
                    <span
                      className={clsx(
                        'text-[9px] sm:text-[10px] font-mono-financial px-1.5 py-0.2 rounded-full shrink-0',
                        isActive
                          ? 'bg-[var(--primary)] text-white'
                          : 'bg-[var(--canvas-subdued)] text-[var(--text-muted)]'
                      )}
                    >
                      {item.badge}
                    </span>
                  )}
                </button>
              );
            })}

            {/* Governance & Intelligence Dropdown */}
            <div className="relative shrink-0">
              <button
                type="button"
                onClick={() => setGovDropdownOpen((prev) => !prev)}
                className={clsx(
                  'px-2.5 sm:px-3.5 py-2 sm:py-2.5 text-[12px] sm:text-[13px] font-medium border-b-2 transition-colors cursor-pointer flex items-center gap-1.5 whitespace-nowrap',
                  governanceNavItems.some((g) => g.id === activeView)
                    ? 'border-[var(--primary)] text-[var(--primary)] font-semibold'
                    : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--surface-border)]'
                )}
              >
                <Shield className="w-3.5 h-3.5 text-[var(--primary)]" />
                <span>Governance & Intelligence</span>
                <ChevronDown className="w-3.5 h-3.5 opacity-60" />
              </button>

              {govDropdownOpen && (
                <div
                  className="absolute left-0 top-full mt-1 w-72 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xl p-2 z-50 space-y-1"
                  onMouseLeave={() => setGovDropdownOpen(false)}
                >
                  <div className="px-2 py-1 text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)] border-b border-[var(--surface-border)] mb-1">
                    Compliance & System Intelligence
                  </div>
                  {governanceNavItems.map((item) => {
                    const isItemActive = activeView === item.id;
                    return (
                      <button
                        key={item.id}
                        type="button"
                        onClick={() => {
                          setActiveView(item.id);
                          setGovDropdownOpen(false);
                          setMobileMenuOpen(false);
                        }}
                        className={clsx(
                          'w-full text-left p-2 rounded text-xs transition-colors cursor-pointer flex flex-col',
                          isItemActive
                            ? 'bg-[var(--primary-subtle)] text-[var(--primary)] font-bold'
                            : 'hover:bg-[var(--canvas-subdued)] text-[var(--text-primary)]'
                        )}
                      >
                        <span>{item.label}</span>
                        <span className="text-[10px] text-[var(--text-muted)] font-normal">
                          {item.desc}
                        </span>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>

            {/* Right-aligned Storybook / Design System discreet trigger */}
            <div className="ml-auto flex items-center shrink-0">
              <button
                type="button"
                onClick={() => {
                  setActiveView('storybook');
                  setMobileMenuOpen(false);
                }}
                className={clsx(
                  'px-2.5 py-1 text-[11px] font-mono-financial rounded border transition-colors cursor-pointer flex items-center gap-1',
                  activeView === 'storybook'
                    ? 'bg-purple-100 dark:bg-purple-950 text-purple-900 dark:text-purple-200 border-purple-300 font-bold'
                    : 'text-[var(--text-muted)] border-transparent hover:border-[var(--surface-border)] hover:bg-[var(--canvas-subdued)]'
                )}
                title="Open Design System & Storybook"
              >
                <BookOpen className="w-3 h-3 text-purple-600" />
                <span>Storybook & Specs</span>
              </button>
            </div>
          </div>
        </nav>
      </header>

      {/* 4. Breadcrumb Bar */}
      <div className="bg-[var(--canvas-subdued)] border-b border-[var(--surface-border-subtle)] text-[11px] sm:text-[12px] py-1.5 px-3 sm:px-4 select-none">
        <div className="max-w-7xl mx-auto flex items-center gap-1 sm:gap-1.5 text-[var(--text-muted)] min-w-0">
          <span className="hidden sm:inline">SMARTREVERSAL</span>
          <ChevronRight className="hidden sm:inline w-3.5 h-3.5 shrink-0" />
          <span className="hidden xs:inline">Banking Operations</span>
          <span className="xs:hidden">Ops</span>
          <ChevronRight className="w-3.5 h-3.5 shrink-0" />
          <strong className="text-[var(--text-primary)] font-semibold truncate">{getBreadcrumb()}</strong>
          <span className="hidden md:inline ml-auto font-mono-financial text-[11px] shrink-0">
            Mode: {theme === 'daylight' ? 'Daylight (Section 9)' : 'Soft Dim'} • Density: {density}
          </span>
        </div>
      </div>

      {/* 5. Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-4 md:p-6 space-y-6" role="main">
        {children}
      </main>

      {/* Modals & Portals */}
      <GlobalSearchModal />
      <SessionTimeoutModal />
      <ToastContainer />
    </div>
  );
};
