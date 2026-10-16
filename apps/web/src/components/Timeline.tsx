import React from 'react';
import {
  GitCommit,
  CheckCircle,
  AlertCircle,
  Clock,
  Shield,
} from 'lucide-react';
import clsx from 'clsx';
import type { AuditEvent } from '../types/domain';

export interface TimelineProps {
  events: AuditEvent[];
  className?: string;
}

export const Timeline: React.FC<TimelineProps> = ({ events, className }) => {
  return (
    <div className={clsx('relative pl-6 space-y-6 before:absolute before:left-2.5 before:top-2 before:bottom-2 before:w-0.5 before:bg-[var(--surface-border)]', className)}>
      {events.map((event) => {

        return (
          <div key={event.id} className="relative group text-[13px]">
            {/* Timeline Node Icon */}
            <span
              className={clsx(
                'absolute -left-6 top-0.5 w-5 h-5 rounded-full flex items-center justify-center ring-4 ring-[var(--surface)] text-white text-[10px]',
                event.action.includes('RESOLVED') || event.action.includes('AUTHORIZED')
                  ? 'bg-emerald-600'
                  : event.action.includes('PROPOSED')
                  ? 'bg-amber-600'
                  : event.action.includes('EXCEPTION')
                  ? 'bg-rose-600'
                  : 'bg-[var(--primary)]'
              )}
            >
              {event.action.includes('AUTHORIZED') ? (
                <CheckCircle className="w-3 h-3" />
              ) : event.action.includes('EXCEPTION') ? (
                <AlertCircle className="w-3 h-3" />
              ) : (
                <GitCommit className="w-3 h-3" />
              )}
            </span>

            {/* Event Content Card */}
            <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-md p-3 shadow-xs space-y-1.5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-semibold text-[var(--text-primary)]">
                  {event.action.replace(/_/g, ' ')}
                </span>
                <span className="text-[11px] font-mono-financial text-[var(--text-muted)] flex items-center gap-1">
                  <Clock className="w-3 h-3" />
                  {new Date(event.timestamp).toLocaleString()}
                </span>
              </div>

              <div className="flex flex-wrap items-center gap-3 text-[12px] text-[var(--text-secondary)]">
                <div>
                  Actor: <strong className="font-mono-financial text-[var(--text-primary)]">{event.actorId}</strong>
                </div>
                <div className="px-1.5 py-0.2 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[11px] font-mono-financial">
                  Role: {event.actorRole}
                </div>
                <div>
                  Target: <span className="font-mono-financial">{event.entityType} ({event.entityId})</span>
                </div>
              </div>

              {/* Cryptographic tamper-evident hash footer */}
              <div className="pt-2 mt-2 border-t border-[var(--surface-border-subtle)] flex flex-wrap items-center justify-between text-[11px] text-[var(--text-muted)] font-mono-financial">
                <div className="flex items-center gap-1">
                  <Shield className="w-3 h-3 text-slate-400" />
                  <span>Hash: {event.entryHash.substring(0, 16)}...</span>
                </div>
                <div>Prev: {event.prevHash.substring(0, 10)}...</div>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
};
