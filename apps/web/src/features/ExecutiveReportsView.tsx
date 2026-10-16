import React, { useState } from 'react';
import { BarChart3, FileText, Flame } from 'lucide-react';
import {
  EXECUTIVE_KPI_SUMMARY,
  CBN_REGULATORY_DISPUTE_RETURN,
  MERCHANT_RISK_HOTSPOTS,
} from './reports/reportsData';
import { ExecutiveKpiTab } from './reports/ExecutiveKpiTab';
import { RegulatoryGeneratorTab } from './reports/RegulatoryGeneratorTab';
import { RiskHeatmapTab } from './reports/RiskHeatmapTab';

export const ExecutiveReportsView: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'KPIS' | 'REGULATORY' | 'HEATMAP'>('KPIS');

  return (
    <div className="space-y-6">
      {/* Top Navigation Tabs */}
      <div className="flex items-center gap-2 border-b border-[var(--surface-border)] pb-2 overflow-x-auto">
        <button
          onClick={() => setActiveTab('KPIS')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 whitespace-nowrap ${
            activeTab === 'KPIS'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <BarChart3 className="w-4 h-4" />
          <span>Executive KPIs & Recovery</span>
        </button>

        <button
          onClick={() => setActiveTab('REGULATORY')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 whitespace-nowrap ${
            activeTab === 'REGULATORY'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <FileText className="w-4 h-4" />
          <span>CBN Regulatory Return Generator</span>
        </button>

        <button
          onClick={() => setActiveTab('HEATMAP')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 whitespace-nowrap ${
            activeTab === 'HEATMAP'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <Flame className="w-4 h-4" />
          <span>Merchant & Terminal Risk Heatmap</span>
        </button>
      </div>

      {/* Tab Contents */}
      {activeTab === 'KPIS' && <ExecutiveKpiTab kpi={EXECUTIVE_KPI_SUMMARY} />}
      {activeTab === 'REGULATORY' && (
        <RegulatoryGeneratorTab report={CBN_REGULATORY_DISPUTE_RETURN} />
      )}
      {activeTab === 'HEATMAP' && <RiskHeatmapTab hotspots={MERCHANT_RISK_HOTSPOTS} />}
    </div>
  );
};
