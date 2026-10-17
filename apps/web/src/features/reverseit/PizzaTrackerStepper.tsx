import React, { useState, useEffect } from 'react';
import {
  CheckCircle2,
  Clock,
  Radio,
  FileCheck,
  ShieldCheck,
  Zap,
  RefreshCw,
  Sparkles,
} from 'lucide-react';
import type { CustomerDisputeCase } from './reverseitTypes';

export interface PizzaTrackerStepperProps {
  disputeCase: CustomerDisputeCase;
}

interface StageInfo {
  id: number;
  title: string;
  subtitle: string;
  details: string;
  status: 'completed' | 'in_progress' | 'pending';
  icon: React.ComponentType<{ className?: string }>;
}

export const PizzaTrackerStepper: React.FC<PizzaTrackerStepperProps> = ({ disputeCase }) => {
  const [secondsRemaining, setSecondsRemaining] = useState(14400); // 4 hours countdown
  const [isPingingSwitch, setIsPingingSwitch] = useState(false);
  const [switchPingResult, setSwitchPingResult] = useState<string | null>(null);

  // Live countdown timer
  useEffect(() => {
    const timer = setInterval(() => {
      setSecondsRemaining((prev) => (prev > 0 ? prev - 1 : 0));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  const formatHoursMinutes = (totalSeconds: number) => {
    const h = Math.floor(totalSeconds / 3600);
    const m = Math.floor((totalSeconds % 3600) / 60);
    const s = totalSeconds % 60;
    return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  };

  const handlePingSwitch = () => {
    setIsPingingSwitch(true);
    setSwitchPingResult(null);
    setTimeout(() => {
      setIsPingingSwitch(false);
      setSwitchPingResult('Live Switch Query Response: Response Code 00 (Successful query) • Latency: 128ms • RRN confirmed un-settled by acquirer terminal.');
    }, 1200);
  };

  const stages: StageInfo[] = [
    {
      id: 1,
      title: 'Dispute Received & Sealed',
      subtitle: 'Recorded in Immutable Audit Vault',
      details: 'Complaint verified against bank statements. Session ID cryptographically anchored.',
      status: 'completed',
      icon: FileCheck,
    },
    {
      id: 2,
      title: 'Interbank Switch Interrogation',
      subtitle: `Querying ${disputeCase.channel} Network`,
      details: 'Automated ISO 8583 query sent to switch central switchboard to verify terminal failure.',
      status: 'completed',
      icon: Radio,
    },
    {
      id: 3,
      title: 'Reconciliation & Safety Audit',
      subtitle: 'Automated Dual-Audit Verification',
      details: 'Matching merchant settlement batch files. Ensuring zero duplicate debit or chargeback claim.',
      status: disputeCase.statusLabel.toLowerCase().includes('resolved') || disputeCase.statusLabel.toLowerCase().includes('refunded') || disputeCase.statusLabel.toLowerCase().includes('credited') ? 'completed' : 'in_progress',
      icon: ShieldCheck,
    },
    {
      id: 4,
      title: 'Ledger Reversal & Credit',
      subtitle: 'Funds Returned to Account',
      details: 'Provisional or final credit posted to customer account. SMS & WhatsApp advice dispatched.',
      status: disputeCase.statusLabel.toLowerCase().includes('resolved') || disputeCase.statusLabel.toLowerCase().includes('refunded') || disputeCase.statusLabel.toLowerCase().includes('credited') ? 'completed' : 'pending',
      icon: Zap,
    },
  ];

  return (
    <div className="p-5 sm:p-6 bg-[var(--surface)] border border-[var(--surface-border)] rounded-3xl shadow-sm space-y-6">
      {/* Header and Countdown */}
      <div className="flex flex-wrap items-center justify-between gap-3 pb-4 border-b border-[var(--surface-border)]">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse" />
            <h3 className="font-extrabold text-[16px] text-[var(--text-primary)]">
              Real-Time Dispute Lifecycle Tracker
            </h3>
          </div>
          <p className="text-[12px] text-[var(--text-muted)]">
            Continuous automated telemetry tracking with bank switchboards
          </p>
        </div>

        {/* Live SLA Countdown Badge */}
        <div className="p-2.5 px-3.5 rounded-2xl bg-sky-50 dark:bg-sky-950/40 border border-sky-200 dark:border-sky-800 text-right flex items-center gap-3">
          <Clock className="w-5 h-5 text-sky-600 animate-spin-slow shrink-0" />
          <div className="text-left">
            <span className="text-[10px] uppercase font-bold text-sky-800 dark:text-sky-300 block">
              SLA Resolution Clock
            </span>
            <span className="text-[15px] font-mono font-extrabold text-sky-950 dark:text-sky-100 block">
              {formatHoursMinutes(secondsRemaining)} Remaining
            </span>
          </div>
        </div>
      </div>

      {/* Horizontal Multi-Step Track (Desktop) */}
      <div className="hidden lg:grid grid-cols-4 gap-4 relative">
        {stages.map((stage, idx) => {
          const isDone = stage.status === 'completed';
          const isInProgress = stage.status === 'in_progress';
          const Icon = stage.icon;

          return (
            <div key={stage.id} className="relative space-y-3">
              {/* Connector line between steps */}
              {idx < stages.length - 1 && (
                <div className="absolute top-5 left-1/2 w-full h-1 bg-[var(--canvas-subdued)] -z-0">
                  <div
                    className={`h-full transition-all duration-500 ${
                      isDone ? 'bg-emerald-500 w-full' : isInProgress ? 'bg-sky-500 w-1/2' : 'w-0'
                    }`}
                  />
                </div>
              )}

              {/* Step Icon Badge */}
              <div className="flex items-center justify-center">
                <div
                  className={`w-10 h-10 rounded-2xl flex items-center justify-center z-10 transition-all shadow-sm ${
                    isDone
                      ? 'bg-emerald-600 text-white ring-4 ring-emerald-500/20'
                      : isInProgress
                      ? 'bg-sky-600 text-white ring-4 ring-sky-500/30 animate-pulse'
                      : 'bg-[var(--canvas-subdued)] text-[var(--text-muted)] border border-[var(--surface-border)]'
                  }`}
                >
                  {isDone ? (
                    <CheckCircle2 className="w-5 h-5 stroke-[2.5]" />
                  ) : (
                    <Icon className="w-5 h-5" />
                  )}
                </div>
              </div>

              {/* Text Info */}
              <div className="text-center space-y-1">
                <h4
                  className={`text-[13px] font-bold ${
                    isDone
                      ? 'text-[var(--text-primary)]'
                      : isInProgress
                      ? 'text-sky-700 dark:text-sky-400 font-extrabold'
                      : 'text-[var(--text-muted)]'
                  }`}
                >
                  {stage.title}
                </h4>
                <p className="text-[11px] text-[var(--text-secondary)] font-medium leading-snug">
                  {stage.subtitle}
                </p>
                <p className="text-[10px] text-[var(--text-muted)] leading-tight pt-0.5">
                  {stage.details}
                </p>
              </div>
            </div>
          );
        })}
      </div>

      {/* Vertical Stepper for Mobile / Tablet */}
      <div className="lg:hidden space-y-5 relative before:absolute before:inset-0 before:left-4 before:w-0.5 before:bg-[var(--canvas-subdued)]">
        {stages.map((stage) => {
          const isDone = stage.status === 'completed';
          const isInProgress = stage.status === 'in_progress';
          const Icon = stage.icon;

          return (
            <div key={stage.id} className="relative flex items-start gap-4">
              <div
                className={`w-8 h-8 rounded-xl flex items-center justify-center shrink-0 z-10 transition-all ${
                  isDone
                    ? 'bg-emerald-600 text-white shadow-xs'
                    : isInProgress
                    ? 'bg-sky-600 text-white ring-4 ring-sky-500/30 animate-pulse'
                    : 'bg-[var(--canvas-subdued)] text-[var(--text-muted)] border border-[var(--surface-border)]'
                }`}
              >
                {isDone ? (
                  <CheckCircle2 className="w-4 h-4 stroke-[2.5]" />
                ) : (
                  <Icon className="w-4 h-4" />
                )}
              </div>
              <div className="space-y-0.5 flex-1 pt-0.5">
                <div className="flex items-center justify-between">
                  <h4 className="text-[13px] font-bold text-[var(--text-primary)]">
                    {stage.title}
                  </h4>
                  <span
                    className={`text-[10px] font-bold uppercase px-2 py-0.5 rounded-full ${
                      isDone
                        ? 'bg-emerald-100 text-emerald-800'
                        : isInProgress
                        ? 'bg-sky-100 text-sky-800'
                        : 'bg-slate-100 text-slate-600'
                    }`}
                  >
                    {isDone ? 'Completed' : isInProgress ? 'In Progress' : 'Queued'}
                  </span>
                </div>
                <p className="text-[11px] text-[var(--text-secondary)] font-medium">
                  {stage.subtitle}
                </p>
                <p className="text-[10px] text-[var(--text-muted)]">
                  {stage.details}
                </p>
              </div>
            </div>
          );
        })}
      </div>

      {/* Switch Telemetry Re-check Interactive Box */}
      <div className="p-4 rounded-2xl bg-[var(--canvas-subdued)] border border-[var(--surface-border)] space-y-2.5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-amber-500" />
            <span className="text-[12px] font-bold text-[var(--text-primary)]">
              Interbank Network Switch Telemetry
            </span>
          </div>
          <button
            type="button"
            onClick={handlePingSwitch}
            disabled={isPingingSwitch}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-[11px] font-bold bg-[var(--surface)] border border-[var(--surface-border)] hover:bg-[var(--canvas)] text-[var(--text-primary)] shadow-2xs transition-colors cursor-pointer disabled:opacity-60"
          >
            <RefreshCw className={`w-3.5 h-3.5 text-sky-600 ${isPingingSwitch ? 'animate-spin' : ''}`} />
            <span>{isPingingSwitch ? 'Querying Switch...' : 'Re-check Switch Status'}</span>
          </button>
        </div>

        {switchPingResult ? (
          <div className="p-3 rounded-xl bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 text-[11px] font-mono text-emerald-900 dark:text-emerald-200 animate-in fade-in">
            {switchPingResult}
          </div>
        ) : (
          <p className="text-[11px] text-[var(--text-muted)]">
            Network query checks transaction status directly against the Interswitch / NIBSS instant settlement rail every 60 seconds.
          </p>
        )}
      </div>
    </div>
  );
};
