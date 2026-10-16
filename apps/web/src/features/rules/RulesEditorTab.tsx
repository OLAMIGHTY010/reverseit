import React, { useState } from 'react';
import {
  Sliders,
  Code,
  ShieldCheck,
  CheckCircle,
  AlertTriangle,
  RotateCcw,
  Send,
  Eye,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { SafetyGateRulesConfig } from './rulesTypes';

interface RulesEditorTabProps {
  config: SafetyGateRulesConfig;
  onSaveProposedConfig: (newCfg: SafetyGateRulesConfig, justification: string) => void;
  onResetDefaults: () => void;
}

export const RulesEditorTab: React.FC<RulesEditorTabProps> = ({
  config,
  onSaveProposedConfig,
  onResetDefaults,
}) => {
  const [form, setForm] = useState<SafetyGateRulesConfig>(config);
  const [viewMode, setViewMode] = useState<'VISUAL' | 'YAML'>('VISUAL');
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [makerJustification, setMakerJustification] = useState('');

  const yamlString = `# SmartReversal 8-Check Safety Pre-flight Engine Rules
# Central Bank of Nigeria Electronic Payment Compliance
tier1_auto_reversal_max_ngn: ${form.tier1AutoReversalMaxNgn}
tier2_supervisor_threshold_ngn: ${form.tier2SupervisorThresholdNgn}
tier3_committee_threshold_ngn: ${form.tier3CommitteeThresholdNgn}

# Machine Learning Anomaly Detection Threshold
minimum_model_confidence_pct: ${form.minimumModelConfidencePct}

# Window Configurations
max_duplicate_window_seconds: ${form.maxDuplicateWindowSeconds}
fx_rounding_tolerance_ngn: ${form.fxRoundingToleranceNgn}
regulatory_urgent_alert_hours: ${form.regulatoryUrgentAlertHours}

# Safety Gate Check Enforcements
require_counterparty_tsq_query: ${form.requireCounterpartyTsqQuery}
enable_idempotency_lock: ${form.enableIdempotencyLock}
block_blacklisted_beneficiaries: ${form.blockBlacklistedBeneficiaries}
`;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!makerJustification.trim()) return;
    onSaveProposedConfig(form, makerJustification);
    setIsModalOpen(false);
    setMakerJustification('');
  };

  return (
    <div className="space-y-6">
      {/* Tab Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs">
        <div>
          <div className="flex items-center gap-2">
            <Sliders className="w-5 h-5 text-indigo-600" />
            <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
              Decision Rules & SLA Thresholds Editor
            </h3>
            <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-blue-100 text-blue-800 font-mono-financial">
              Policy Governance • Hot-Reload Engine
            </span>
          </div>
          <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
            Configure automated 8-check pre-flight criteria, Maker-Checker authorization tiers, and
            tolerance boundaries.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="flex rounded-md border border-[var(--surface-border)] p-0.5 bg-[var(--canvas-subdued)]">
            <button
              onClick={() => setViewMode('VISUAL')}
              className={`px-3 py-1 text-[11px] font-bold rounded transition-colors flex items-center gap-1 ${
                viewMode === 'VISUAL'
                  ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-xs'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
              }`}
            >
              <Eye className="w-3.5 h-3.5" />
              <span>Visual Sliders</span>
            </button>
            <button
              onClick={() => setViewMode('YAML')}
              className={`px-3 py-1 text-[11px] font-bold rounded transition-colors flex items-center gap-1 ${
                viewMode === 'YAML'
                  ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-xs'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
              }`}
            >
              <Code className="w-3.5 h-3.5" />
              <span>YAML Spec</span>
            </button>
          </div>

          <Button
            variant="ghost"
            size="sm"
            onClick={onResetDefaults}
            leftIcon={<RotateCcw className="w-3.5 h-3.5" />}
          >
            Reset
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsModalOpen(true)}
            leftIcon={<Send className="w-3.5 h-3.5" />}
          >
            Propose Rule Updates
          </Button>
        </div>
      </div>

      {viewMode === 'VISUAL' ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Card 1: Monetary Authorization Tiers */}
          <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
            <div className="flex items-center justify-between pb-2 border-b border-[var(--surface-border-subtle)]">
              <span className="font-bold text-[13px] text-[var(--text-primary)] flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-emerald-600" />
                <span>Multi-Tier Authorization Gates</span>
              </span>
              <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                Maker-Checker Matrix
              </span>
            </div>

            <div className="space-y-4 text-[12px]">
              <div>
                <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
                  <span>Tier 1: Automated Instant Reversal Ceiling</span>
                  <span className="font-mono-financial text-indigo-700">
                    ₦{form.tier1AutoReversalMaxNgn.toLocaleString()}
                  </span>
                </div>
                <input
                  type="range"
                  min="5000"
                  max="200000"
                  step="5000"
                  value={form.tier1AutoReversalMaxNgn}
                  onChange={(e) =>
                    setForm({ ...form, tier1AutoReversalMaxNgn: Number(e.target.value) })
                  }
                  className="w-full accent-indigo-600"
                />
                <p className="text-[11px] text-[var(--text-muted)]">
                  Exceptions below this limit pass through 8-check gate without manual analyst intervention.
                </p>
              </div>

              <div>
                <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
                  <span>Tier 2: Supervisor Authorization Threshold</span>
                  <span className="font-mono-financial text-indigo-700">
                    ₦{form.tier2SupervisorThresholdNgn.toLocaleString()}
                  </span>
                </div>
                <input
                  type="range"
                  min="100000"
                  max="2000000"
                  step="50000"
                  value={form.tier2SupervisorThresholdNgn}
                  onChange={(e) =>
                    setForm({ ...form, tier2SupervisorThresholdNgn: Number(e.target.value) })
                  }
                  className="w-full accent-indigo-600"
                />
                <p className="text-[11px] text-[var(--text-muted)]">
                  Requires 1 Checker signoff. Amounts exceeding this escalate to Committee.
                </p>
              </div>

              <div>
                <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
                  <span>Tier 3: Executive Committee Threshold</span>
                  <span className="font-mono-financial text-indigo-700">
                    ₦{form.tier3CommitteeThresholdNgn.toLocaleString()}
                  </span>
                </div>
                <input
                  type="range"
                  min="2000000"
                  max="20000000"
                  step="500000"
                  value={form.tier3CommitteeThresholdNgn}
                  onChange={(e) =>
                    setForm({ ...form, tier3CommitteeThresholdNgn: Number(e.target.value) })
                  }
                  className="w-full accent-indigo-600"
                />
              </div>
            </div>
          </div>

          {/* Card 2: AI Confidence & Safety Check Enforcements */}
          <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
            <div className="flex items-center justify-between pb-2 border-b border-[var(--surface-border-subtle)]">
              <span className="font-bold text-[13px] text-[var(--text-primary)] flex items-center gap-2">
                <Sliders className="w-4 h-4 text-indigo-600" />
                <span>Detection & Temporal Windows</span>
              </span>
              <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
                Pre-Flight Rules
              </span>
            </div>

            <div className="space-y-4 text-[12px]">
              <div>
                <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
                  <span>Minimum ML Model Confidence Required</span>
                  <span className="font-mono-financial text-emerald-700 font-bold">
                    {form.minimumModelConfidencePct}%
                  </span>
                </div>
                <input
                  type="range"
                  min="80"
                  max="99"
                  step="1"
                  value={form.minimumModelConfidencePct}
                  onChange={(e) =>
                    setForm({ ...form, minimumModelConfidencePct: Number(e.target.value) })
                  }
                  className="w-full accent-emerald-600"
                />
                <p className="text-[11px] text-[var(--text-muted)]">
                  Models scoring below this threshold are routed to human Analyst investigation queue.
                </p>
              </div>

              <div>
                <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
                  <span>Duplicate Detection Sliding Window</span>
                  <span className="font-mono-financial text-indigo-700">
                    {form.maxDuplicateWindowSeconds}s ({form.maxDuplicateWindowSeconds / 60}m)
                  </span>
                </div>
                <input
                  type="range"
                  min="30"
                  max="600"
                  step="30"
                  value={form.maxDuplicateWindowSeconds}
                  onChange={(e) =>
                    setForm({ ...form, maxDuplicateWindowSeconds: Number(e.target.value) })
                  }
                  className="w-full accent-indigo-600"
                />
              </div>

              <div>
                <div className="flex justify-between font-bold text-[var(--text-primary)] mb-1">
                  <span>Cross-Border FX Rounding Tolerance</span>
                  <span className="font-mono-financial text-indigo-700">
                    ±₦{form.fxRoundingToleranceNgn.toFixed(2)}
                  </span>
                </div>
                <input
                  type="range"
                  min="1"
                  max="20"
                  step="0.5"
                  value={form.fxRoundingToleranceNgn}
                  onChange={(e) =>
                    setForm({ ...form, fxRoundingToleranceNgn: Number(e.target.value) })
                  }
                  className="w-full accent-indigo-600"
                />
              </div>
            </div>
          </div>

          {/* Card 3: 8-Check Gate Toggle Matrix */}
          <div className="md:col-span-2 p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
            <span className="font-bold text-[13px] text-[var(--text-primary)] flex items-center gap-2">
              <CheckCircle className="w-4 h-4 text-blue-600" />
              <span>Mandatory 8-Check Safety Pre-flight Assertions</span>
            </span>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-[12px]">
              <label className="flex items-start gap-2.5 p-3 rounded border border-[var(--surface-border)] bg-[var(--canvas-subdued)] cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.requireCounterpartyTsqQuery}
                  onChange={(e) =>
                    setForm({ ...form, requireCounterpartyTsqQuery: e.target.checked })
                  }
                  className="mt-1 text-indigo-600 rounded"
                />
                <div>
                  <div className="font-bold text-[var(--text-primary)]">Enforce TSQ Query</div>
                  <div className="text-[11px] text-[var(--text-muted)]">
                    Never reverse until destination switch confirms 0 credit posted.
                  </div>
                </div>
              </label>

              <label className="flex items-start gap-2.5 p-3 rounded border border-[var(--surface-border)] bg-[var(--canvas-subdued)] cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.enableIdempotencyLock}
                  onChange={(e) => setForm({ ...form, enableIdempotencyLock: e.target.checked })}
                  className="mt-1 text-indigo-600 rounded"
                />
                <div>
                  <div className="font-bold text-[var(--text-primary)]">Single-Writer Lock</div>
                  <div className="text-[11px] text-[var(--text-muted)]">
                    Enforces unique Redis idempotency lock to prevent double reversals.
                  </div>
                </div>
              </label>

              <label className="flex items-start gap-2.5 p-3 rounded border border-[var(--surface-border)] bg-[var(--canvas-subdued)] cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.blockBlacklistedBeneficiaries}
                  onChange={(e) =>
                    setForm({ ...form, blockBlacklistedBeneficiaries: e.target.checked })
                  }
                  className="mt-1 text-indigo-600 rounded"
                />
                <div>
                  <div className="font-bold text-[var(--text-primary)]">Watchlist AML Gate</div>
                  <div className="text-[11px] text-[var(--text-muted)]">
                    Auto-blocks reversals involving high-risk or flagged beneficiary accounts.
                  </div>
                </div>
              </label>
            </div>
          </div>
        </div>
      ) : (
        <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2">
          <div className="flex items-center justify-between text-[11px] font-mono-financial text-[var(--text-muted)] pb-2 border-b border-[var(--surface-border-subtle)]">
            <span>rules_spec_v2.yaml (Read-Only Preview)</span>
            <span>Canonical Schema: CBN-REV-2026</span>
          </div>
          <pre className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[12px] font-mono-financial text-[var(--text-primary)] overflow-x-auto leading-relaxed">
            {yamlString}
          </pre>
        </div>
      )}

      {/* Maker-Checker Proposal Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-200">
          <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl w-full max-w-lg overflow-hidden">
            <div className="px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
              <h4 className="font-bold text-[15px] text-[var(--text-primary)]">
                Submit Rules Change Proposal (Dual-Control)
              </h4>
            </div>

            <form onSubmit={handleSubmit} className="p-5 space-y-4 text-[12px]">
              <div className="p-3 bg-amber-50 border border-amber-200 rounded text-amber-900 flex items-start gap-2">
                <AlertTriangle className="w-4 h-4 text-amber-600 shrink-0 mt-0.5" />
                <span>
                  Modifications to core safety gate thresholds require formal justification and must
                  be ratified by a designated Supervisor before hot-reloading.
                </span>
              </div>

              <div>
                <label className="block font-bold text-[var(--text-primary)] mb-1">
                  Change Rationale & Audit Justification *
                </label>
                <textarea
                  required
                  rows={3}
                  value={makerJustification}
                  onChange={(e) => setMakerJustification(e.target.value)}
                  placeholder="e.g. Adjusted Tier 1 threshold to ₦50,000 in accordance with new CBN Circular for micro-payments..."
                  className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2 border-t border-[var(--surface-border-subtle)]">
                <Button
                  variant="ghost"
                  size="sm"
                  type="button"
                  onClick={() => setIsModalOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  type="submit"
                  disabled={!makerJustification.trim()}
                  leftIcon={<Send className="w-3.5 h-3.5" />}
                >
                  Submit for Supervisor Ratification
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
