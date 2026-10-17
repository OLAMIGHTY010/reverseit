import React, { useState, useEffect } from 'react';
import { Clock, AlertTriangle, AlertOctagon, CheckCircle2, ShieldAlert } from 'lucide-react';
import clsx from 'clsx';
import type { SLAStatus } from '../../types/domain';
import { calculateStatutoryFineExposure } from '../fraud/DisputeAbuseEngine';

interface CountdownTimerProps {
  targetDate: string; // ISO format
  label?: string;
  type?: 'internal' | 'regulatory';
  compact?: boolean;
}

export const CountdownTimer: React.FC<CountdownTimerProps> = ({
  targetDate,
  label,
  type = 'internal',
  compact = false,
}) => {
  const [timeLeft, setTimeLeft] = useState(() => calculateDelta(targetDate));

  useEffect(() => {
    const timer = setInterval(() => {
      setTimeLeft(calculateDelta(targetDate));
    }, 15000); // Update every 15s
    return () => clearInterval(timer);
  }, [targetDate]);

  const { isBreached, formatted, status, minutesRemaining } = timeLeft;
  const isUrgentSla = !isBreached && minutesRemaining <= 240; // Under 4 hours
  const fineInfo = type === 'regulatory' ? calculateStatutoryFineExposure(targetDate) : null;

  const badgeColor = isBreached
    ? 'bg-rose-50 text-rose-800 border-rose-300 dark:bg-rose-950/40 dark:text-rose-300 dark:border-rose-800'
    : isUrgentSla
    ? 'bg-amber-100 text-amber-950 border-amber-400 dark:bg-amber-950/60 dark:text-amber-200 dark:border-amber-700 animate-pulse'
    : status === 'APPROACHING'
    ? 'bg-amber-50 text-amber-900 border-amber-300 dark:bg-amber-950/40 dark:text-amber-300 dark:border-amber-800'
    : 'bg-slate-50 text-slate-700 border-slate-300 dark:bg-slate-900 dark:text-slate-300 dark:border-slate-700';

  const icon = isBreached ? (
    <AlertOctagon className="w-3.5 h-3.5 text-rose-600 shrink-0 animate-pulse" />
  ) : isUrgentSla ? (
    <ShieldAlert className="w-3.5 h-3.5 text-amber-700 shrink-0" />
  ) : status === 'APPROACHING' ? (
    <AlertTriangle className="w-3.5 h-3.5 text-amber-600 shrink-0" />
  ) : type === 'regulatory' ? (
    <Clock className="w-3.5 h-3.5 text-sky-600 shrink-0" />
  ) : (
    <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600 shrink-0" />
  );

  return (
    <div
      className={clsx(
        'inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-[11px] font-mono-financial tracking-tight font-medium',
        badgeColor,
        compact ? 'text-[10px] px-1.5 py-0' : 'text-[11px]'
      )}
      title={`${label || (type === 'regulatory' ? 'CBN / Central Bank Statutory Deadline' : 'Internal Bank Target SLA')}: ${targetDate}${
        fineInfo?.isBreached ? ` • Accrued Statutory Fine: ₦${fineInfo.accruedFineNgn.toLocaleString()} (${fineInfo.daysOverdue} days overdue)` : ''
      }`}
    >
      {icon}
      {label && <span className="text-[10px] uppercase font-sans font-bold opacity-75">{label}:</span>}
      <span className={clsx(isBreached && 'font-bold text-rose-700 dark:text-rose-400')}>
        {formatted}
      </span>
      {isBreached && (
        <span className="text-[9px] font-sans uppercase font-extrabold px-1 py-0.2 rounded bg-rose-200 text-rose-900 dark:bg-rose-900 dark:text-rose-100">
          {type === 'regulatory' && fineInfo ? `BREACH • +₦${(fineInfo.accruedFineNgn / 1000).toFixed(0)}k FINE` : 'BREACHED'}
        </span>
      )}
      {!isBreached && isUrgentSla && (
        <span className="text-[9px] font-sans uppercase font-extrabold px-1 py-0.2 rounded bg-amber-300 text-amber-950 dark:bg-amber-800 dark:text-amber-100">
          ESCALATE (&lt;4H)
        </span>
      )}
      {!isBreached && !isUrgentSla && status === 'APPROACHING' && (
        <span className="text-[9px] font-sans uppercase font-extrabold px-1 py-0.2 rounded bg-amber-200 text-amber-900 dark:bg-amber-900 dark:text-amber-200">
          DUE SOON
        </span>
      )}
    </div>
  );
};

function calculateDelta(targetIso: string): {
  isBreached: boolean;
  minutesRemaining: number;
  formatted: string;
  status: SLAStatus;
} {
  const target = new Date(targetIso).getTime();
  const now = Date.now();
  const diffMs = target - now;
  const isBreached = diffMs <= 0;
  const absDiff = Math.abs(diffMs);

  const hours = Math.floor(absDiff / (1000 * 60 * 60));
  const minutes = Math.floor((absDiff % (1000 * 60 * 60)) / (1000 * 60));

  let formatted: string;
  if (hours > 24) {
    const days = Math.floor(hours / 24);
    formatted = `${days}d ${hours % 24}h`;
  } else if (hours > 0) {
    formatted = `${hours}h ${minutes}m`;
  } else {
    formatted = `${minutes}m`;
  }

  if (isBreached) {
    formatted = `-${formatted}`;
  }

  let status: SLAStatus = 'ON_TRACK';
  if (isBreached) {
    status = 'BREACHED';
  } else if (diffMs <= 60 * 60 * 1000) {
    // Under 1 hour
    status = 'APPROACHING';
  }

  return {
    isBreached,
    minutesRemaining: Math.round(diffMs / 60000),
    formatted,
    status,
  };
}
