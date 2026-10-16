import React, { useState } from 'react';
import {
  RefreshCw,
  FileSpreadsheet,
  Sliders,
  Download,
  Search,
  Filter,
  Clock,
  ShieldAlert,
  Link2,
  Building,
  Server,
  Layers,
} from 'lucide-react';
import { Button } from '../components/Button';
import { useApp } from '../context/AppContext';
import {
  SETTLEMENT_CYCLES,
  INITIAL_RECONCILIATION_BREAKS,
  DEFAULT_TOLERANCE_CONFIG,
} from './reconciliation/reconciliationData';
import type {
  ReconciliationBreak,
  SettlementBatchCycle,
  ToleranceConfig,
  BreakCategory,
} from './reconciliation/reconciliationTypes';
import { ToleranceSettingsModal } from './reconciliation/ToleranceSettingsModal';
import { ManualPairingModal } from './reconciliation/ManualPairingModal';
import { DisputeExportModal } from './reconciliation/DisputeExportModal';
import { BreakDetailDrawer } from './reconciliation/BreakDetailDrawer';

export const ReconciliationView: React.FC = () => {
  const { addToast, setActiveView } = useApp();

  // State
  const [cycles] = useState<SettlementBatchCycle[]>(SETTLEMENT_CYCLES);
  const [selectedCycleId, setSelectedCycleId] = useState<string>('cycle-nip-0900');
  const [breaks, setBreaks] = useState<ReconciliationBreak[]>(INITIAL_RECONCILIATION_BREAKS);
  const [toleranceConfig, setToleranceConfig] = useState<ToleranceConfig>(DEFAULT_TOLERANCE_CONFIG);

  // Filters
  const [statusFilter, setStatusFilter] = useState<'ALL' | 'UNMATCHED' | 'TOLERANCE' | 'PAIRED'>('ALL');
  const [categoryFilter, setCategoryFilter] = useState<BreakCategory>('ALL');
  const [searchQuery, setSearchQuery] = useState('');
  const [channelFilter, setChannelFilter] = useState<string>('ALL');

  // Modals & Drawers
  const [isToleranceModalOpen, setIsToleranceModalOpen] = useState(false);
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);
  const [isPairingModalOpen, setIsPairingModalOpen] = useState(false);
  const [selectedBreak, setSelectedBreak] = useState<ReconciliationBreak | null>(null);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  // Cycle Trigger
  const [isRunning, setIsRunning] = useState(false);

  const activeCycle = cycles.find((c) => c.id === selectedCycleId) || cycles[0];

  const handleTriggerRun = () => {
    setIsRunning(true);
    setTimeout(() => {
      setIsRunning(false);
      addToast({
        type: 'success',
        title: 'Settlement Intake Cycle Completed',
        message: `Ingested ${activeCycle.totalSwitchRecords.toLocaleString()} switch records against core ledger for ${activeCycle.sessionCode}.`,
      });
    }, 1200);
  };

  const handleOpenBreakDetail = (b: ReconciliationBreak) => {
    setSelectedBreak(b);
    setIsDrawerOpen(true);
  };

  const handleApplyTolerance = (breakId: string) => {
    setBreaks((prev) =>
      prev.map((b) =>
        b.id === breakId
          ? {
              ...b,
              status: 'TOLERANCE_MATCHED',
              isToleranceApplied: true,
              toleranceRule: `Applied ±₦${toleranceConfig.amountToleranceNgn.toFixed(2)} Tolerance Rule`,
            }
          : b
      )
    );
    if (selectedBreak?.id === breakId) {
      setSelectedBreak((prev) =>
        prev
          ? {
              ...prev,
              status: 'TOLERANCE_MATCHED',
              isToleranceApplied: true,
              toleranceRule: `Applied ±₦${toleranceConfig.amountToleranceNgn.toFixed(2)} Tolerance Rule`,
            }
          : null
      );
    }
    addToast({
      type: 'success',
      title: 'Tolerance Rule Applied',
      message: `Break ${breakId} reconciled within allowable variance threshold.`,
    });
  };

  const handleConfirmManualPair = (breakId: string, notes: string) => {
    setBreaks((prev) =>
      prev.map((b) =>
        b.id === breakId
          ? {
              ...b,
              status: 'MANUALLY_PAIRED',
              makerNotes: notes,
              checkerSignoff: {
                status: 'PENDING',
              },
            }
          : b
      )
    );
    addToast({
      type: 'info',
      title: 'Pairing Submitted for Dual-Control Sign-Off',
      message: `Maker force-match registered for break ${breakId}. Awaiting Checker sign-off.`,
    });
  };

  const handleEscalateInvestigation = (breakId: string) => {
    setIsDrawerOpen(false);
    setActiveView('investigations');
    addToast({
      type: 'info',
      title: 'Escalated to Investigation Workspace',
      message: `Opened case workspace for reconciliation break ${breakId}.`,
    });
  };

  // Filtered breaks
  const filteredBreaks = breaks.filter((b) => {
    if (channelFilter !== 'ALL' && b.channel !== channelFilter) return false;
    if (statusFilter === 'UNMATCHED' && b.status !== 'UNMATCHED') return false;
    if (statusFilter === 'TOLERANCE' && b.status !== 'TOLERANCE_MATCHED') return false;
    if (statusFilter === 'PAIRED' && b.status !== 'MANUALLY_PAIRED') return false;
    if (categoryFilter !== 'ALL' && b.breakType !== categoryFilter) return false;

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const matchLedger =
        b.ledgerRecord?.reference.toLowerCase().includes(q) ||
        b.ledgerRecord?.accountNumber.toLowerCase().includes(q) ||
        b.ledgerRecord?.narration.toLowerCase().includes(q);
      const matchSwitch =
        b.switchRecord?.stan.toLowerCase().includes(q) ||
        b.switchRecord?.rrn.toLowerCase().includes(q) ||
        b.switchRecord?.switchPartner.toLowerCase().includes(q);
      const matchId = b.id.toLowerCase().includes(q) || b.settlementSession.toLowerCase().includes(q);
      return matchLedger || matchSwitch || matchId;
    }

    return true;
  });

  const unmatchedCount = breaks.filter((b) => b.status === 'UNMATCHED').length;
  const toleranceCount = breaks.filter((b) => b.status === 'TOLERANCE_MATCHED').length;
  const pairedCount = breaks.filter((b) => b.status === 'MANUALLY_PAIRED').length;

  return (
    <div className="space-y-6">
      {/* Top Header & Operational Actions */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-[17px] font-bold text-[var(--text-primary)]">
              Multi-Source Reconciliation & Settlement Workspace
            </h2>
            <span className="text-[11px] font-bold font-mono-financial px-2 py-0.5 rounded bg-blue-50 text-blue-800 border border-blue-200">
              CBN BPS/DIR/05 Compliant
            </span>
          </div>
          <p className="text-[13px] text-[var(--text-muted)] mt-0.5">
            Two-way and three-way settlement reconciliation matching Finacle & ALAT core ledgers
            against NIBSS, Interswitch, and Mastercard clearing files.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            leftIcon={<Sliders className="w-4 h-4 text-indigo-600" />}
            onClick={() => setIsToleranceModalOpen(true)}
          >
            Tolerance Rules (±₦{toleranceConfig.amountToleranceNgn.toFixed(2)})
          </Button>

          <Button
            variant="secondary"
            size="sm"
            leftIcon={<Download className="w-4 h-4" />}
            onClick={() => setIsExportModalOpen(true)}
          >
            Export Dispute File
          </Button>

          <Button
            variant="primary"
            size="sm"
            isLoading={isRunning}
            leftIcon={<RefreshCw className="w-4 h-4" />}
            onClick={handleTriggerRun}
          >
            Run Intake Cycle
          </Button>
        </div>
      </div>

      {/* Settlement Cycles & Channel Batch Monitor */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {cycles.map((cycle) => {
          const isSelected = cycle.id === selectedCycleId;
          const matchRate = (
            (cycle.matchedCount / cycle.totalLedgerRecords) *
            100
          ).toFixed(2);

          return (
            <div
              key={cycle.id}
              onClick={() => setSelectedCycleId(cycle.id)}
              className={`p-4 rounded-lg border cursor-pointer transition-all ${
                isSelected
                  ? 'bg-blue-50/20 border-blue-500 shadow-sm ring-1 ring-blue-500'
                  : 'bg-[var(--surface)] border-[var(--surface-border)] hover:border-slate-400'
              }`}
            >
              <div className="flex items-center justify-between">
                <span className="font-bold text-[14px] text-[var(--text-primary)] flex items-center gap-1.5">
                  <Layers className="w-4 h-4 text-indigo-600" />
                  <span>{cycle.channel} Clearing Cycle</span>
                </span>
                <span
                  className={`text-[10px] font-bold px-2 py-0.5 rounded font-mono-financial ${
                    cycle.status === 'BALANCED'
                      ? 'bg-emerald-50 text-emerald-900 border border-emerald-300'
                      : 'bg-amber-50 text-amber-900 border border-amber-300'
                  }`}
                >
                  {cycle.status.replace(/_/g, ' ')}
                </span>
              </div>

              <div className="mt-2 text-[11px] font-mono-financial text-[var(--text-muted)]">
                {cycle.sessionCode} • {cycle.cutOffWindow}
              </div>

              <div className="mt-3 flex items-baseline justify-between">
                <div>
                  <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-semibold">
                    Match Accuracy
                  </div>
                  <div className="text-[22px] font-bold font-mono-financial text-[var(--text-primary)]">
                    {matchRate}%
                  </div>
                </div>
                <div className="text-right">
                  <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-semibold">
                    Net Break Volume
                  </div>
                  <div className="text-[13px] font-bold font-mono-financial text-rose-700">
                    {cycle.netDiscrepancyAmount}
                  </div>
                </div>
              </div>

              <div className="border-t border-[var(--surface-border-subtle)] mt-3 pt-2 text-[11px] font-mono-financial flex justify-between text-[var(--text-secondary)]">
                <span>Total: {cycle.totalLedgerRecords.toLocaleString()}</span>
                <span className="text-emerald-700">Matched: {cycle.matchedCount.toLocaleString()}</span>
                <span className="text-rose-700 font-bold">Breaks: {cycle.unmatchedCount}</span>
              </div>
            </div>
          );
        })}
      </div>

      {/* Break Filter & Search Bar */}
      <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          {/* Status Tabs */}
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => setStatusFilter('ALL')}
              className={`px-3 py-1.5 rounded-md text-[12px] font-semibold transition-colors ${
                statusFilter === 'ALL'
                  ? 'bg-slate-900 text-white'
                  : 'bg-[var(--canvas-subdued)] text-[var(--text-secondary)] hover:bg-slate-200'
              }`}
            >
              All Breaks ({breaks.length})
            </button>
            <button
              onClick={() => setStatusFilter('UNMATCHED')}
              className={`px-3 py-1.5 rounded-md text-[12px] font-semibold transition-colors flex items-center gap-1.5 ${
                statusFilter === 'UNMATCHED'
                  ? 'bg-rose-700 text-white'
                  : 'bg-rose-50 text-rose-800 hover:bg-rose-100'
              }`}
            >
              <span>Action Required</span>
              <span className="px-1.5 py-0.2 rounded-full bg-rose-200 text-rose-950 text-[10px]">
                {unmatchedCount}
              </span>
            </button>
            <button
              onClick={() => setStatusFilter('TOLERANCE')}
              className={`px-3 py-1.5 rounded-md text-[12px] font-semibold transition-colors flex items-center gap-1.5 ${
                statusFilter === 'TOLERANCE'
                  ? 'bg-indigo-700 text-white'
                  : 'bg-indigo-50 text-indigo-800 hover:bg-indigo-100'
              }`}
            >
              <span>Tolerance Matched</span>
              <span className="px-1.5 py-0.2 rounded-full bg-indigo-200 text-indigo-950 text-[10px]">
                {toleranceCount}
              </span>
            </button>
            <button
              onClick={() => setStatusFilter('PAIRED')}
              className={`px-3 py-1.5 rounded-md text-[12px] font-semibold transition-colors flex items-center gap-1.5 ${
                statusFilter === 'PAIRED'
                  ? 'bg-emerald-700 text-white'
                  : 'bg-emerald-50 text-emerald-800 hover:bg-emerald-100'
              }`}
            >
              <span>Force Paired</span>
              <span className="px-1.5 py-0.2 rounded-full bg-emerald-200 text-emerald-950 text-[10px]">
                {pairedCount}
              </span>
            </button>
          </div>

          {/* Channel selector */}
          <div className="flex items-center gap-2 text-[12px]">
            <Filter className="w-3.5 h-3.5 text-[var(--text-muted)]" />
            <span className="text-[var(--text-muted)] font-medium">Channel:</span>
            <select
              value={channelFilter}
              onChange={(e) => setChannelFilter(e.target.value)}
              className="px-2.5 py-1 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)]"
            >
              <option value="ALL">All Channels</option>
              <option value="NIP">NIP (Instant Transfer)</option>
              <option value="POS">POS Terminal</option>
              <option value="CARD">Card / ATM</option>
            </select>
          </div>
        </div>

        {/* Search input & Category dropdown */}
        <div className="flex items-center gap-3 pt-2 border-t border-[var(--surface-border-subtle)]">
          <div className="relative flex-1">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filter breaks by reference, STAN, RRN, account, or narration..."
              className="w-full pl-9 pr-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
          </div>

          <select
            value={categoryFilter}
            onChange={(e) => setCategoryFilter(e.target.value as BreakCategory)}
            className="px-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] text-[var(--text-primary)]"
          >
            <option value="ALL">All Break Typologies</option>
            <option value="LEDGER_DEBIT_NO_SWITCH">Ledger Debit, No Switch Match</option>
            <option value="SWITCH_RECORD_NO_LEDGER">Ghost Switch Settlement</option>
            <option value="AMOUNT_MISMATCH">Amount Variance / Fee Discrepancy</option>
            <option value="TIMING_CUTOFF_BREAK">Timing Window Cutoff Straddle</option>
          </select>
        </div>
      </div>

      {/* Dual-Sided Settlement Break Grid */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs overflow-hidden">
        <div className="px-5 py-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileSpreadsheet className="w-4 h-4 text-indigo-600" />
            <span className="font-bold text-[13px] text-[var(--text-primary)]">
              Discrepancy Break Matrix — Dual-Sided Comparison
            </span>
          </div>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            Showing {filteredBreaks.length} of {breaks.length} exception records
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px] font-mono-financial">
            <thead>
              <tr className="border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
                <th className="py-2.5 px-4">Break & Severity</th>
                <th className="py-2.5 px-4">Core Ledger Debit Record</th>
                <th className="py-2.5 px-4 text-center">Variance Delta</th>
                <th className="py-2.5 px-4">Switch Clearing Record</th>
                <th className="py-2.5 px-4">Status & Resolution</th>
                <th className="py-2.5 px-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--surface-border-subtle)]">
              {filteredBreaks.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-[var(--text-muted)] font-sans">
                    No reconciliation breaks match your search and filter criteria.
                  </td>
                </tr>
              ) : (
                filteredBreaks.map((b) => (
                  <tr
                    key={b.id}
                    className="hover:bg-[var(--canvas-subdued)] transition-colors group cursor-pointer"
                    onClick={() => handleOpenBreakDetail(b)}
                  >
                    {/* Break Meta */}
                    <td className="py-3 px-4 space-y-1 align-top">
                      <div className="flex items-center gap-1.5">
                        <span
                          className={`text-[9px] font-bold px-1.5 py-0.2 rounded ${
                            b.severity === 'CRITICAL'
                              ? 'bg-rose-100 text-rose-800'
                              : b.severity === 'HIGH'
                              ? 'bg-amber-100 text-amber-800'
                              : 'bg-blue-100 text-blue-800'
                          }`}
                        >
                          {b.severity}
                        </span>
                        <span className="text-[11px] font-bold text-[var(--text-primary)]">
                          {b.id}
                        </span>
                      </div>
                      <div className="text-[11px] font-sans font-medium text-[var(--text-primary)] leading-tight">
                        {b.breakLabel}
                      </div>
                      <div className="text-[10px] text-[var(--text-muted)]">{b.settlementSession}</div>
                    </td>

                    {/* Core Ledger Record */}
                    <td className="py-3 px-4 space-y-0.5 align-top">
                      {b.ledgerRecord ? (
                        <div>
                          <div className="flex items-center gap-1 font-sans font-bold text-[11px] text-blue-700">
                            <Building className="w-3 h-3" />
                            <span>{b.ledgerRecord.coreSystem}</span>
                          </div>
                          <div className="text-[var(--text-primary)] font-bold text-[13px]">
                            ₦{b.ledgerRecord.amount}
                          </div>
                          <div className="text-[10px] text-[var(--text-muted)]">
                            Ref: {b.ledgerRecord.reference}
                          </div>
                          <div className="text-[10px] text-[var(--text-muted)]">
                            Acct: {b.ledgerRecord.accountNumber}
                          </div>
                        </div>
                      ) : (
                        <div className="p-2 bg-rose-50 border border-rose-200 rounded text-rose-800 text-[10px] font-sans">
                          No debit posted on core ledger
                        </div>
                      )}
                    </td>

                    {/* Variance Delta */}
                    <td className="py-3 px-4 text-center align-middle font-bold">
                      <div
                        className={`inline-block px-2 py-1 rounded text-[12px] ${
                          b.amountDifference > 0
                            ? 'bg-rose-50 text-rose-700 border border-rose-200'
                            : 'bg-emerald-50 text-emerald-800 border border-emerald-200'
                        }`}
                      >
                        {b.amountDifference > 0 ? (
                          <>Δ ₦{b.amountDifference.toFixed(2)}</>
                        ) : (
                          <>Balanced (₦0.00)</>
                        )}
                      </div>
                      {b.timeDifferenceSeconds > 0 && (
                        <div className="text-[9px] text-[var(--text-muted)] mt-1 font-sans flex items-center justify-center gap-0.5">
                          <Clock className="w-2.5 h-2.5" />
                          <span>{b.timeDifferenceSeconds}s drift</span>
                        </div>
                      )}
                    </td>

                    {/* Switch Record */}
                    <td className="py-3 px-4 space-y-0.5 align-top">
                      {b.switchRecord ? (
                        <div>
                          <div className="flex items-center gap-1 font-sans font-bold text-[11px] text-purple-700">
                            <Server className="w-3 h-3" />
                            <span>{b.switchRecord.switchPartner}</span>
                          </div>
                          <div className="text-[var(--text-primary)] font-bold text-[13px]">
                            ₦{b.switchRecord.clearingAmount}
                          </div>
                          <div className="text-[10px] text-[var(--text-muted)]">
                            STAN: {b.switchRecord.stan} | RRN: {b.switchRecord.rrn}
                          </div>
                          <div className="text-[10px] text-amber-800 font-bold">
                            {b.switchRecord.switchResponseCode}
                          </div>
                        </div>
                      ) : (
                        <div className="p-2 bg-rose-50 border border-rose-200 rounded text-rose-800 text-[10px] font-sans">
                          Missing from switch clearing file
                        </div>
                      )}
                    </td>

                    {/* Status & Resolution */}
                    <td className="py-3 px-4 space-y-1 align-top">
                      <span
                        className={`inline-block text-[10px] font-bold px-2 py-0.5 rounded ${
                          b.status === 'UNMATCHED'
                            ? 'bg-rose-100 text-rose-900 border border-rose-300'
                            : b.status === 'TOLERANCE_MATCHED'
                            ? 'bg-indigo-100 text-indigo-900 border border-indigo-300'
                            : b.status === 'MANUALLY_PAIRED'
                            ? 'bg-emerald-100 text-emerald-900 border border-emerald-300'
                            : 'bg-slate-100 text-slate-900'
                        }`}
                      >
                        {b.status.replace(/_/g, ' ')}
                      </span>

                      {b.toleranceRule && (
                        <div className="text-[10px] text-indigo-800 font-sans line-clamp-1">
                          {b.toleranceRule}
                        </div>
                      )}

                      {b.caseReferenceId && (
                        <div className="text-[10px] text-blue-700 font-sans font-medium flex items-center gap-1">
                          <ShieldAlert className="w-3 h-3" />
                          <span>Case {b.caseReferenceId}</span>
                        </div>
                      )}
                    </td>

                    {/* Action buttons */}
                    <td className="py-3 px-4 text-right align-top" onClick={(e) => e.stopPropagation()}>
                      <div className="flex items-center justify-end gap-1.5">
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => handleOpenBreakDetail(b)}
                        >
                          Inspect
                        </Button>

                        {b.status === 'UNMATCHED' && (
                          <Button
                            variant="secondary"
                            size="xs"
                            onClick={() => {
                              setSelectedBreak(b);
                              setIsPairingModalOpen(true);
                            }}
                            leftIcon={<Link2 className="w-3 h-3" />}
                          >
                            Pair
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Simulator Fixture Ingestion Card */}
      <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-3">
        <h3 className="text-[14px] font-semibold text-[var(--text-primary)] flex items-center gap-2">
          <FileSpreadsheet className="w-4 h-4 text-emerald-600" />
          <span>Active Simulator Settlement Ingestion Fixtures</span>
        </h3>
        <p className="text-[12px] text-[var(--text-secondary)]">
          The engine continuously ingests settlement files from local repository fixtures:
        </p>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-[12px] font-mono-financial">
          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded space-y-1">
            <div className="font-semibold text-[var(--text-primary)]">sample_settlement_nip.csv</div>
            <div className="text-[var(--text-muted)]">NIP Settlement sessions NIP9901 - NIP9905</div>
            <div className="text-emerald-700 font-medium">✓ Ingested & Synced with Finacle Core 11</div>
          </div>

          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded space-y-1">
            <div className="font-semibold text-[var(--text-primary)]">sample_pos_batch.json</div>
            <div className="text-[var(--text-muted)]">Terminal TERM-8801, STAN 004402, Response 06</div>
            <div className="text-rose-700 font-medium">⚠ Timeout Mismatch Exception Active (Break 002)</div>
          </div>
        </div>
      </div>

      {/* Modals & Drawer */}
      <ToleranceSettingsModal
        isOpen={isToleranceModalOpen}
        onClose={() => setIsToleranceModalOpen(false)}
        config={toleranceConfig}
        onSave={(newCfg) => {
          setToleranceConfig(newCfg);
          addToast({
            type: 'success',
            title: 'Tolerance Config Updated',
            message: `Amount tolerance set to ±₦${newCfg.amountToleranceNgn.toFixed(2)}, window ${newCfg.timeWindowSeconds / 60}m.`,
          });
        }}
      />

      <ManualPairingModal
        isOpen={isPairingModalOpen}
        onClose={() => setIsPairingModalOpen(false)}
        reconciliationBreak={selectedBreak}
        onConfirmPair={handleConfirmManualPair}
      />

      <DisputeExportModal
        isOpen={isExportModalOpen}
        onClose={() => setIsExportModalOpen(false)}
        breaks={breaks}
        sessionCode={activeCycle.sessionCode}
      />

      <BreakDetailDrawer
        isOpen={isDrawerOpen}
        onClose={() => setIsDrawerOpen(false)}
        reconciliationBreak={selectedBreak}
        onOpenPairingModal={() => {
          setIsDrawerOpen(false);
          setIsPairingModalOpen(true);
        }}
        onApplyTolerance={handleApplyTolerance}
        onEscalateInvestigation={handleEscalateInvestigation}
      />
    </div>
  );
};
