import React, { useState } from 'react';
import {
  ShieldCheck,
  ShieldAlert,
  Search,
  Filter,
  FileText,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../components/Button';
import { StatusBadge } from '../components/StatusBadge';
import { ApprovalDialog } from '../components/ApprovalDialog';
import { IrreversibleConfirmDialog } from '../components/IrreversibleConfirmDialog';
import { ReversalDetailDrawer } from './reversals/ReversalDetailDrawer';
import { useApp } from '../context/AppContext';
import { apiClient } from '../services/apiClient';
import { INITIAL_REVERSALS } from '../services/mockSimulatorData';
import type { ReversalProposal, ReversalDetailRecord } from '../types/domain';

type StatusFilter = 'ALL' | 'approval' | 'resolved' | 'failed';

export const ReversalsView: React.FC = () => {
  const { role, addToast } = useApp();
  const [reversals, setReversals] = useState<ReversalProposal[]>(() => INITIAL_REVERSALS);
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('ALL');

  // Screen 8 Detail Drawer state
  const [selectedDetail, setSelectedDetail] = useState<ReversalDetailRecord | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);

  // Screen 11 Approval Dialog state
  const [selectedProposal, setSelectedProposal] = useState<ReversalProposal | null>(null);
  const [isApprovalOpen, setIsApprovalOpen] = useState(false);
  const [isIrreversibleOpen, setIsIrreversibleOpen] = useState(false);
  const [pendingAction, setPendingAction] = useState<{ id: string; decision: 'APPROVE' | 'REJECT'; notes?: string } | null>(null);

  const fetchReversals = async () => {
    const list = await apiClient.getReversals();
    setReversals(list);
  };

  const handleOpenDetail = async (proposal: ReversalProposal) => {
    const detail = await apiClient.getReversalDetail(proposal.id);
    if (detail) {
      setSelectedDetail(detail);
      setSelectedProposal(proposal);
      setIsDetailOpen(true);
    } else {
      setSelectedProposal(proposal);
      setIsApprovalOpen(true);
    }
  };

  const handleOpenReview = (proposal: ReversalProposal) => {
    setSelectedProposal(proposal);
    setIsApprovalOpen(true);
  };

  const handleAuthorizeStep = async (proposalId: string, decision: 'APPROVE' | 'REJECT', notes?: string) => {
    if (decision === 'APPROVE') {
      // Require irreversible confirmation dialog
      setPendingAction({ id: proposalId, decision, notes });
      setIsIrreversibleOpen(true);
    } else {
      await apiClient.authorizeReversal({
        reversalId: proposalId,
        authorizedBy: `Supervisor (${role})`,
        decision: 'REJECT',
        notes,
      });
      addToast({
        type: 'warning',
        title: 'Reversal Proposal Rejected',
        message: 'The proposed reversal was rejected and logged to the audit trail.',
      });
      setIsDetailOpen(false);
      fetchReversals();
    }
  };

  const handleFinalIrreversibleConfirm = async () => {
    if (!pendingAction) return;
    await apiClient.authorizeReversal({
      reversalId: pendingAction.id,
      authorizedBy: `Supervisor (${role})`,
      decision: 'APPROVE',
      notes: pendingAction.notes,
    });
    addToast({
      type: 'success',
      title: 'Reversal Dispatched & Settled',
      message: 'Dual-control authorization complete. Reversal payload dispatched to Core Switch.',
    });
    setPendingAction(null);
    setIsDetailOpen(false);
    fetchReversals();
  };

  const isChecker = role === 'SUPERVISOR_CHECKER' || role === 'ADMIN';

  // Filtered proposals
  const filteredReversals = reversals.filter((r) => {
    const matchesStatus = statusFilter === 'ALL' || r.status === statusFilter;
    const matchesSearch =
      !searchQuery ||
      r.reference.toLowerCase().includes(searchQuery.toLowerCase()) ||
      r.reasonCode.toLowerCase().includes(searchQuery.toLowerCase()) ||
      r.destinationAccount.includes(searchQuery) ||
      r.proposedBy.toLowerCase().includes(searchQuery.toLowerCase());
    return matchesStatus && matchesSearch;
  });

  return (
    <div className="space-y-5">
      {/* 1. Header Info Bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xs">
        <div>
          <h2 className="text-[16px] font-bold text-[var(--text-primary)] flex items-center gap-2">
            <ShieldAlert className="w-5 h-5 text-purple-700" />
            <span>Reversal Queue, Detail and Approval</span>
          </h2>
          <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
            Strict dual-authorization protocol: No single operator can both propose and authorize a financial reversal. 8-point safety gate enforced.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="text-[12px] px-3 py-1 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded font-mono-financial">
            Active Role: <strong className="text-[var(--text-primary)]">{role}</strong>
          </div>
        </div>
      </div>

      {/* 2. Screen 7: Reversal Queue with Filters */}
      <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
        {/* Controls Bar */}
        <div className="p-3 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-3 text-[12px]">
          {/* Status Tabs */}
          <div className="flex items-center gap-1">
            <span className="font-bold text-[var(--text-muted)] uppercase tracking-wider text-[11px] mr-1 flex items-center gap-1">
              <Filter className="w-3.5 h-3.5" /> Queue Status:
            </span>
            <button
              type="button"
              onClick={() => setStatusFilter('ALL')}
              className={clsx(
                'px-2.5 py-1 rounded font-semibold cursor-pointer transition-colors',
                statusFilter === 'ALL'
                  ? 'bg-purple-700 text-white shadow-xs'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border border-[var(--surface-border)] hover:bg-[var(--surface-raised)]'
              )}
            >
              All ({reversals.length})
            </button>
            <button
              type="button"
              onClick={() => setStatusFilter('approval')}
              className={clsx(
                'px-2.5 py-1 rounded font-semibold cursor-pointer transition-colors',
                statusFilter === 'approval'
                  ? 'bg-purple-700 text-white shadow-xs'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border border-[var(--surface-border)] hover:bg-[var(--surface-raised)]'
              )}
            >
              Pending Approval ({reversals.filter((r) => r.status === 'approval').length})
            </button>
            <button
              type="button"
              onClick={() => setStatusFilter('resolved')}
              className={clsx(
                'px-2.5 py-1 rounded font-semibold cursor-pointer transition-colors',
                statusFilter === 'resolved'
                  ? 'bg-purple-700 text-white shadow-xs'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border border-[var(--surface-border)] hover:bg-[var(--surface-raised)]'
              )}
            >
              Settled / Dispatched ({reversals.filter((r) => r.status === 'resolved').length})
            </button>
            <button
              type="button"
              onClick={() => setStatusFilter('failed')}
              className={clsx(
                'px-2.5 py-1 rounded font-semibold cursor-pointer transition-colors',
                statusFilter === 'failed'
                  ? 'bg-purple-700 text-white shadow-xs'
                  : 'bg-[var(--surface)] text-[var(--text-secondary)] border border-[var(--surface-border)] hover:bg-[var(--surface-raised)]'
              )}
            >
              Rejected ({reversals.filter((r) => r.status === 'failed').length})
            </button>
          </div>

          {/* Search Input */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-[var(--text-muted)] absolute left-2.5 top-2.5" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filter by ref, reason, account..."
              className="pl-8 pr-3 py-1.5 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[12px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-purple-500 w-64"
            />
          </div>
        </div>

        {/* Proposals List */}
        <div className="divide-y divide-[var(--surface-border-subtle)]">
          {filteredReversals.length === 0 ? (
            <div className="p-8 text-center text-[var(--text-muted)] text-[13px]">
              No reversal proposals match the selected filters.
            </div>
          ) : (
            filteredReversals.map((rev) => {
              const isPending = rev.status === 'approval';
              return (
                <div
                  key={rev.id}
                  className="p-4 hover:bg-[var(--surface-raised)] transition-colors flex flex-wrap items-center justify-between gap-4 text-[13px]"
                >
                  <div className="space-y-1.5 max-w-xl">
                    <div className="flex items-center gap-2">
                      <strong className="font-mono-financial text-[14px] text-[var(--text-primary)]">
                        {rev.reference}
                      </strong>
                      <span className="text-[11px] font-mono-financial px-1.5 py-0.2 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)]">
                        {rev.channel}
                      </span>
                      <StatusBadge status={rev.status} size="sm" />
                    </div>

                    <p className="text-[12px] text-[var(--text-secondary)] leading-relaxed">
                      Reason: <strong className="text-purple-900 font-mono-financial">{rev.reasonCode}</strong> &bull; &ldquo;{rev.justification}&rdquo;
                    </p>

                    <div className="text-[11px] text-[var(--text-muted)] font-mono-financial flex flex-wrap items-center gap-3">
                      <span>Maker: {rev.proposedBy}</span>
                      <span>&bull; At: {new Date(rev.proposedAt).toLocaleTimeString()}</span>
                      <span>&bull; Idempotency: {rev.idempotencyKey.substring(0, 16)}...</span>
                      <span>&bull; Destination: {rev.destinationAccount} ({rev.destinationBank})</span>
                    </div>
                  </div>

                  <div className="flex items-center gap-4">
                    <div className="text-right">
                      <div className="text-[11px] text-[var(--text-muted)] font-medium">Reversal Exposure</div>
                      <div className="text-[18px] font-bold font-mono-financial text-[var(--text-primary)]">
                        {rev.currency} {Number(rev.reversalAmount).toLocaleString('en-US', { minimumFractionDigits: 2 })}
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => handleOpenDetail(rev)}
                        leftIcon={<FileText className="w-3.5 h-3.5" />}
                      >
                        View Dossier
                      </Button>

                      {isPending ? (
                        <Button
                          variant={isChecker ? 'primary' : 'secondary'}
                          size="sm"
                          onClick={() => handleOpenReview(rev)}
                          leftIcon={<ShieldCheck className="w-3.5 h-3.5" />}
                        >
                          {isChecker ? 'Authorize Reversal' : 'Review Proposal'}
                        </Button>
                      ) : (
                        <span className="px-2.5 py-1 text-[11px] font-semibold rounded bg-emerald-50 text-emerald-900 border border-emerald-300 font-mono-financial">
                          Executed by {rev.authorizedBy?.split(' ')[0]}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* Screen 8: Reversal Detail Drawer with 8 Safety Checks */}
      <ReversalDetailDrawer
        detail={selectedDetail}
        isOpen={isDetailOpen}
        onClose={() => setIsDetailOpen(false)}
        onProceedToAuthorize={() => {
          setIsDetailOpen(false);
          setIsApprovalOpen(true);
        }}
        isChecker={isChecker}
      />

      {/* Screen 11: Maker-Checker Review Dialog */}
      <ApprovalDialog
        proposal={selectedProposal}
        isOpen={isApprovalOpen}
        onClose={() => setIsApprovalOpen(false)}
        onAuthorize={handleAuthorizeStep}
      />

      {/* Irreversible Confirm Dialog */}
      <IrreversibleConfirmDialog
        isOpen={isIrreversibleOpen}
        onClose={() => setIsIrreversibleOpen(false)}
        onConfirm={handleFinalIrreversibleConfirm}
        title="High-Stakes Financial Reversal Dispatch"
        description="Executing this transaction will permanently credit the originator account and post an irreversible reversal journal entry to Core Banking Switch ledger. This action cannot be undone."
        confirmKeyword="CONFIRM REVERSAL"
        actionButtonText="Permanently Execute Reversal"
      />
    </div>
  );
};
