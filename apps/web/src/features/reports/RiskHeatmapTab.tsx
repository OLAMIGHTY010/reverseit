import React, { useState } from 'react';
import {
  MapPin,
  Search,
  Filter,
  Flame,
  Store,
} from 'lucide-react';
import type { MerchantRiskHotspot } from './reportsTypes';

interface RiskHeatmapTabProps {
  hotspots: MerchantRiskHotspot[];
}

export const RiskHeatmapTab: React.FC<RiskHeatmapTabProps> = ({ hotspots }) => {
  const [search, setSearch] = useState('');
  const [filterRating, setFilterRating] = useState<string>('ALL');

  const filtered = hotspots.filter((h) => {
    if (filterRating !== 'ALL' && h.riskRating !== filterRating) return false;
    if (search.trim()) {
      const q = search.toLowerCase();
      return (
        h.merchantName.toLowerCase().includes(q) ||
        h.location.toLowerCase().includes(q) ||
        h.terminalId.toLowerCase().includes(q) ||
        h.acquirerBank.toLowerCase().includes(q)
      );
    }
    return true;
  });

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs">
        <div>
          <div className="flex items-center gap-2">
            <Flame className="w-5 h-5 text-rose-600" />
            <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
              Merchant Risk & Anomaly Hotspot Heatmap
            </h3>
            <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-rose-100 text-rose-800 font-mono-financial">
              Risk Telemetry • Geographic Distribution
            </span>
          </div>
          <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
            Pinpoints recurring terminal failure modes, merchant dispute clusters, and acquirer
            switch timeout anomalies.
          </p>
        </div>

        {/* Filter Controls */}
        <div className="flex items-center gap-3">
          <div className="relative">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search merchant, terminal or location..."
              className="pl-9 pr-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-rose-500"
            />
          </div>

          <div className="flex items-center gap-1.5 text-[12px]">
            <Filter className="w-3.5 h-3.5 text-[var(--text-muted)]" />
            <select
              value={filterRating}
              onChange={(e) => setFilterRating(e.target.value)}
              className="px-2.5 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)]"
            >
              <option value="ALL">All Risk Ratings</option>
              <option value="CRITICAL">Critical</option>
              <option value="HIGH">High</option>
              <option value="ELEVATED">Elevated</option>
            </select>
          </div>
        </div>
      </div>

      {/* Hotspots Grid / Table */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs overflow-hidden">
        <div className="px-5 py-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <span className="font-bold text-[13px] text-[var(--text-primary)]">
            High-Frequency Dispute Terminals & Merchant Clusters
          </span>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            Showing {filtered.length} of {hotspots.length} flagged terminals
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px] font-mono-financial">
            <thead>
              <tr className="border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
                <th className="py-2.5 px-4">Merchant & Location</th>
                <th className="py-2.5 px-4">Terminal & Channel</th>
                <th className="py-2.5 px-4 text-center">30d Dispute Count</th>
                <th className="py-2.5 px-4">Disputed Volume (₦)</th>
                <th className="py-2.5 px-4">Dominant Failure Mode</th>
                <th className="py-2.5 px-4">Acquirer Partner</th>
                <th className="py-2.5 px-4 text-right">Risk Grade</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--surface-border-subtle)]">
              {filtered.map((h) => (
                <tr key={h.id} className="hover:bg-[var(--canvas-subdued)] transition-colors">
                  <td className="py-3 px-4 space-y-0.5">
                    <div className="font-bold text-[var(--text-primary)] font-sans flex items-center gap-1.5">
                      <Store className="w-3.5 h-3.5 text-slate-500" />
                      <span>{h.merchantName}</span>
                    </div>
                    <div className="text-[11px] text-[var(--text-muted)] flex items-center gap-1">
                      <MapPin className="w-3 h-3 text-rose-500" />
                      <span>{h.location}</span>
                    </div>
                  </td>

                  <td className="py-3 px-4 space-y-0.5">
                    <div className="font-bold text-[var(--text-primary)]">{h.terminalId}</div>
                    <div className="text-[10px] text-[var(--text-muted)]">{h.channel} Channel</div>
                  </td>

                  <td className="py-3 px-4 text-center">
                    <span className="inline-block px-2 py-0.5 rounded-full bg-rose-50 border border-rose-200 text-rose-900 font-bold text-[12px]">
                      {h.disputeCount30d} cases
                    </span>
                  </td>

                  <td className="py-3 px-4 font-bold text-rose-700">{h.totalDisputedVolumeNgn}</td>

                  <td className="py-3 px-4 font-sans text-[11px] text-[var(--text-secondary)]">
                    {h.primaryFailureMode}
                  </td>

                  <td className="py-3 px-4 font-sans text-[11px] text-[var(--text-muted)]">
                    {h.acquirerBank}
                  </td>

                  <td className="py-3 px-4 text-right">
                    <span
                      className={`inline-block px-2 py-0.5 rounded font-bold text-[10px] ${
                        h.riskRating === 'CRITICAL'
                          ? 'bg-rose-600 text-white'
                          : h.riskRating === 'HIGH'
                          ? 'bg-amber-600 text-white'
                          : 'bg-yellow-100 text-yellow-900 border border-yellow-300'
                      }`}
                    >
                      {h.riskRating}
                    </span>
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
