import React, { useState } from 'react';
import { X, UserCheck, Users } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ExceptionTeam } from '../../types/domain';

interface AssignModalProps {
  isOpen: boolean;
  exceptionIds: string[];
  currentAssignee?: string;
  currentTeam?: ExceptionTeam;
  onClose: () => void;
  onAssign: (analystId: string, analystName: string, team: ExceptionTeam) => Promise<void>;
}

const ANALYSTS = [
  { id: 'analyst.maker@wema.ng', name: 'Olusola Adeyemi (Maker)', role: 'Tier-2 Recon' },
  { id: 'analyst.amaka@wema.ng', name: 'Amaka Obi (Dispute Lead)', role: 'Dispute Resolution' },
  { id: 'analyst.chukwu@wema.ng', name: 'Chukwuma Eze (Fraud Ops)', role: 'Fraud Ops' },
  { id: 'analyst.triage@wema.ng', name: 'Zainab Bello (Triage)', role: 'Tier-1 Triage' },
  { id: 'supervisor.checker@wema.ng', name: 'Folake Balogun (Checker)', role: 'Supervisor' },
];

const TEAMS: Array<{ id: ExceptionTeam; label: string }> = [
  { id: 'TIER_1_TRIAGE', label: 'Tier-1 Automated Triage' },
  { id: 'TIER_2_RECON', label: 'Tier-2 Core Settlement Recon' },
  { id: 'DISPUTE_RESOLUTION', label: 'Customer Dispute Resolution' },
  { id: 'FRAUD_OPS', label: 'Fraud & Financial Crimes Ops' },
];

export const AssignModal: React.FC<AssignModalProps> = ({
  isOpen,
  exceptionIds,
  currentAssignee,
  currentTeam = 'TIER_2_RECON',
  onClose,
  onAssign,
}) => {
  const [selectedAnalyst, setSelectedAnalyst] = useState<string>(
    currentAssignee || ANALYSTS[0].id
  );
  const [selectedTeam, setSelectedTeam] = useState<ExceptionTeam>(currentTeam);
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const isBulk = exceptionIds.length > 1;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    const analyst = ANALYSTS.find((a) => a.id === selectedAnalyst) || ANALYSTS[0];
    try {
      await onAssign(analyst.id, analyst.name, selectedTeam);
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="assign-modal-title"
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-xs animate-in fade-in duration-150"
    >
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl max-w-md w-full overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            {isBulk ? (
              <Users className="w-4 h-4 text-sky-600" />
            ) : (
              <UserCheck className="w-4 h-4 text-sky-600" />
            )}
            <h3 id="assign-modal-title" className="text-[14px] font-bold text-[var(--text-primary)]">
              {isBulk
                ? `Bulk Assign ${exceptionIds.length} Exceptions`
                : 'Assign / Reassign Exception'}
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            aria-label="Close assign dialog"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4 text-[13px]">
          <div>
            <label
              htmlFor="analyst-select"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1.5"
            >
              Assignee Analyst
            </label>
            <select
              id="analyst-select"
              value={selectedAnalyst}
              onChange={(e) => setSelectedAnalyst(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            >
              {ANALYSTS.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} ({a.role})
                </option>
              ))}
            </select>
          </div>

          <div>
            <label
              htmlFor="team-select"
              className="block text-[12px] font-semibold text-[var(--text-secondary)] mb-1.5"
            >
              Operational Team / Queue
            </label>
            <select
              id="team-select"
              value={selectedTeam}
              onChange={(e) => setSelectedTeam(e.target.value as ExceptionTeam)}
              className="w-full px-3 py-2 bg-[var(--surface)] border border-[var(--surface-border)] rounded text-[13px] text-[var(--text-primary)] focus:outline-none focus:ring-1 focus:ring-sky-500"
            >
              {TEAMS.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex items-center justify-end gap-2 pt-3 border-t border-[var(--surface-border-subtle)]">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onClose}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              size="sm"
              isLoading={isSubmitting}
            >
              {isBulk ? 'Confirm Bulk Assignment' : 'Confirm Assignment'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};
