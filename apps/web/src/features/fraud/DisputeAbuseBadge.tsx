import React from 'react';
import { ShieldCheck, AlertTriangle, ShieldAlert } from 'lucide-react';
import clsx from 'clsx';
import type { DisputeAbuseProfile, DisputeAbuseRiskTier } from '../../types/domain';

interface DisputeAbuseBadgeProps {
  profile: DisputeAbuseProfile;
  onClick?: () => void;
  compact?: boolean;
}

export const DisputeAbuseBadge: React.FC<DisputeAbuseBadgeProps> = ({
  profile,
  onClick,
  compact = false,
}) => {
  const { score, tier } = profile;

  const tierConfig: Record<
    DisputeAbuseRiskTier,
    {
      bg: string;
      text: string;
      border: string;
      icon: React.ReactNode;
      label: string;
    }
  > = {
    LOW_RISK_TRUSTED: {
      bg: 'bg-emerald-50 dark:bg-emerald-950/40',
      text: 'text-emerald-800 dark:text-emerald-300',
      border: 'border-emerald-300 dark:border-emerald-700',
      icon: <ShieldCheck className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />,
      label: 'Trusted Account',
    },
    STANDARD: {
      bg: 'bg-slate-50 dark:bg-slate-900',
      text: 'text-slate-700 dark:text-slate-300',
      border: 'border-slate-300 dark:border-slate-700',
      icon: <ShieldCheck className="w-3.5 h-3.5 text-slate-500" />,
      label: 'Standard',
    },
    ELEVATED_WATCH: {
      bg: 'bg-amber-50 dark:bg-amber-950/40',
      text: 'text-amber-900 dark:text-amber-300',
      border: 'border-amber-300 dark:border-amber-700',
      icon: <AlertTriangle className="w-3.5 h-3.5 text-amber-600 animate-pulse" />,
      label: 'Elevated Watch',
    },
    HIGH_RISK_RESTRICTED: {
      bg: 'bg-rose-50 dark:bg-rose-950/40',
      text: 'text-rose-900 dark:text-rose-300',
      border: 'border-rose-300 dark:border-rose-700',
      icon: <ShieldAlert className="w-3.5 h-3.5 text-rose-600 animate-pulse" />,
      label: 'Restricted / Abuse Alert',
    },
  };

  const current = tierConfig[tier] || tierConfig.STANDARD;

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!onClick}
      title={`Customer Dispute Abuse Score: ${score}/100. Tier: ${current.label}. Click to inspect fraud breakdown.`}
      className={clsx(
        'inline-flex items-center gap-1.5 rounded border font-mono-financial transition-all duration-150',
        current.bg,
        current.text,
        current.border,
        compact ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-0.5 text-[11px]',
        onClick ? 'cursor-pointer hover:shadow-xs hover:brightness-95' : 'cursor-default'
      )}
    >
      {current.icon}
      <span className="font-sans font-bold uppercase tracking-wider text-[10px]">
        Abuse Risk:
      </span>
      <span className="font-bold">{score}/100</span>
      {!compact && (
        <span
          className={clsx(
            'ml-0.5 px-1 py-0.2 rounded text-[9px] uppercase font-sans font-extrabold',
            tier === 'HIGH_RISK_RESTRICTED'
              ? 'bg-rose-200 text-rose-900 dark:bg-rose-900/60 dark:text-rose-200'
              : tier === 'ELEVATED_WATCH'
              ? 'bg-amber-200 text-amber-900 dark:bg-amber-900/60 dark:text-amber-200'
              : 'bg-slate-200/60 text-slate-700 dark:bg-slate-800 dark:text-slate-300'
          )}
        >
          {current.label}
        </span>
      )}
    </button>
  );
};
