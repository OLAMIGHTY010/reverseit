import React, { useState } from 'react';
import {
  Headset,
  ShieldCheck,
  AlertOctagon,
  RefreshCw,
  Search,
  Lock,
} from 'lucide-react';
import { Button } from '../components/Button';
import { AgentSearchPanel } from './agent/AgentSearchPanel';
import { AgentApprovedWordingCard } from './agent/AgentApprovedWordingCard';
import { AgentActiveExceptionCard } from './agent/AgentActiveExceptionCard';
import { AgentVerifiedSuccessCard } from './agent/AgentVerifiedSuccessCard';
import { AgentDisambiguationCard } from './agent/AgentDisambiguationCard';
import { AgentDisputeIntakeForm } from './agent/AgentDisputeIntakeForm';
import { useApp } from '../context/AppContext';
import { apiClient } from '../services/apiClient';
import type { CustomerIntakeQuery, AgentQueryResult, Transaction } from '../types/domain';

export const AgentIntakeView: React.FC = () => {
  const { role, addToast } = useApp();

  const [currentQuery, setCurrentQuery] = useState<CustomerIntakeQuery | null>(null);
  const [result, setResult] = useState<AgentQueryResult | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isSimulatePermissionDenied, setIsSimulatePermissionDenied] = useState(false);

  // Initial auto-query with NIP-9901 for quick first impression
  const handleExecuteSearch = async (query: CustomerIntakeQuery) => {
    setCurrentQuery(query);
    setIsLoading(true);

    try {
      // Simulate real-time switch query latency
      const res = await apiClient.searchCustomerQuery(query, role);
      setResult(res);
    } catch {
      addToast({
        type: 'error',
        title: 'Query Failed',
        message: 'Could not contact switch search journal. Please retry.',
      });
    } finally {
      setIsLoading(false);
    }
  };

  const handleSelectDisambiguatedCandidate = async (txn: Transaction) => {
    // When agent selects a disambiguated candidate, drill into that reference
    await handleExecuteSearch({
      queryType: 'REFERENCE',
      reference: txn.reference,
    });
  };

  // Permission check
  const isAgentPermitted =
    !isSimulatePermissionDenied &&
    (role === 'CONTACT_CENTRE_AGENT' ||
      role === 'BRANCH_AGENT' ||
      role === 'ANALYST_MAKER' ||
      role === 'SUPERVISOR_CHECKER' ||
      role === 'ADMIN');

  if (!isAgentPermitted) {
    return (
      <div className="p-8 text-center bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xs space-y-4 max-w-lg mx-auto mt-12">
        <Lock className="w-12 h-12 text-rose-600 mx-auto" />
        <h3 className="text-[17px] font-bold text-[var(--text-primary)]">
          Permission Denied: Agent View Restricted
        </h3>
        <p className="text-[13px] text-[var(--text-secondary)] leading-relaxed">
          Your current persona (<strong>{role}</strong>) does not have authorization to access first-contact customer intake and caller quote scripts.
        </p>
        <div className="pt-2">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => setIsSimulatePermissionDenied(false)}
          >
            Reset Permission State
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* 1. Header Information & Agent Identity */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xs">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded bg-sky-100 text-sky-800">
            <Headset className="w-6 h-6" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-[16px] font-bold text-[var(--text-primary)]">
                First-Contact Agent Intake & Query Resolution Panel
              </h2>
              <span className="text-[11px] font-mono-financial font-semibold px-2 py-0.5 rounded bg-sky-100 text-sky-900 border border-sky-300">
                Contact-Centre / Branch
              </span>
            </div>
            <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
              Rapid customer query resolution. Masked financial attributes, audited data reveals, and strict compliance-approved wording.
            </p>
          </div>
        </div>

        {/* Persona & Test Simulators */}
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setIsSimulatePermissionDenied((prev) => !prev)}
            className="text-[11px] px-2 py-1 bg-[var(--canvas-subdued)] hover:bg-[var(--surface-raised)] border border-[var(--surface-border)] rounded text-[var(--text-secondary)] cursor-pointer"
            title="Toggle simulated permission denied state"
          >
            Simulate 403 Forbidden
          </button>

          <div className="text-[12px] px-3 py-1 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded font-mono-financial text-[var(--text-secondary)] flex items-center gap-1.5">
            <ShieldCheck className="w-3.5 h-3.5 text-sky-600" />
            <span>Active Agent: <strong className="text-[var(--text-primary)]">{role}</strong></span>
          </div>
        </div>
      </div>

      {/* 2. Identifier Search Panel */}
      <AgentSearchPanel onSearch={handleExecuteSearch} isLoading={isLoading} />

      {/* 3. Stale Data Warning Banner if applicable */}
      {result?.isSourceStale && (
        <div className="p-3 bg-amber-500/10 border border-amber-300 rounded text-[12px] text-amber-950 flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <AlertOctagon className="w-4 h-4 text-amber-600 shrink-0" />
            <span>
              {result.sourceStaleMessage || '⚠️ Switch reconciliation feed is currently degraded (>15m lag). Evidence may not reflect live status.'}
            </span>
          </div>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => currentQuery && handleExecuteSearch(currentQuery)}
            leftIcon={<RefreshCw className="w-3 h-3" />}
          >
            Refresh Feed
          </Button>
        </div>
      )}

      {/* 4. Loading Skeleton */}
      {isLoading && (
        <div className="p-8 border border-[var(--surface-border)] rounded-md bg-[var(--surface)] text-center space-y-3">
          <div className="inline-block animate-spin text-sky-600">
            <RefreshCw className="w-8 h-8" />
          </div>
          <h4 className="text-[14px] font-bold text-[var(--text-primary)]">
            Querying Live Switch Journals & Core Ledger...
          </h4>
          <p className="text-[12px] text-[var(--text-muted)] max-w-sm mx-auto">
            Matching transaction identifier across NIP, POS, Card gateway journals and active exception cases.
          </p>
        </div>
      )}

      {/* 5. Empty State (No query run yet) */}
      {!isLoading && !result && (
        <div className="p-8 border border-[var(--surface-border)] rounded-md bg-[var(--surface)] text-center space-y-3 text-[13px]">
          <Search className="w-10 h-10 text-[var(--text-muted)] mx-auto opacity-70" />
          <h4 className="font-bold text-[15px] text-[var(--text-primary)]">
            Awaiting Customer Query Search
          </h4>
          <p className="text-[12px] text-[var(--text-muted)] max-w-md mx-auto">
            Enter a transaction reference, session ID, RRN, STAN, date/amount, or customer phone/account above. Or select one of the rapid test presets.
          </p>
        </div>
      )}

      {/* 6. Active Query Result Displays */}
      {!isLoading && result && (
        <div className="space-y-5">
          {/* Approved Speech Wording Block (Always prominently anchored above/below result) */}
          <AgentApprovedWordingCard
            wording={result.approvedWording}
            canQuoteRefund={result.canQuoteRefund}
            stateLabel={
              result.state === 'MATCH_EXCEPTION'
                ? 'Active Recon Exception'
                : result.state === 'VERIFIED_SUCCESS'
                ? 'Verified Successful'
                : result.state === 'MULTIPLE_CANDIDATES'
                ? 'Ambiguous Disambiguation'
                : 'No Match Intake'
            }
          />

          {/* State A: Active Exception Match */}
          {result.state === 'MATCH_EXCEPTION' && (
            <AgentActiveExceptionCard
              result={result}
              onRefresh={() => currentQuery && handleExecuteSearch(currentQuery)}
            />
          )}

          {/* State B: Verified Successful */}
          {result.state === 'VERIFIED_SUCCESS' && (
            <AgentVerifiedSuccessCard result={result} />
          )}

          {/* State D: Multiple Candidates (Disambiguation) */}
          {result.state === 'MULTIPLE_CANDIDATES' && result.candidates && (
            <AgentDisambiguationCard
              candidates={result.candidates}
              onSelectCandidate={handleSelectDisambiguatedCandidate}
            />
          )}

          {/* State C: No Match Found (Customer Dispute Intake Form) */}
          {result.state === 'NO_MATCH' && (
            <AgentDisputeIntakeForm
              initialQuery={result.query}
              onCaseCreated={(_caseNum, ref) => {
                // After creating case, search it
                handleExecuteSearch({ queryType: 'REFERENCE', reference: ref });
              }}
            />
          )}
        </div>
      )}
    </div>
  );
};
