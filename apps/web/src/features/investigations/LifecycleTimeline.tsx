import React, { useState } from 'react';
import {
  Clock,
  AlertTriangle,
  Server,
  Cpu,
  User,
  Shield,
  Layers,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Copy,
  Check,
} from 'lucide-react';
import clsx from 'clsx';
import type { TimelineEvent, TimelineSource } from '../../types/domain';

interface LifecycleTimelineProps {
  events: TimelineEvent[];
  onSelectEvidence?: (evidenceId: string) => void;
}

const SOURCE_CONFIG: Record<
  TimelineSource,
  { label: string; bg: string; text: string; border: string; icon: React.ReactNode }
> = {
  CBS: {
    label: 'Core Ledger (CBS)',
    bg: 'bg-emerald-50',
    text: 'text-emerald-900',
    border: 'border-emerald-300',
    icon: <Server className="w-3.5 h-3.5 text-emerald-700" />,
  },
  SWITCH: {
    label: 'Switch Network',
    bg: 'bg-sky-50',
    text: 'text-sky-900',
    border: 'border-sky-300',
    icon: <Layers className="w-3.5 h-3.5 text-sky-700" />,
  },
  AI_ENGINE: {
    label: 'AI Service',
    bg: 'bg-purple-50',
    text: 'text-purple-900',
    border: 'border-purple-300',
    icon: <Cpu className="w-3.5 h-3.5 text-purple-700" />,
  },
  CUSTOMER: {
    label: 'Customer / ALAT',
    bg: 'bg-indigo-50',
    text: 'text-indigo-900',
    border: 'border-indigo-300',
    icon: <User className="w-3.5 h-3.5 text-indigo-700" />,
  },
  OPS: {
    label: 'Operations & Audit',
    bg: 'bg-slate-100',
    text: 'text-slate-900',
    border: 'border-slate-300',
    icon: <Shield className="w-3.5 h-3.5 text-slate-700" />,
  },
  NETWORK: {
    label: 'Network Socket',
    bg: 'bg-amber-50',
    text: 'text-amber-900',
    border: 'border-amber-300',
    icon: <AlertTriangle className="w-3.5 h-3.5 text-amber-700" />,
  },
};

export const LifecycleTimeline: React.FC<LifecycleTimelineProps> = ({
  events,
  onSelectEvidence,
}) => {
  const [selectedSourceFilter, setSelectedSourceFilter] = useState<TimelineSource | 'ALL'>('ALL');
  const [expandedPayloadIds, setExpandedPayloadIds] = useState<string[]>([]);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const filteredEvents = events.filter((ev) => {
    if (selectedSourceFilter === 'ALL') return true;
    return ev.source === selectedSourceFilter;
  });

  const togglePayload = (id: string) => {
    setExpandedPayloadIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    );
  };

  const handleCopy = (id: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  return (
    <div className="space-y-4">
      {/* Timeline Controls & Source Filters */}
      <div className="flex flex-wrap items-center justify-between gap-2 p-2.5 rounded-md bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] text-[12px]">
        <div className="flex items-center gap-1.5 font-semibold text-[var(--text-secondary)]">
          <Clock className="w-4 h-4 text-sky-600" />
          <span>Timeline Events ({filteredEvents.length})</span>
        </div>

        {/* Source Pills */}
        <div className="flex flex-wrap items-center gap-1">
          <button
            type="button"
            onClick={() => setSelectedSourceFilter('ALL')}
            className={clsx(
              'px-2 py-0.5 rounded font-mono-financial text-[11px] border transition-colors',
              selectedSourceFilter === 'ALL'
                ? 'bg-sky-600 text-white border-sky-600 font-bold'
                : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border)] hover:bg-[var(--surface-raised)]'
            )}
          >
            All
          </button>
          {(['CBS', 'SWITCH', 'AI_ENGINE', 'CUSTOMER', 'OPS'] as TimelineSource[]).map((src) => (
            <button
              key={src}
              type="button"
              onClick={() => setSelectedSourceFilter(src)}
              className={clsx(
                'px-2 py-0.5 rounded font-mono-financial text-[11px] border transition-colors flex items-center gap-1',
                selectedSourceFilter === src
                  ? 'bg-sky-600 text-white border-sky-600 font-bold'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--surface-border)] hover:bg-[var(--surface-raised)]'
              )}
            >
              {src.replace('_', ' ')}
            </button>
          ))}
        </div>
      </div>

      {/* Semantic Ordered List Timeline */}
      <ol
        role="list"
        aria-label="Transaction Lifecycle Chronological Event List"
        className="relative pl-6 space-y-6 before:absolute before:left-2.5 before:top-2 before:bottom-2 before:w-0.5 before:bg-[var(--surface-border)]"
      >
        {filteredEvents.map((ev, index) => {
          const cfg = SOURCE_CONFIG[ev.source] || SOURCE_CONFIG.CBS;
          const isPayloadExpanded = expandedPayloadIds.includes(ev.id);
          const payloadJson = JSON.stringify(ev.payload, null, 2);

          return (
            <li
              key={ev.id}
              tabIndex={0}
              aria-label={`Event ${index + 1}: ${ev.title} from ${cfg.label}`}
              className="relative group text-[13px] focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-500 rounded"
            >
              {/* Gap Marker */}
              {ev.gapMessage && (
                <div className="mb-4 -ml-4 p-2 rounded bg-amber-50 border border-amber-300 text-amber-950 text-[11px] font-mono-financial font-semibold flex items-center gap-1.5 shadow-xs">
                  <AlertTriangle className="w-3.5 h-3.5 text-amber-700 shrink-0" />
                  <span>{ev.gapMessage}</span>
                </div>
              )}

              {/* Timeline Node Icon */}
              <span
                className={clsx(
                  'absolute -left-6 top-1 w-5 h-5 rounded-full flex items-center justify-center ring-4 ring-[var(--surface)] shadow-xs',
                  ev.severity === 'ERROR'
                    ? 'bg-rose-600 text-white'
                    : ev.severity === 'WARNING'
                    ? 'bg-amber-600 text-white'
                    : ev.severity === 'SUCCESS'
                    ? 'bg-emerald-600 text-white'
                    : 'bg-sky-600 text-white'
                )}
                aria-hidden="true"
              >
                {cfg.icon}
              </span>

              {/* Event Card */}
              <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-md p-3.5 shadow-xs space-y-2 hover:border-[var(--text-muted)] transition-colors">
                {/* Header: Source, Clock Skew, Late Arrival */}
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <span
                      className={clsx(
                        'inline-flex items-center gap-1 px-2 py-0.5 rounded border text-[11px] font-bold font-mono-financial',
                        cfg.bg,
                        cfg.text,
                        cfg.border
                      )}
                    >
                      {cfg.icon}
                      <span>{cfg.label}</span>
                    </span>

                    {/* Clock Skew Badge */}
                    {Math.abs(ev.clockSkewMs) > 1000 && (
                      <span
                        className="px-1.5 py-0.2 rounded font-mono-financial text-[10px] font-bold bg-amber-100 text-amber-900 border border-amber-300"
                        title={`Timestamp divergence between source and receiver: ${ev.clockSkewMs}ms`}
                      >
                        Skew: +{(ev.clockSkewMs / 1000).toFixed(1)}s
                      </span>
                    )}

                    {/* Late-arriving Event Marker */}
                    {ev.isLateArriving && (
                      <span
                        className="px-1.5 py-0.2 rounded font-sans text-[10px] font-extrabold bg-rose-100 text-rose-900 border border-rose-300 animate-pulse"
                        title={ev.lateArrivalReason || 'Late-arriving packet'}
                      >
                        LATE EVENT
                      </span>
                    )}
                  </div>

                  {/* Dual Timestamps: Source vs Received */}
                  <div className="text-[11px] font-mono-financial text-[var(--text-muted)] flex items-center gap-2">
                    <span title={`Source timestamp: ${ev.sourceTimestamp}`}>
                      Src: {new Date(ev.sourceTimestamp).toLocaleTimeString()}
                    </span>
                    <span title={`Host ingestion timestamp: ${ev.receivedTimestamp}`}>
                      Ingested: {new Date(ev.receivedTimestamp).toLocaleTimeString()}
                    </span>
                  </div>
                </div>

                {/* Title & Description */}
                <div>
                  <h4 className="font-semibold text-[13px] text-[var(--text-primary)]">
                    {ev.title}
                  </h4>
                  <p className="text-[12px] text-[var(--text-secondary)] leading-relaxed mt-0.5">
                    {ev.description}
                  </p>
                </div>

                {/* Late Arrival Reason Banner */}
                {ev.lateArrivalReason && (
                  <div className="p-2 rounded bg-rose-50/70 border border-rose-200 text-rose-900 text-[11px]">
                    {ev.lateArrivalReason}
                  </div>
                )}

                {/* Footer Controls: Jump to Evidence & Raw Payload toggle */}
                <div className="pt-2 border-t border-[var(--surface-border-subtle)] flex items-center justify-between text-[11px]">
                  {ev.linkedEvidenceId && onSelectEvidence ? (
                    <button
                      type="button"
                      onClick={() => onSelectEvidence(ev.linkedEvidenceId!)}
                      className="text-sky-600 hover:text-sky-800 font-semibold flex items-center gap-1 transition-colors"
                    >
                      <ExternalLink className="w-3 h-3" />
                      <span>Jump to Evidence Dossier ({ev.linkedEvidenceId})</span>
                    </button>
                  ) : (
                    <span className="text-[var(--text-muted)] font-mono-financial">
                      Event ID: {ev.id}
                    </span>
                  )}

                  <button
                    type="button"
                    onClick={() => togglePayload(ev.id)}
                    className="text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-mono-financial flex items-center gap-1"
                  >
                    {isPayloadExpanded ? (
                      <>
                        <ChevronDown className="w-3.5 h-3.5" />
                        <span>Hide Raw Payload</span>
                      </>
                    ) : (
                      <>
                        <ChevronRight className="w-3.5 h-3.5" />
                        <span>Inspect Raw Payload</span>
                      </>
                    )}
                  </button>
                </div>

                {/* Expandable Masked Raw Payload */}
                {isPayloadExpanded && (
                  <div className="mt-2 p-2.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[11px] font-mono-financial text-[var(--text-primary)] relative">
                    <div className="flex items-center justify-between pb-1.5 mb-1.5 border-b border-[var(--surface-border-subtle)] text-[10px] text-[var(--text-muted)]">
                      <span>RAW PAYLOAD (MASKED / ISO 8583 / JSON)</span>
                      <button
                        type="button"
                        onClick={() => handleCopy(ev.id, payloadJson)}
                        className="text-sky-600 hover:text-sky-800 flex items-center gap-1"
                      >
                        {copiedId === ev.id ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-600" />
                            <span>Copied</span>
                          </>
                        ) : (
                          <>
                            <Copy className="w-3 h-3" />
                            <span>Copy JSON</span>
                          </>
                        )}
                      </button>
                    </div>
                    <pre className="overflow-x-auto whitespace-pre leading-tight max-h-48 text-[11px]">
                      {payloadJson}
                    </pre>
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ol>
    </div>
  );
};
