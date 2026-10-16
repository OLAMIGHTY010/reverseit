import React, { useState } from 'react';
import {
  ShieldCheck,
  UserCheck,
  Lock,
  ArrowLeft,
  Building2,
  KeyRound,
  AlertCircle,
  CheckCircle2,
  Sparkles,
  ArrowRight,
} from 'lucide-react';
import { useApp } from '../../context/AppContext';
import { apiClient } from '../../services/apiClient';
import type { Role } from '../../types/domain';

export const StaffLoginView: React.FC = () => {
  const { loginStaff, setPortalMode, addToast } = useApp();

  const [institution, setInstitution] = useState('DEMO-BANK');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [otp, setOtp] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  // Quick 1-Click Demo Login
  const handleDemoLogin = async (persona: 'MAKER' | 'CHECKER') => {
    setIsLoading(true);
    setErrorMessage(null);

    const isChecker = persona === 'CHECKER';
    const demoEmail = isChecker ? 'supervisor@demo.example' : 'officer@demo.example';
    const demoPassword = 'DemoPassword123!';
    const demoRole: Role = isChecker ? 'SUPERVISOR_CHECKER' : 'ANALYST_MAKER';
    const demoName = isChecker ? 'Chioma Eze (Supervisor)' : 'Ade Bello (Reversal Officer)';

    try {
      const res = await apiClient.login({
        institution: 'DEMO-BANK',
        email: demoEmail,
        password: demoPassword,
        otp: '123456',
      });

      loginStaff(
        {
          id: res.user_id,
          name: demoName,
          email: demoEmail,
          role: demoRole,
          institution: 'DEMO-BANK',
          rawRoles: res.roles,
          mfa: res.mfa,
        },
        res.access_token
      );

      addToast({
        type: 'success',
        title: `Signed in as ${isChecker ? 'Checker' : 'Maker'}`,
        message: `Welcome, ${demoName}. Session established with dual-control privileges.`,
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Login failed';
      setErrorMessage(msg);
      addToast({
        type: 'error',
        title: 'Authentication Error',
        message: msg,
      });
    } finally {
      setIsLoading(false);
    }
  };

  // Standard Form Submit
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email || !password) {
      setErrorMessage('Please enter both email and password.');
      return;
    }

    setIsLoading(true);
    setErrorMessage(null);

    try {
      const res = await apiClient.login({
        institution: institution || 'DEMO-BANK',
        email,
        password,
        otp: otp || undefined,
      });

      // Map backend roles to frontend Role
      let assignedRole: Role = 'ANALYST_MAKER';
      if (res.roles.includes('SUPERVISOR')) {
        assignedRole = 'SUPERVISOR_CHECKER';
      } else if (res.roles.includes('AUDITOR')) {
        assignedRole = 'AUDITOR';
      } else if (res.roles.includes('ADMIN_INTEGRATION') || res.roles.includes('ADMIN')) {
        assignedRole = 'ADMIN';
      }

      loginStaff(
        {
          id: res.user_id,
          name: email.split('@')[0].replace('.', ' ').toUpperCase(),
          email,
          role: assignedRole,
          institution: institution || 'DEMO-BANK',
          rawRoles: res.roles,
          mfa: res.mfa,
        },
        res.access_token
      );

      addToast({
        type: 'success',
        title: 'Staff Login Successful',
        message: `Authenticated as ${assignedRole}. Token attached to API gateway.`,
      });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Invalid credentials';
      setErrorMessage(msg);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-[var(--canvas)] flex flex-col justify-between p-4 sm:p-6 lg:p-8">
      {/* Top Header bar with return link */}
      <div className="max-w-5xl mx-auto w-full flex items-center justify-between">
        <button
          type="button"
          onClick={() => setPortalMode('customer')}
          className="inline-flex items-center gap-2 text-xs sm:text-sm font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors cursor-pointer"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>Return to Customer Dispute Portal</span>
        </button>

        <div className="flex items-center gap-2 text-[11px] font-mono-financial px-2.5 py-1 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-muted)]">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          <span>Core API Gateway :8080</span>
        </div>
      </div>

      {/* Main Container */}
      <div className="max-w-4xl mx-auto w-full my-8">
        {/* Title & Badge */}
        <div className="text-center mb-8 space-y-2">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--primary-subtle)] text-[var(--primary)] text-xs font-semibold">
            <ShieldCheck className="w-4 h-4" />
            <span>Bank Staff Dual-Control Gateway</span>
          </div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-[var(--text-primary)]">
            SMARTREVERSAL Operations Console
          </h1>
          <p className="text-sm text-[var(--text-muted)] max-w-md mx-auto">
            Secured internal portal for exception triage, investigation copilot, and maker-checker reversal authorization.
          </p>
        </div>

        {/* 2 One-Click Demo Personas Banner */}
        <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl p-5 shadow-sm mb-8">
          <div className="flex items-center gap-2 mb-4 text-xs font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <Sparkles className="w-4 h-4 text-amber-500" />
            <span>Quick Demo Access (2 Production Roles Seeded in Backend)</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Persona 1: Maker / Analyst */}
            <div className="border border-[var(--surface-border)] rounded-lg p-4 bg-[var(--canvas-subdued)] hover:border-[var(--primary)] transition-all flex flex-col justify-between">
              <div>
                <div className="flex items-center justify-between mb-2">
                  <span className="inline-flex items-center gap-1.5 text-xs font-semibold px-2 py-0.5 rounded bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-primary)]">
                    <UserCheck className="w-3.5 h-3.5 text-[var(--primary)]" />
                    Maker: Reversal Officer
                  </span>
                  <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                    officer@demo.example
                  </span>
                </div>
                <h3 className="font-semibold text-sm text-[var(--text-primary)]">Ade Bello</h3>
                <p className="text-xs text-[var(--text-muted)] mt-1 leading-relaxed">
                  Triage discrepancies, investigate switch events, and propose reversals. Enforces Separation of Duties.
                </p>
              </div>

              <button
                type="button"
                disabled={isLoading}
                onClick={() => handleDemoLogin('MAKER')}
                className="mt-4 w-full py-2 px-3 bg-[var(--primary)] hover:bg-[var(--primary-hover)] text-white text-xs font-semibold rounded shadow-xs transition-colors flex items-center justify-center gap-2 cursor-pointer disabled:opacity-50"
              >
                <span>Sign in as Maker</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Persona 2: Checker / Supervisor */}
            <div className="border border-[var(--surface-border)] rounded-lg p-4 bg-[var(--canvas-subdued)] hover:border-[var(--primary)] transition-all flex flex-col justify-between">
              <div>
                <div className="flex items-center justify-between mb-2">
                  <span className="inline-flex items-center gap-1.5 text-xs font-semibold px-2 py-0.5 rounded bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-primary)]">
                    <ShieldCheck className="w-3.5 h-3.5 text-slate-700 dark:text-slate-300" />
                    Checker: Supervisor
                  </span>
                  <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                    supervisor@demo.example
                  </span>
                </div>
                <h3 className="font-semibold text-sm text-[var(--text-primary)]">Chioma Eze</h3>
                <p className="text-xs text-[var(--text-muted)] mt-1 leading-relaxed">
                  Dual-control authorization authority. Authorizes or rejects proposed reversals and audits ledger integrity.
                </p>
              </div>

              <button
                type="button"
                disabled={isLoading}
                onClick={() => handleDemoLogin('CHECKER')}
                className="mt-4 w-full py-2 px-3 bg-slate-800 hover:bg-slate-900 dark:bg-slate-700 dark:hover:bg-slate-600 text-white text-xs font-semibold rounded shadow-xs transition-colors flex items-center justify-center gap-2 cursor-pointer disabled:opacity-50"
              >
                <span>Sign in as Checker</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>

        {/* Traditional Credentials Form */}
        <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl p-6 shadow-sm">
          <div className="border-b border-[var(--surface-border)] pb-3 mb-4">
            <h2 className="text-sm font-semibold text-[var(--text-primary)] flex items-center gap-2">
              <Lock className="w-4 h-4 text-[var(--text-muted)]" />
              <span>Standard Staff Credentials</span>
            </h2>
            <p className="text-xs text-[var(--text-muted)]">
              Connects directly to the live Go backend endpoint: <code>POST /v1/auth/login</code>
            </p>
          </div>

          {errorMessage && (
            <div className="mb-4 p-3 rounded-md bg-rose-50 dark:bg-rose-950/40 border border-rose-200 dark:border-rose-900 text-xs text-rose-800 dark:text-rose-200 flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0 text-rose-600" />
              <span>{errorMessage}</span>
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {/* Institution */}
              <div>
                <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">
                  Institution Code
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-2.5 flex items-center pointer-events-none text-[var(--text-muted)]">
                    <Building2 className="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    value={institution}
                    onChange={(e) => setInstitution(e.target.value.toUpperCase())}
                    placeholder="DEMO-BANK"
                    className="w-full pl-9 pr-3 py-2 text-xs bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md font-mono-financial focus:outline-none focus:border-[var(--primary)] text-[var(--text-primary)]"
                  />
                </div>
              </div>

              {/* Email */}
              <div>
                <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">
                  Staff Email Address
                </label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="officer@demo.example"
                  className="w-full px-3 py-2 text-xs bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md focus:outline-none focus:border-[var(--primary)] text-[var(--text-primary)]"
                />
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {/* Password */}
              <div>
                <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">
                  Staff Password
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-2.5 flex items-center pointer-events-none text-[var(--text-muted)]">
                    <Lock className="w-4 h-4" />
                  </div>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="••••••••••••"
                    className="w-full pl-9 pr-3 py-2 text-xs bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md focus:outline-none focus:border-[var(--primary)] text-[var(--text-primary)]"
                  />
                </div>
              </div>

              {/* MFA / OTP */}
              <div>
                <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">
                  One-Time Passcode (OTP / Synthetic Authenticator)
                </label>
                <div className="relative">
                  <div className="absolute inset-y-0 left-0 pl-2.5 flex items-center pointer-events-none text-[var(--text-muted)]">
                    <KeyRound className="w-4 h-4" />
                  </div>
                  <input
                    type="text"
                    maxLength={6}
                    value={otp}
                    onChange={(e) => setOtp(e.target.value)}
                    placeholder="Optional for demo"
                    className="w-full pl-9 pr-3 py-2 text-xs bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-md font-mono-financial focus:outline-none focus:border-[var(--primary)] text-[var(--text-primary)]"
                  />
                </div>
              </div>
            </div>

            <button
              type="submit"
              disabled={isLoading}
              className="w-full py-2.5 px-4 bg-[var(--primary)] hover:bg-[var(--primary-hover)] text-white text-xs font-semibold rounded-md shadow-xs transition-colors flex items-center justify-center gap-2 cursor-pointer disabled:opacity-50"
            >
              {isLoading ? (
                <span>Authenticating with Go API...</span>
              ) : (
                <>
                  <CheckCircle2 className="w-4 h-4" />
                  <span>Authenticate & Enter Operations Console</span>
                </>
              )}
            </button>
          </form>
        </div>
      </div>

      {/* Footer disclaimer */}
      <footer className="text-center text-[11px] text-[var(--text-muted)] space-y-1">
        <p>SMARTREVERSAL Enterprise Settlement Platform • CBN Regulatory Compliance Certified</p>
        <p>All operator actions are signed, stamped with Merkle-tree roots, and recorded to immutable audit trails.</p>
      </footer>
    </div>
  );
};
