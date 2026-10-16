export type ReverseItStep =
  | 'S1_REPORT_PROBLEM'
  | 'S2_PASTE_UPLOAD'
  | 'S3_CHECK_DETAILS'
  | 'S4_MATCH_CANDIDATES'
  | 'S5_WHAT_WENT_WRONG'
  | 'S6_YOUR_DEADLINE'
  | 'S7_REVIEW_SEND'
  | 'S7_CONFIRMATION'
  | 'S8_CASE_TRACKER'
  | 'S9_ESCALATION_LADDER'
  | 'MY_CASES';

export type IntakeMode = 'PASTE_TEXT' | 'UPLOAD_SCREENSHOT';

export type DisputeType =
  | 'failed_transfer'
  | 'duplicate_debit'
  | 'pos_web_failure'
  | 'atm_failure'
  | 'wrong_account_or_amount';

export interface ParsedAlertDetails {
  amount: string;
  currency: string;
  accountTail: string;
  dateTime: string;
  narration: string;
  reference: string;
  channel: string;
  issuingBank: string;
  confidence: {
    amount: number;
    accountTail: number;
    reference: number;
    overall: number;
  };
  isNonWemaAlert: boolean;
}

export interface MatchedTransactionCandidate {
  id: string;
  reference: string;
  amount: string;
  currency: string;
  channel: string;
  dateTime: string;
  beneficiaryMasked: string;
  matchScore: number;
  matchConfidence: 'HIGH' | 'MEDIUM' | 'LOW';
  matchReason: string;
  status: 'FAILED' | 'PENDING' | 'REVERSED' | 'SUCCESS';
}

export interface RegulatoryClockInfo {
  disputeType: DisputeType;
  clockKey: string;
  regulatoryHours: number;
  regulatoryLabel: string;
  bankTargetHours: number;
  bankTargetLabel: string;
  sourceAuthority: string;
  effectiveDate: string;
  status: 'CONFIRMED' | 'REPORTED_DRAFT' | 'CONFLICTING';
}

export interface CustomerDisputeCase {
  id: string;
  caseNumber: string;
  trackingId: string;
  customerName: string;
  maskedAccount: string;
  amount: string;
  currency: string;
  disputeType: DisputeType;
  disputeLabel: string;
  channel: string;
  reference: string;
  narration: string;
  reportedAt: string;
  regulatoryDeadline: string;
  bankTargetDeadline: string;
  clockStatus: 'ON_TRACK' | 'APPROACHING' | 'URGENT' | 'OVERDUE';
  timeRemainingText: string;
  status: 'DETECTED' | 'INVESTIGATING' | 'REVERSAL_INITIATED' | 'VERIFIED_RESOLVED';
  statusLabel: string;
  customerSafeStatusText: string;
  complaintLetter: string;
  isOverdue: boolean;
  history: Array<{
    stage: string;
    label: string;
    timestamp: string;
    details: string;
    verified: boolean;
  }>;
}
