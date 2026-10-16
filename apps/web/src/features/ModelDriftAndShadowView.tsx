import React, { useState } from 'react';
import { Brain, Layers } from 'lucide-react';
import { useApp } from '../context/AppContext';
import {
  MODEL_REGISTRY,
  FEATURE_DRIFT_METRICS,
  SHADOW_PREDICTIONS,
} from './models/modelsData';
import type { ModelMetricSnapshot } from './models/modelsTypes';
import { ModelDriftTab } from './models/ModelDriftTab';
import { ShadowComparisonTab } from './models/ShadowComparisonTab';

export const ModelDriftAndShadowView: React.FC = () => {
  const { addToast } = useApp();

  const [activeTab, setActiveTab] = useState<'DRIFT' | 'SHADOW'>('DRIFT');
  const [models, setModels] = useState<ModelMetricSnapshot[]>(MODEL_REGISTRY);

  const handlePromoteModel = (modelName: string, justification: string) => {
    setModels((prev) =>
      prev.map((m) =>
        m.modelName.includes('XGBoost')
          ? { ...m, status: 'ACTIVE_PRODUCTION' }
          : m.modelName.includes('Rule Engine')
          ? { ...m, status: 'RETIRED' }
          : m
      )
    );
    addToast({
      type: 'success',
      title: 'Shadow Model Promoted to Active Production',
      message: `${modelName} has been ratified. Rollback checkpoints preserved: "${justification.slice(0, 40)}..."`,
    });
  };

  return (
    <div className="space-y-6">
      {/* Top Tab Switcher */}
      <div className="flex items-center gap-2 border-b border-[var(--surface-border)] pb-2 overflow-x-auto">
        <button
          onClick={() => setActiveTab('DRIFT')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 whitespace-nowrap ${
            activeTab === 'DRIFT'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <Brain className="w-4 h-4" />
          <span>ML Model Performance & Concept Drift</span>
        </button>

        <button
          onClick={() => setActiveTab('SHADOW')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 whitespace-nowrap ${
            activeTab === 'SHADOW'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <Layers className="w-4 h-4" />
          <span>Shadow Mode Model Comparison Matrix</span>
        </button>
      </div>

      {/* Tab Contents */}
      {activeTab === 'DRIFT' ? (
        <ModelDriftTab models={models} driftMetrics={FEATURE_DRIFT_METRICS} />
      ) : (
        <ShadowComparisonTab
          predictions={SHADOW_PREDICTIONS}
          onPromoteModel={handlePromoteModel}
        />
      )}
    </div>
  );
};
