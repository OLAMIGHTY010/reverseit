import React from 'react';
import {
  FileCheck2,
  Sparkles,
  HelpCircle,
  AlertTriangle,
  ShieldCheck,
} from 'lucide-react';
import clsx from 'clsx';
import type { EvidenceItem } from '../types/domain';

export interface EvidencePanelProps {
  evidence: EvidenceItem[];
  caseNumber?: string;
  reference?: string;
  className?: string;
}

export const EvidencePanel: React.FC<EvidencePanelProps> = ({
  evidence,
  caseNumber,
  reference,
  className,
}) => {
  const facts = evidence.filter((e) => e.type === 'FACT');
  const inferred = evidence.filter((e) => e.type === 'INFERRED');
  const missing = evidence.filter((e) => e.type === 'MISSING');
  const unresolved = evidence.filter((e) => e.type === 'UNRESOLVED');

  return (
    <div
      className={clsx(
        'border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]',
        className
      )}
    >
      {/* Header */}
      <div className="px-4 py-3 bg-[var(--canvas-subdued)] flex items-center justify-between">
        <div>
          <h3 className="text-[14px] font-semibold text-[var(--text-primary)]">
            Investigation Evidence Dossier
          </h3>
          <p className="text-[12px] text-[var(--text-muted)]">
            {caseNumber && <span className="font-mono-financial font-medium text-[var(--text-secondary)]">{caseNumber}</span>}
            {reference && <span> • Ref: <span className="font-mono-financial">{reference}</span></span>}
          </p>
        </div>
        <div className="text-[11px] font-mono-financial px-2 py-0.5 rounded bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)]">
          Dual-Control Evidence Standard
        </div>
      </div>

      {/* Grid of 4 Quadrants / Sections */}
      <div className="p-4 space-y-4">
        {/* 1. FACT SECTION */}
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-[13px] font-semibold text-emerald-950">
            <span className="p-1 rounded bg-emerald-100 text-emerald-800">
              <FileCheck2 className="w-4 h-4" aria-hidden="true" />
            </span>
            <span>1. Fact: Immutable Records & Switch Logs ({facts.length})</span>
          </div>
          <div className="space-y-1.5 pl-6">
            {facts.length === 0 ? (
              <p className="text-[12px] text-[var(--text-muted)] italic">No verified ledger facts attached.</p>
            ) : (
              facts.map((item) => (
                <div
                  key={item.id}
                  className="p-2.5 text-[12px] bg-emerald-50/50 border border-emerald-200 rounded text-emerald-950 space-y-1"
                >
                  <div className="flex items-center justify-between font-medium">
                    <span className="font-semibold text-emerald-900">{item.title}</span>
                    <span className="inline-flex items-center gap-1 text-[11px] text-emerald-700 font-mono-financial">
                      <ShieldCheck className="w-3 h-3" /> Verified Source
                    </span>
                  </div>
                  <p className="text-emerald-800 leading-relaxed">{item.details}</p>
                  <div className="text-[11px] text-emerald-700/80 font-mono-financial">
                    Source: {item.source} {item.timestamp && `• ${new Date(item.timestamp).toLocaleTimeString()}`}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        {/* 2. INFERRED SECTION */}
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-[13px] font-semibold text-sky-950">
            <span className="p-1 rounded bg-sky-100 text-sky-800">
              <Sparkles className="w-4 h-4" aria-hidden="true" />
            </span>
            <span>2. Inferred: AI Model Hypotheses ({inferred.length})</span>
          </div>
          <div className="space-y-1.5 pl-6">
            {inferred.length === 0 ? (
              <p className="text-[12px] text-[var(--text-muted)] italic">No model inferences recorded.</p>
            ) : (
              inferred.map((item) => (
                <div
                  key={item.id}
                  className="p-2.5 text-[12px] bg-sky-50/50 border border-sky-200 rounded text-sky-950 space-y-1"
                >
                  <div className="flex items-center justify-between font-medium">
                    <span className="font-semibold text-sky-900">{item.title}</span>
                    <span className="text-[11px] text-sky-700 bg-sky-100 px-1.5 py-0.2 rounded font-mono-financial">
                      Probabilistic Inference
                    </span>
                  </div>
                  <p className="text-sky-800 leading-relaxed">{item.details}</p>
                  <div className="text-[11px] text-sky-700/80 font-mono-financial">
                    Engine: {item.source}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        {/* 3. MISSING SECTION */}
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-[13px] font-semibold text-amber-950">
            <span className="p-1 rounded bg-amber-100 text-amber-800">
              <HelpCircle className="w-4 h-4" aria-hidden="true" />
            </span>
            <span>3. Missing: Unreceived Settlement Legs ({missing.length})</span>
          </div>
          <div className="space-y-1.5 pl-6">
            {missing.length === 0 ? (
              <p className="text-[12px] text-[var(--text-muted)] italic">No missing counterparty legs identified.</p>
            ) : (
              missing.map((item) => (
                <div
                  key={item.id}
                  className="p-2.5 text-[12px] bg-amber-50/60 border border-amber-200 rounded text-amber-950 space-y-1"
                >
                  <div className="font-semibold text-amber-900">{item.title}</div>
                  <p className="text-amber-800 leading-relaxed">{item.details}</p>
                  <div className="text-[11px] text-amber-700/80 font-mono-financial">
                    Expected from: {item.source}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        {/* 4. UNRESOLVED SECTION */}
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-[13px] font-semibold text-rose-950">
            <span className="p-1 rounded bg-rose-100 text-rose-800">
              <AlertTriangle className="w-4 h-4" aria-hidden="true" />
            </span>
            <span>4. Unresolved: Suspense & Action Items ({unresolved.length})</span>
          </div>
          <div className="space-y-1.5 pl-6">
            {unresolved.length === 0 ? (
              <p className="text-[12px] text-[var(--text-muted)] italic">No unresolved suspense balances.</p>
            ) : (
              unresolved.map((item) => (
                <div
                  key={item.id}
                  className="p-2.5 text-[12px] bg-rose-50/60 border border-rose-200 rounded text-rose-950 space-y-1"
                >
                  <div className="font-semibold text-rose-900">{item.title}</div>
                  <p className="text-rose-800 leading-relaxed">{item.details}</p>
                  <div className="text-[11px] text-rose-700/80 font-mono-financial">
                    Target GL: {item.source}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
