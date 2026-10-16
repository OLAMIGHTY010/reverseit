import React, { useState } from 'react';
import {
  Layers,
  TrendingUp,
  Send,
  Zap,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { ShadowPredictionComparison } from './modelsTypes';

interface ShadowComparisonTabProps {
  predictions: ShadowPredictionComparison[];
  onPromoteModel: (modelName: string, justification: string) => void;
}

export const ShadowComparisonTab: React.FC<ShadowComparisonTabProps> = ({
  predictions,
  onPromoteModel,
}) => {
  const [filterDivergentOnly, setFilterDivergentOnly] = useState(false);
  const [isPromoteModalOpen, setIsPromoteModalOpen] = useState(false);
  const [justification, setJustification] = useState('');

  const filtered = filterDivergentOnly
    ? predictions.filter((p) => !p.isConsensus)
    : predictions;

  const consensusCount = predictions.filter((p) => p.isConsensus).length;
  const consensusPct = ((consensusCount / predictions.length) * 100).toFixed(1);

  const handlePromoteSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!justification.trim()) return;
    onPromoteModel('XGBoost Anomaly Reversal Predictor v2.1.0', justification);
    setIsPromoteModalOpen(false);
    setJustification('');
  };

  return (
    <div className="space-y-6">
      {/* Top Banner & Promotion Gate */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs">
        <div>
          <div className="flex items-center gap-2">
            <Layers className="w-5 h-5 text-indigo-600" />
            <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
              Shadow Mode Decision Matrix & Divergence Explorer
            </h3>
            <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-indigo-100 text-indigo-800 font-mono-financial">
              Tri-Model Comparison Matrix
            </span>
          </div>
          <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
            Compares production 8-check deterministic gate verdicts against shadow ML candidates in
            real-time without money movement risk.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <label className="flex items-center gap-2 cursor-pointer text-[12px] font-medium text-[var(--text-secondary)] px-3 py-1.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">
            <input
              type="checkbox"
              checked={filterDivergentOnly}
              onChange={(e) => setFilterDivergentOnly(e.target.checked)}
              className="w-4 h-4 text-indigo-600 rounded"
            />
            <span>Divergent Decisions Only ({predictions.length - consensusCount})</span>
          </label>

          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsPromoteModalOpen(true)}
            leftIcon={<TrendingUp className="w-4 h-4" />}
          >
            Promote Shadow Model
          </Button>
        </div>
      </div>

      {/* Consensus Metric Banner */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 text-[12px] font-mono-financial">
        <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-1">
          <span className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
            Tri-Model Consensus Rate
          </span>
          <div className="text-[22px] font-bold text-emerald-700">{consensusPct}%</div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            {consensusCount} of {predictions.length} decisions identical across all 3 engines
          </div>
        </div>

        <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-1">
          <span className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
            Divergent Decisions Flagged
          </span>
          <div className="text-[22px] font-bold text-amber-700">
            {predictions.length - consensusCount}
          </div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Primary cause: ML high-confidence override on conservative monetary ceilings
          </div>
        </div>

        <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-1">
          <span className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
            Safety Gate Integrity
          </span>
          <div className="text-[22px] font-bold text-[var(--text-primary)]">100.0%</div>
          <div className="text-[11px] text-[var(--text-secondary)]">
            Zero shadow false positives that bypassed TSQ counterparty check
          </div>
        </div>
      </div>

      {/* Side-by-Side Comparison Table */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs overflow-hidden">
        <div className="px-5 py-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <span className="font-bold text-[13px] text-[var(--text-primary)]">
            Real-Time Decision Stream (Production vs Shadow Models)
          </span>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            Showing {filtered.length} of {predictions.length} predictions
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px] font-mono-financial">
            <thead>
              <tr className="border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
                <th className="py-2.5 px-4">Transaction Ref</th>
                <th className="py-2.5 px-4">Channel & Amount</th>
                <th className="py-2.5 px-4 text-center">Production Rule Engine</th>
                <th className="py-2.5 px-4 text-center">Shadow Random Forest</th>
                <th className="py-2.5 px-4 text-center">Shadow XGBoost v2.1</th>
                <th className="py-2.5 px-4 text-center">Consensus</th>
                <th className="py-2.5 px-4">Confidence</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--surface-border-subtle)]">
              {filtered.map((p) => (
                <tr key={p.id} className="hover:bg-[var(--canvas-subdued)] transition-colors">
                  <td className="py-3 px-4 font-bold text-[var(--text-primary)]">
                    <div>{p.transactionRef}</div>
                    <div className="text-[10px] text-[var(--text-muted)] font-normal">
                      {p.timestamp}
                    </div>
                  </td>

                  <td className="py-3 px-4">
                    <div className="font-bold text-[var(--text-primary)]">₦{p.amountNgn}</div>
                    <div className="text-[10px] text-[var(--text-muted)]">{p.channel} Channel</div>
                  </td>

                  {/* Production Rule Engine */}
                  <td className="py-3 px-4 text-center">
                    <span
                      className={`inline-block text-[10px] font-bold px-2 py-0.5 rounded ${
                        p.ruleEngineVerdict === 'AUTO_REVERSE'
                          ? 'bg-emerald-100 text-emerald-800'
                          : p.ruleEngineVerdict === 'MANUAL_INVESTIGATION'
                          ? 'bg-amber-100 text-amber-800'
                          : 'bg-rose-100 text-rose-800'
                      }`}
                    >
                      {p.ruleEngineVerdict.replace(/_/g, ' ')}
                    </span>
                  </td>

                  {/* Shadow Random Forest */}
                  <td className="py-3 px-4 text-center">
                    <span
                      className={`inline-block text-[10px] font-bold px-2 py-0.5 rounded ${
                        p.randomForestVerdict === 'AUTO_REVERSE'
                          ? 'bg-emerald-100 text-emerald-800'
                          : p.randomForestVerdict === 'MANUAL_INVESTIGATION'
                          ? 'bg-amber-100 text-amber-800'
                          : 'bg-rose-100 text-rose-800'
                      }`}
                    >
                      {p.randomForestVerdict.replace(/_/g, ' ')}
                    </span>
                  </td>

                  {/* Shadow XGBoost */}
                  <td className="py-3 px-4 text-center">
                    <span
                      className={`inline-block text-[10px] font-bold px-2 py-0.5 rounded ${
                        p.xgboostVerdict === 'AUTO_REVERSE'
                          ? 'bg-emerald-100 text-emerald-800'
                          : p.xgboostVerdict === 'MANUAL_INVESTIGATION'
                          ? 'bg-amber-100 text-amber-800'
                          : 'bg-rose-100 text-rose-800'
                      }`}
                    >
                      {p.xgboostVerdict.replace(/_/g, ' ')}
                    </span>
                  </td>

                  {/* Consensus */}
                  <td className="py-3 px-4 text-center">
                    {p.isConsensus ? (
                      <span className="px-2 py-0.5 rounded bg-emerald-50 text-emerald-900 border border-emerald-300 font-bold text-[10px]">
                        ✓ CONSENSUS
                      </span>
                    ) : (
                      <div className="space-y-0.5">
                        <span className="px-2 py-0.5 rounded bg-amber-50 text-amber-900 border border-amber-300 font-bold text-[10px]">
                          ⚠ DIVERGENT
                        </span>
                        {p.divergenceNotes && (
                          <p className="text-[10px] text-amber-900 font-sans max-w-xs text-left leading-tight mt-1">
                            {p.divergenceNotes}
                          </p>
                        )}
                      </div>
                    )}
                  </td>

                  {/* Confidence */}
                  <td className="py-3 px-4 font-bold text-[var(--text-primary)]">
                    {p.confidenceScorePct}%
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Model Promotion Modal */}
      {isPromoteModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-200">
          <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl w-full max-w-lg overflow-hidden">
            <div className="px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
              <h4 className="font-bold text-[15px] text-[var(--text-primary)]">
                Promote Shadow Model to Active Production
              </h4>
            </div>

            <form onSubmit={handlePromoteSubmit} className="p-5 space-y-4 text-[12px]">
              <div className="p-3 bg-indigo-50 border border-indigo-200 rounded text-indigo-950 flex items-start gap-2">
                <Zap className="w-4 h-4 text-indigo-600 shrink-0 mt-0.5" />
                <span>
                  Promoting <strong>XGBoost Anomaly Reversal Predictor v2.1.0</strong> will make it the
                  primary decision engine for automated 8-check reversibility scoring, replacing the
                  baseline rule engine with automated rollback capability.
                </span>
              </div>

              <div>
                <label className="block font-bold text-[var(--text-primary)] mb-1">
                  Model Governance & Committee Ratification Notes *
                </label>
                <textarea
                  required
                  rows={3}
                  value={justification}
                  onChange={(e) => setJustification(e.target.value)}
                  placeholder="e.g. Evaluated across 14,000 live shadow transactions with 99.4% ROC-AUC and zero false-positive recalls. Passed AI risk threat model..."
                  className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2 border-t border-[var(--surface-border-subtle)]">
                <Button
                  variant="ghost"
                  size="sm"
                  type="button"
                  onClick={() => setIsPromoteModalOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  type="submit"
                  disabled={!justification.trim()}
                  leftIcon={<Send className="w-3.5 h-3.5" />}
                >
                  Submit for Executive Rollout
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
