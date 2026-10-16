import React from 'react';
import {
  Eye,
  Clock,
  Search,
  ShieldAlert,
  Send,
  RefreshCw,
  CheckCircle2,
  AlertOctagon,
  ShieldOff,
} from 'lucide-react';
import clsx from 'clsx';
import type { FinancialStatus } from '../types/domain';

export interface StatusBadgeProps {
  status: FinancialStatus;
  label?: string;
  className?: string;
  size?: 'sm' | 'md';
}

const statusIcons: Record<FinancialStatus, React.ComponentType<{ className?: string }>> = {
  detected: Eye,
  awaiting: Clock,
  investigating: Search,
  approval: ShieldAlert,
  submitted: Send,
  verifying: RefreshCw,
  resolved: CheckCircle2,
  failed: AlertOctagon,
  suppressed: ShieldOff,
};

const statusLabels: Record<FinancialStatus, string> = {
  detected: 'Detected',
  awaiting: 'Awaiting Action',
  investigating: 'Investigating',
  approval: 'Pending Approval',
  submitted: 'Submitted',
  verifying: 'Verifying',
  resolved: 'Resolved',
  failed: 'Failed',
  suppressed: 'Suppressed',
};

const statusStyles: Record<FinancialStatus, string> = {
  detected:
    'bg-[var(--status-detected-bg)] border-[var(--status-detected-border)] text-[var(--status-detected-text)]',
  awaiting:
    'bg-[var(--status-awaiting-bg)] border-[var(--status-awaiting-border)] text-[var(--status-awaiting-text)]',
  investigating:
    'bg-[var(--status-investigating-bg)] border-[var(--status-investigating-border)] text-[var(--status-investigating-text)]',
  approval:
    'bg-[var(--status-approval-bg)] border-[var(--status-approval-border)] text-[var(--status-approval-text)]',
  submitted:
    'bg-[var(--status-submitted-bg)] border-[var(--status-submitted-border)] text-[var(--status-submitted-text)]',
  verifying:
    'bg-[var(--status-verifying-bg)] border-[var(--status-verifying-border)] text-[var(--status-verifying-text)]',
  resolved:
    'bg-[var(--status-resolved-bg)] border-[var(--status-resolved-border)] text-[var(--status-resolved-text)]',
  failed:
    'bg-[var(--status-failed-bg)] border-[var(--status-failed-border)] text-[var(--status-failed-text)]',
  suppressed:
    'bg-[var(--status-suppressed-bg)] border-[var(--status-suppressed-border)] text-[var(--status-suppressed-text)]',
};

export const StatusBadge: React.FC<StatusBadgeProps> = ({
  status,
  label,
  className,
  size = 'md',
}) => {
  const Icon = statusIcons[status] || Eye;
  const displayLabel = label || statusLabels[status] || status;

  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1.5 font-medium border rounded select-none font-mono-financial tracking-tight',
        size === 'sm' ? 'px-2 py-0.5 text-[11px]' : 'px-2.5 py-1 text-[12px]',
        statusStyles[status] || statusStyles.detected,
        className
      )}
      role="status"
      aria-label={`Status: ${displayLabel}`}
    >
      <Icon
        className={clsx(
          size === 'sm' ? 'w-3 h-3' : 'w-3.5 h-3.5',
          status === 'verifying' && 'animate-spin'
        )}
        aria-hidden="true"
      />
      <span>{displayLabel}</span>
    </span>
  );
};
