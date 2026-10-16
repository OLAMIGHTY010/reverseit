import React from 'react';
import {
  Brain,
  Activity,
} from 'lucide-react';
import type { ModelMetricSnapshot, FeatureDriftPsi } from './modelsTypes';

interface ModelDriftTabProps {
  models: ModelMetricSnapshot[];
  driftMetrics: FeatureDriftPsi[];
}

export const ModelDriftTab: React.FC<ModelDriftTabProps> = ({ models, driftMetrics }) => {
  return (
    <div className="space-y-6">
      {/* Model Registry Performance Table */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs overflow-hidden">
        <div className="px-5 py-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Brain className="w-4 h-4 text-indigo-600" />
            <span className="font-bold text-[13px] text-[var(--text-primary)]">
              Model Performance & Telemetry Registry
            </span>
          </div>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            Benchmark against Baseline
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px] font-mono-financial">
            <thead>
              <tr className="border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
                <th className="py-2.5 px-4">Model & Version</th>
                <th className="py-2.5 px-4">Deployment Role</th>
                <th className="py-2.5 px-4 text-right">Accuracy %</th>
                <th className="py-2.5 px-4 text-right">Precision %</th>
                <th className="py-2.5 px-4 text-right">Recall %</th>
                <th className="py-2.5 px-4 text-right">F1 Score</th>
                <th className="py-2.5 px-4 text-right">ROC-AUC</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--surface-border-subtle)]">
              {models.map((m) => (
                <tr key={m.modelName} className="hover:bg-[var(--canvas-subdued)] transition-colors">
                  <td className="py-3 px-4 font-sans font-bold text-[var(--text-primary)]">
                    <div>{m.modelName}</div>
                    <div className="text-[10px] font-mono-financial text-[var(--text-muted)]">
                      {m.version} • Deployed: {m.deployedAt}
                    </div>
                  </td>
                  <td className="py-3 px-4">
                    <span
                      className={`inline-block text-[10px] font-bold px-2 py-0.5 rounded font-mono-financial ${
                        m.status === 'ACTIVE_PRODUCTION'
                          ? 'bg-emerald-100 text-emerald-800 border border-emerald-300'
                          : 'bg-indigo-100 text-indigo-800 border border-indigo-300'
                      }`}
                    >
                      {m.status.replace(/_/g, ' ')}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-right font-bold text-[var(--text-primary)]">
                    {m.accuracyPct}%
                  </td>
                  <td className="py-3 px-4 text-right">{m.precisionPct}%</td>
                  <td className="py-3 px-4 text-right">{m.recallPct}%</td>
                  <td className="py-3 px-4 text-right">{m.f1Score.toFixed(3)}</td>
                  <td className="py-3 px-4 text-right text-indigo-700 font-bold">
                    {m.rocAuc.toFixed(3)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Population Stability Index (PSI) Concept Drift Monitor */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs overflow-hidden">
        <div className="px-5 py-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Activity className="w-4 h-4 text-amber-600" />
            <span className="font-bold text-[13px] text-[var(--text-primary)]">
              Feature-Level Population Stability Index (PSI) Drift Monitor
            </span>
          </div>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            Retrain Threshold: PSI &gt; 0.25
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px] font-mono-financial">
            <thead>
              <tr className="border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
                <th className="py-2.5 px-4">Feature Name</th>
                <th className="py-2.5 px-4">Category</th>
                <th className="py-2.5 px-4 text-right">Baseline Mean</th>
                <th className="py-2.5 px-4 text-right">Current Mean</th>
                <th className="py-2.5 px-4 text-right">PSI Score</th>
                <th className="py-2.5 px-4 text-center">Drift State</th>
                <th className="py-2.5 px-4">SHAP Feature Importance</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--surface-border-subtle)]">
              {driftMetrics.map((f) => (
                <tr key={f.featureName} className="hover:bg-[var(--canvas-subdued)] transition-colors">
                  <td className="py-3 px-4 font-bold text-[var(--text-primary)]">{f.featureName}</td>
                  <td className="py-3 px-4 text-[11px] text-[var(--text-muted)] font-sans">
                    {f.category}
                  </td>
                  <td className="py-3 px-4 text-right">{f.baselineDistributionMean}</td>
                  <td className="py-3 px-4 text-right">{f.currentDistributionMean}</td>
                  <td className="py-3 px-4 text-right font-bold text-[13px]">
                    {f.psiScore.toFixed(3)}
                  </td>
                  <td className="py-3 px-4 text-center">
                    <span
                      className={`inline-block text-[10px] font-bold px-2 py-0.5 rounded font-mono-financial ${
                        f.driftStatus === 'STABLE'
                          ? 'bg-emerald-100 text-emerald-800'
                          : f.driftStatus === 'MODERATE_DRIFT'
                          ? 'bg-amber-100 text-amber-800'
                          : 'bg-rose-100 text-rose-800 border border-rose-300'
                      }`}
                    >
                      {f.driftStatus.replace(/_/g, ' ')}
                    </span>
                  </td>
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-2">
                      <div className="flex-1 bg-[var(--canvas-subdued)] h-2 rounded-full overflow-hidden">
                        <div
                          className="bg-indigo-600 h-full rounded-full"
                          style={{ width: `${f.shapImportancePct}%` }}
                        />
                      </div>
                      <span className="text-[11px] text-[var(--text-muted)] w-10 text-right">
                        {f.shapImportancePct}%
                      </span>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
