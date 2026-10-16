import React from 'react';
import { Clock, FileText, Info } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ReverseItStep } from './reverseitTypes';

export interface ReverseItHeaderProps {
  currentStep: ReverseItStep;
  activeCaseCount: number;
  onNavigateToMyCases: () => void;
  onStartNewReport: () => void;
}

export const ReverseItHeader: React.FC<ReverseItHeaderProps> = ({
  currentStep,
  activeCaseCount,
  onNavigateToMyCases,
  onStartNewReport,
}) => {
  const isMyCases = currentStep === 'MY_CASES';

  return (
    <div className="space-y-3">
      {/* Main ReverseIt Navigation Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xs">
        <div className="flex items-center gap-3">
          <img src="/logo.png" alt="ReverseIt Logo" className="w-9 h-9 object-contain" />
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-[18px] font-bold tracking-tight text-[var(--text-primary)]">
                ReverseIt
              </h1>
              <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-sky-100 text-sky-800 border border-sky-300">
                Customer Companion
              </span>
            </div>
            <p className="text-[12px] text-[var(--text-muted)]">
              Instant debit alert intake, CBN regulatory countdowns, and automated exception tracking.
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {isMyCases ? (
            <Button
              variant="primary"
              size="sm"
              onClick={onStartNewReport}
              leftIcon={<FileText className="w-3.5 h-3.5" />}
            >
              Report New Dispute
            </Button>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              onClick={onNavigateToMyCases}
              leftIcon={<Clock className="w-3.5 h-3.5" />}
            >
              My Cases ({activeCaseCount})
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};
