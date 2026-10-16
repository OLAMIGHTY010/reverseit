import React from 'react';
import {
  FileCheck2,
  Sparkles,
  HelpCircle,
  AlertTriangle,
  ShieldCheck,
  Send,
  AlertOctagon,
  Clock,
  RefreshCw,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';
import type { EvidenceItem } from '../../types/domain';

interface EvidenceDossierProps {
  evidence: EvidenceItem[];
  onRequestMissingCheck: (evidenceId: string, source: string) => void;
  targetEvidenceId?: string | null;
  isLoading?: boolean;
  hasError?: boolean;
  onRetry?: () => void;
}

export const EvidenceDossier: React.FC<EvidenceDossierProps> = ({
  evidence,
  onRequestMissingCheck,
  targetEvidenceId,
  isLoading = false,
  hasError = false,
  onRetry,
}) => {
  if (isLoading) {
    return (
      <div className="p-4 space-y-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md">
        <div className="h-6 w-48 bg-[var(--canvas-subdued)] rounded animate-pulse" />
        <div className="grid grid-cols-2 gap-3">
          <div className="h-28 bg-[var(--canvas-subdued)] rounded animate-pulse" />
          <div className="h-28 bg-[var(--canvas-subdued)] rounded animate-pulse" />
        </div>
      </div>
    );
  }

  if (hasError) {
    return (
      <div className="p-6 text-center space-y-3 bg-[var(--surface)] border border-rose-300 rounded-md text-rose-950">
        <AlertOctagon className="w-8 h-8 text-rose-600 mx-auto" />
        <h4 className="font-bold text-[14px]">Failed to Retrieve Evidence Dossier</h4>
        <p className="text-[12px] opacity-90 max-w-sm mx-auto">
          Central Switch and Core Banking ledger proof sources timed out.
        </p>
        {onRetry && (
          <Button size="sm" variant="secondary" onClick={onRetry} leftIcon={<RefreshCw className="w-3.5 h-3.5" />}>
            Retry Evidence Fetch
          </Button>
        )}
      </div>
    );
  }

  const facts = evidence.filter((e) => e.type === 'FACT');
  const inferred = evidence.filter((e) => e.type === 'INFERRED');
  const missing = evidence.filter((e) => e.type === 'MISSING');
  const unresolved = evidence.filter((e) => e.type === 'UNRESOLVED');

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
      {/* Header */}
      <div className="px-4 py-3 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-[14px] font-bold text-[var(--text-primary)]">
            4-Quadrant Evidence Dossier
          </h3>
          <p className="text-[12px] text-[var(--text-muted)]">
            Every item must be verified fresh before supporting an operational reversal or write-off.
          </p>
        </div>
        <div className="text-[11px] font-mono-financial px-2 py-0.5 rounded bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)]">
          Dual-Control Evidence Standard
        </div>
      </div>

      <div className="p-4 space-y-5 text-[13px]">
        {/* 1. CONFIRMED FACTS */}
        <section aria-labelledby="section-facts" className="space-y-2">
          <div className="flex items-center justify-between">
            <h4
              id="section-facts"
              className="flex items-center gap-2 font-bold text-emerald-950 text-[13px]"
            >
              <span className="p-1 rounded bg-emerald-100 text-emerald-800">
                <FileCheck2 className="w-4 h-4" />
              </span>
              <span>1. Confirmed Facts ({facts.length})</span>
            </h4>
            <span className="text-[11px] font-mono-financial text-emerald-800">
              Immutable Switch & Ledger Records
            </span>
          </div>

          <div className="space-y-2 pl-6">
            {facts.map((item) => {
              const isTargeted = targetEvidenceId === item.id;
              return (
                <div
                  key={item.id}
                  id={`evidence-${item.id}`}
                  className={clsx(
                    'p-3 rounded border text-[12px] transition-all space-y-1.5',
                    isTargeted
                      ? 'bg-emerald-100/70 border-emerald-500 ring-2 ring-emerald-400'
                      : 'bg-emerald-50/50 border-emerald-200 text-emerald-950'
                  )}
                >
                  <div className="flex items-center justify-between font-semibold">
                    <span className="text-emerald-900">{item.title}</span>
                    <div className="flex items-center gap-1.5">
                      <span className="inline-flex items-center gap-1 text-[11px] font-mono-financial px-1.5 py-0.2 rounded bg-emerald-100 text-emerald-800 border border-emerald-300">
                        <ShieldCheck className="w-3 h-3" /> Verified
                      </span>
                      {/* Freshness Badge */}
                      <span
                        className={clsx(
                          'text-[10px] font-mono-financial px-1.5 py-0.2 rounded border font-semibold',
                          item.isStale
                            ? 'bg-rose-100 text-rose-900 border-rose-300'
                            : 'bg-emerald-100 text-emerald-800 border-emerald-300'
                        )}
                      >
                        {item.freshnessLabel}
                      </span>
                    </div>
                  </div>
                  <p className="text-emerald-900 leading-relaxed">{item.details}</p>
                  <div className="text-[11px] text-emerald-700/90 font-mono-financial flex items-center justify-between pt-1 border-t border-emerald-200/60">
                    <span>Source: {item.source}</span>
                    {item.timestamp && <span>{new Date(item.timestamp).toLocaleTimeString()}</span>}
                  </div>
                </div>
              );
            })}
          </div>
        </section>

        {/* 2. INFERRED EXPLANATIONS */}
        <section aria-labelledby="section-inferred" className="space-y-2">
          <div className="flex items-center justify-between">
            <h4
              id="section-inferred"
              className="flex items-center gap-2 font-bold text-sky-950 text-[13px]"
            >
              <span className="p-1 rounded bg-sky-100 text-sky-800">
                <Sparkles className="w-4 h-4" />
              </span>
              <span>2. Inferred Explanations ({inferred.length})</span>
            </h4>
            <span className="text-[11px] font-mono-financial text-sky-800">
              AI Probabilistic Model Hypotheses
            </span>
          </div>

          <div className="space-y-2 pl-6">
            {inferred.map((item) => {
              const isTargeted = targetEvidenceId === item.id;
              return (
                <div
                  key={item.id}
                  id={`evidence-${item.id}`}
                  className={clsx(
                    'p-3 rounded border text-[12px] transition-all space-y-1.5',
                    isTargeted
                      ? 'bg-sky-100/70 border-sky-500 ring-2 ring-sky-400'
                      : 'bg-sky-50/50 border-sky-200 text-sky-950'
                  )}
                >
                  <div className="flex items-center justify-between font-semibold">
                    <span className="text-sky-900">{item.title}</span>
                    <div className="flex items-center gap-1.5">
                      {item.confidence && (
                        <span className="text-[11px] font-mono-financial font-bold px-1.5 py-0.2 rounded bg-sky-100 text-sky-900 border border-sky-300">
                          {item.confidence}% Confidence
                        </span>
                      )}
                      <span className="text-[10px] font-mono-financial px-1.5 py-0.2 rounded bg-sky-100 text-sky-800 border border-sky-300">
                        {item.freshnessLabel}
                      </span>
                    </div>
                  </div>
                  <p className="text-sky-900 leading-relaxed">{item.details}</p>
                  {item.inferredBasis && (
                    <div className="p-2 rounded bg-sky-100/50 text-[11px] text-sky-950 font-mono-financial border border-sky-200">
                      <strong>Basis:</strong> {item.inferredBasis}
                    </div>
                  )}
                  <div className="text-[11px] text-sky-700/90 font-mono-financial pt-1 border-t border-sky-200/60">
                    Engine: {item.source}
                  </div>
                </div>
              );
            })}
          </div>
        </section>

        {/* 3. MISSING EVIDENCE (Interactive Request Check) */}
        <section aria-labelledby="section-missing" className="space-y-2">
          <div className="flex items-center justify-between">
            <h4
              id="section-missing"
              className="flex items-center gap-2 font-bold text-amber-950 text-[13px]"
            >
              <span className="p-1 rounded bg-amber-100 text-amber-800">
                <HelpCircle className="w-4 h-4" />
              </span>
              <span>3. Missing Evidence ({missing.length})</span>
            </h4>
            <span className="text-[11px] font-mono-financial text-amber-800">
              Unreceived Counterparty & Settlement Legs
            </span>
          </div>

          <div className="space-y-2 pl-6">
            {missing.map((item) => {
              const isTargeted = targetEvidenceId === item.id;
              return (
                <div
                  key={item.id}
                  id={`evidence-${item.id}`}
                  className={clsx(
                    'p-3 rounded border text-[12px] transition-all space-y-2',
                    isTargeted
                      ? 'bg-amber-100/70 border-amber-500 ring-2 ring-amber-400'
                      : 'bg-amber-50/60 border-amber-200 text-amber-950'
                  )}
                >
                  <div className="flex items-center justify-between font-semibold">
                    <span className="text-amber-900">{item.title}</span>
                    <span className="text-[10px] font-mono-financial px-1.5 py-0.2 rounded bg-amber-100 text-amber-900 border border-amber-300">
                      {item.freshnessLabel}
                    </span>
                  </div>
                  <p className="text-amber-900 leading-relaxed">{item.details}</p>

                  <div className="pt-2 border-t border-amber-200/60 flex items-center justify-between">
                    <span className="text-[11px] text-amber-800 font-mono-financial">
                      Expected From: {item.source}
                    </span>
                    <Button
                      size="xs"
                      variant="primary"
                      onClick={() => onRequestMissingCheck(item.id, item.source)}
                      leftIcon={<Send className="w-3 h-3" />}
                    >
                      {item.missingActionLabel || 'Request Check from Source'}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        </section>

        {/* 4. UNRESOLVED QUESTIONS & STALE WARNINGS */}
        <section aria-labelledby="section-unresolved" className="space-y-2">
          <div className="flex items-center justify-between">
            <h4
              id="section-unresolved"
              className="flex items-center gap-2 font-bold text-rose-950 text-[13px]"
            >
              <span className="p-1 rounded bg-rose-100 text-rose-800">
                <AlertTriangle className="w-4 h-4" />
              </span>
              <span>4. Unresolved Questions & Suspense Balances ({unresolved.length})</span>
            </h4>
            <span className="text-[11px] font-mono-financial text-rose-800">
              Audit Gate & Quarantine Items
            </span>
          </div>

          <div className="space-y-2 pl-6">
            {unresolved.map((item) => {
              const isTargeted = targetEvidenceId === item.id;
              return (
                <div
                  key={item.id}
                  id={`evidence-${item.id}`}
                  className={clsx(
                    'p-3 rounded border text-[12px] transition-all space-y-2',
                    isTargeted
                      ? 'bg-rose-100/70 border-rose-500 ring-2 ring-rose-400'
                      : 'bg-rose-50/60 border-rose-200 text-rose-950'
                  )}
                >
                  <div className="flex items-center justify-between font-semibold">
                    <span className="text-rose-900">{item.title}</span>
                    {/* Stale Warning Flag */}
                    {item.isStale ? (
                      <span
                        className="inline-flex items-center gap-1 text-[10px] font-sans font-extrabold px-1.5 py-0.5 rounded bg-rose-200 text-rose-900 border border-rose-400"
                        title="Stale evidence cannot support an operational reversal action until refreshed."
                      >
                        <Clock className="w-3 h-3" /> STALE: ACTION BLOCKED
                      </span>
                    ) : (
                      <span className="text-[10px] font-mono-financial px-1.5 py-0.2 rounded bg-rose-100 text-rose-800 border border-rose-300">
                        {item.freshnessLabel}
                      </span>
                    )}
                  </div>
                  <p className="text-rose-900 leading-relaxed">{item.details}</p>

                  <div className="text-[11px] text-rose-700 font-mono-financial pt-1 border-t border-rose-200/60 flex items-center justify-between">
                    <span>Target GL: {item.source}</span>
                    <span>Action Allowed: {item.canSupportAction ? 'YES' : 'NO (REFRESH REQUIRED)'}</span>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      </div>
    </div>
  );
};
