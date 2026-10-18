import React, { useState } from 'react';
import { Clock, FileText, Check, Shield, ChevronRight, Search, Mic, ArrowLeft } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ReverseItStep } from './reverseitTypes';

export interface ReverseItHeaderProps {
  currentStep: ReverseItStep;
  activeCaseCount: number;
  onNavigateToMyCases: () => void;
  onStartNewReport: () => void;
  onQuickTrack?: (query: string) => void;
  onOpenVoiceModal?: () => void;
  onBack?: () => void;
}

interface StepMilestone {
  key: string;
  label: string;
  steps: ReverseItStep[];
}

const MILESTONES: StepMilestone[] = [
  { key: 'report', label: '1. Report', steps: ['S1_REPORT_PROBLEM'] },
  { key: 'alert', label: '2. Alert Info', steps: ['S2_PASTE_UPLOAD'] },
  { key: 'verify', label: '3. Verify', steps: ['S3_CHECK_DETAILS'] },
  { key: 'match', label: '4. Match', steps: ['S4_MATCH_CANDIDATES', 'S5_WHAT_WENT_WRONG'] },
  { key: 'resolution', label: '5. Resolution', steps: ['S6_YOUR_DEADLINE', 'S7_REVIEW_SEND', 'S7_CONFIRMATION'] },
];

export const ReverseItHeader: React.FC<ReverseItHeaderProps> = ({
  currentStep,
  activeCaseCount,
  onNavigateToMyCases,
  onStartNewReport,
  onQuickTrack,
  onOpenVoiceModal,
  onBack,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const isMyCases = currentStep === 'MY_CASES';
  const isTracker = currentStep === 'S8_CASE_TRACKER' || currentStep === 'S9_ESCALATION_LADDER';
  const isWizard = !isMyCases && !isTracker;

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchQuery.trim() && onQuickTrack) {
      onQuickTrack(searchQuery);
    }
  };

  // Determine current milestone index
  const currentMilestoneIndex = MILESTONES.findIndex((m) =>
    m.steps.includes(currentStep)
  );
  const activeIndex = currentMilestoneIndex >= 0 ? currentMilestoneIndex : 0;
  const progressPercent = Math.round(((activeIndex + 1) / MILESTONES.length) * 100);

  return (
    <div className="space-y-4">
      {/* Top action row */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 pb-2.5 border-b border-[var(--surface-border-subtle)]">
        {/* Breadcrumb row with Back Button & Home Link */}
        <div className="flex items-center gap-1.5 sm:gap-2 text-[12px] sm:text-[13px] min-w-0">
          {onBack && currentStep !== 'S1_REPORT_PROBLEM' && (
            <button
              type="button"
              onClick={onBack}
              className="inline-flex items-center gap-1 px-2 py-1 text-[11px] font-semibold rounded-lg bg-[var(--canvas-subdued)] hover:bg-[var(--surface-border-subtle)] border border-[var(--surface-border)] text-[var(--text-primary)] hover:text-[var(--primary)] hover:border-[var(--primary)] transition-all cursor-pointer shrink-0"
              title="Go back to previous step"
              aria-label="Back to previous step"
            >
              <ArrowLeft className="w-3.5 h-3.5 shrink-0" />
              <span>Back</span>
            </button>
          )}

          <button
            type="button"
            onClick={onStartNewReport}
            className="font-bold text-[var(--text-primary)] hover:text-[var(--primary)] cursor-pointer flex items-center gap-1 transition-colors shrink-0"
            title="Go to Dispute Center Home"
          >
            <span className="hidden sm:inline">Dispute Resolution</span>
            <span className="sm:hidden">Dispute</span> Center
          </button>
          <ChevronRight className="w-3.5 h-3.5 text-[var(--text-muted)] shrink-0" />
          <span className="text-[var(--text-secondary)] font-medium truncate">
            {isMyCases
              ? 'My Open Cases'
              : isTracker
              ? 'Live Case Tracker'
              : `Step ${activeIndex + 1} of 5: ${MILESTONES[activeIndex].label}`}
          </span>
        </div>

        {/* Quick Search & Actions */}
        <div className="flex items-center gap-2 w-full sm:w-auto">
          {/* Quick Track Input */}
          {onQuickTrack && (
            <form onSubmit={handleSearchSubmit} className="relative flex-1 sm:w-44 sm:flex-initial min-w-0">
              <input
                type="text"
                placeholder="Track Ref / RRN..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-7 pr-2.5 py-1.5 text-[11px] font-mono rounded-xl bg-[var(--canvas-subdued)] border border-[var(--surface-border)] text-[var(--text-primary)] focus:outline-hidden focus:ring-1 focus:ring-sky-500 transition-all placeholder:text-[var(--text-muted)] placeholder:font-sans"
              />
              <Search className="w-3.5 h-3.5 text-[var(--text-muted)] absolute left-2 top-1/2 -translate-y-1/2 pointer-events-none" />
            </form>
          )}

          {onOpenVoiceModal && (
            <Button
              variant="secondary"
              size="sm"
              onClick={onOpenVoiceModal}
              className="shrink-0 px-2.5 sm:px-3 text-[11px] sm:text-xs"
              leftIcon={<Mic className="w-3.5 h-3.5 text-emerald-600 animate-pulse shrink-0" />}
            >
              <span className="whitespace-nowrap">Voice</span>
              <span className="hidden sm:inline ml-0.5">Dispute</span>
              <span className="hidden lg:inline text-[10px] text-emerald-600 font-bold ml-1">
                (Ohùn / Hausa)
              </span>
            </Button>
          )}

          {isMyCases || isTracker ? (
            <Button
              variant="primary"
              size="sm"
              onClick={onStartNewReport}
              className="shrink-0 px-2.5 sm:px-3 text-[11px] sm:text-xs whitespace-nowrap"
              leftIcon={<FileText className="w-3.5 h-3.5 shrink-0" />}
            >
              <span>New Dispute</span>
            </Button>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              onClick={onNavigateToMyCases}
              className="shrink-0 px-2.5 sm:px-3 text-[11px] sm:text-xs whitespace-nowrap"
              leftIcon={<Clock className="w-3.5 h-3.5 shrink-0" />}
            >
              <span>My Cases</span>
              {activeCaseCount > 0 && (
                <span className="ml-1.5 px-1.5 py-0.2 rounded-full text-[10px] font-bold bg-sky-600 text-white shrink-0">
                  {activeCaseCount}
                </span>
              )}
            </Button>
          )}
        </div>
      </div>

      {/* Interactive Progress Stepper for Wizard Flow */}
      {isWizard && (
        <div className="p-3.5 sm:p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-2xl shadow-xs space-y-3">
          <div className="flex items-center justify-between text-[12px] gap-2">
            <div className="flex items-center gap-1.5 sm:gap-2 shrink-0">
              <span className="font-bold text-[var(--text-primary)] text-[12px] sm:text-[13px]">Dispute Progress</span>
              <span className="text-[10px] sm:text-[11px] px-2 py-0.5 rounded-full bg-sky-50 text-sky-800 dark:bg-sky-950/60 dark:text-sky-300 dark:border-sky-800 font-semibold border border-sky-200 shrink-0">
                {progressPercent}% Complete
              </span>
            </div>
            <div className="flex items-center gap-1 sm:gap-1.5 text-[10px] sm:text-[11px] text-[var(--text-muted)] truncate justify-end">
              <Shield className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />
              <span className="hidden sm:inline truncate">CBN Resolution Guarantee Active</span>
              <span className="sm:hidden font-medium text-emerald-700 dark:text-emerald-400 truncate">CBN Guarantee</span>
            </div>
          </div>

          {/* Stepper Milestones */}
          <div className="hidden sm:grid grid-cols-5 gap-2 pt-1">
            {MILESTONES.map((m, idx) => {
              const isPast = idx < activeIndex;
              const isCurrent = idx === activeIndex;

              return (
                <div key={m.key} className="space-y-1.5">
                  <div className="flex items-center gap-2">
                    <div
                      className={`w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-bold transition-all ${
                        isPast
                          ? 'bg-emerald-600 text-white ring-2 ring-emerald-500/20'
                          : isCurrent
                          ? 'bg-sky-600 text-white shadow-xs ring-2 ring-sky-500/30'
                          : 'bg-[var(--canvas-subdued)] text-[var(--text-muted)] border border-[var(--surface-border)]'
                      }`}
                    >
                      {isPast ? <Check className="w-3.5 h-3.5 stroke-[3]" /> : idx + 1}
                    </div>
                    <span
                      className={`text-[12px] font-medium truncate ${
                        isCurrent
                          ? 'font-bold text-sky-700'
                          : isPast
                          ? 'text-[var(--text-primary)]'
                          : 'text-[var(--text-muted)]'
                      }`}
                    >
                      {m.label.replace(/^\d+\.\s*/, '')}
                    </span>
                  </div>
                  {/* Mini Progress Bar Line */}
                  <div className="h-1 rounded-full bg-[var(--canvas-subdued)] overflow-hidden">
                    <div
                      className={`h-full transition-all duration-300 ${
                        isPast ? 'bg-emerald-500 w-full' : isCurrent ? 'bg-sky-500 w-1/2' : 'w-0'
                      }`}
                    />
                  </div>
                </div>
              );
            })}
          </div>

          {/* Mobile Linear Progress Bar */}
          <div className="sm:hidden space-y-1">
            <div className="flex justify-between text-[11px] text-[var(--text-muted)]">
              <span className="font-semibold text-[var(--text-primary)]">
                {MILESTONES[activeIndex].label}
              </span>
              <span>{progressPercent}%</span>
            </div>
            <div className="h-2 rounded-full bg-[var(--canvas-subdued)] overflow-hidden">
              <div
                className="h-full bg-sky-600 rounded-full transition-all duration-300"
                style={{ width: `${progressPercent}%` }}
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
