import React, { useState, useEffect } from 'react';
import { Clock, AlertTriangle, AlertOctagon, CheckCircle2 } from 'lucide-react';
import clsx from 'clsx';
import type { SLAStatus } from '../../types/domain';

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
    }, 30000); // Update every 30s
    return () => clearInterval(timer);
  }, [targetDate]);

  const { isBreached, formatted, status } = timeLeft;

  const badgeColor = isBreached
    ? 'bg-rose-50 text-rose-800 border-rose-300'
    : status === 'APPROACHING'
    ? 'bg-amber-50 text-amber-900 border-amber-300'
    : 'bg-slate-50 text-slate-700 border-slate-300';

  const icon = isBreached ? (
    <AlertOctagon className="w-3 h-3 text-rose-600 shrink-0 animate-pulse" />
  ) : status === 'APPROACHING' ? (
    <AlertTriangle className="w-3 h-3 text-amber-600 shrink-0" />
  ) : type === 'regulatory' ? (
    <Clock className="w-3 h-3 text-slate-500 shrink-0" />
  ) : (
    <CheckCircle2 className="w-3 h-3 text-emerald-600 shrink-0" />
  );

  return (
    <div
      className={clsx(
        'inline-flex items-center gap-1.5 px-2 py-0.5 rounded border text-[11px] font-mono-financial tracking-tight font-medium',
        badgeColor,
        compact ? 'text-[10px] px-1.5 py-0' : 'text-[11px]'
      )}
      title={`${label || (type === 'regulatory' ? 'Regulatory Deadline' : 'Internal SLA Target')}: ${targetDate}`}
    >
      {icon}
      {label && <span className="text-[10px] uppercase font-sans font-bold opacity-75">{label}:</span>}
      <span className={clsx(isBreached && 'font-bold text-rose-700')}>
        {formatted}
      </span>
      {isBreached && (
        <span className="text-[9px] font-sans uppercase font-extrabold px-1 py-0.2 rounded bg-rose-200 text-rose-900">
          BREACHED
        </span>
      )}
      {!isBreached && status === 'APPROACHING' && (
        <span className="text-[9px] font-sans uppercase font-extrabold px-1 py-0.2 rounded bg-amber-200 text-amber-900">
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
