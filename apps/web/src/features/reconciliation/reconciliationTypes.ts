import type { Channel } from '../../types/domain';

export type BreakCategory =
  | 'ALL'
  | 'LEDGER_DEBIT_NO_SWITCH'
  | 'SWITCH_RECORD_NO_LEDGER'
  | 'AMOUNT_MISMATCH'
  | 'TIMING_CUTOFF_BREAK';

export type BreakStatus =
  | 'UNMATCHED'
  | 'TOLERANCE_MATCHED'
  | 'MANUALLY_PAIRED'
  | 'ESCALATED_DISPUTE'
  | 'SETTLED';

export interface LedgerRecord {
  txnId: string;
  reference: string;
  accountNumber: string;
  amount: string;
  currency: string;
  postedAt: string;
  postingStatus: 'POSTED' | 'PENDING' | 'REVERSED' | 'MISSING';
  narration: string;
  coreSystem: 'Finacle Core 11' | 'Flexcube' | 'ALAT Ledger';
}

export interface SwitchRecord {
  stan: string;
  rrn: string;
  terminalId?: string;
  sessionId: string;
  switchResponseCode: string;
  clearingAmount: string;
  currency: string;
  clearedAt: string;
  clearingStatus: 'CLEARED' | 'REJECTED' | 'TIMED_OUT' | 'MISSING';
  switchPartner:
    | 'NIBSS Instant Payment'
    | 'Interswitch Web'
    | 'Interswitch Web & POS'
    | 'Mastercard Clearing'
    | 'Mastercard Clearing System'
    | 'Visa Base II'
    | 'Unified Payments';
}

export interface ReconciliationBreak {
  id: string;
  sessionDate: string;
  settlementSession: string;
  channel: Channel;
  breakType: 'LEDGER_DEBIT_NO_SWITCH' | 'SWITCH_RECORD_NO_LEDGER' | 'AMOUNT_MISMATCH' | 'TIMING_CUTOFF_BREAK';
  breakLabel: string;
  severity: 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';
  status: BreakStatus;
  amountDifference: number; // e.g. 0 if missing entirely, or positive delta
  timeDifferenceSeconds: number;
  ledgerRecord?: LedgerRecord;
  switchRecord?: SwitchRecord;
  toleranceApplicable: boolean;
  toleranceRule?: string;
  isToleranceApplied?: boolean;
  pairedWithId?: string;
  caseReferenceId?: string;
  makerNotes?: string;
  checkerSignoff?: {
    approvedBy?: string;
    approvedAt?: string;
    status: 'PENDING' | 'APPROVED' | 'REJECTED';
  };
}

export interface SettlementBatchCycle {
  id: string;
  sessionCode: string;
  channel: Channel;
  date: string;
  cutOffWindow: string;
  switchPartner: string;
  totalLedgerRecords: number;
  totalSwitchRecords: number;
  matchedCount: number;
  unmatchedCount: number;
  netDiscrepancyAmount: string;
  status: 'OPEN' | 'BALANCED' | 'DISCREPANCY_FLAGGED';
}

export interface ToleranceConfig {
  amountToleranceNgn: number; // e.g. 5.00
  timeWindowSeconds: number; // e.g. 1800 (30 mins)
  autoPairToleranceMatches: boolean;
  requireCheckerSignoff: boolean;
}
