/**
 * Domain types matching packages/api-contracts/openapi.yaml
 */

export type ThemeMode = 'daylight' | 'soft-dim';
export type DensityMode = 'comfortable' | 'compact';

export type Role =
  | 'ANALYST_MAKER'
  | 'SUPERVISOR_CHECKER'
  | 'AUDITOR'
  | 'ADMIN'
  | 'CONTACT_CENTRE_AGENT'
  | 'BRANCH_AGENT';

export type Channel = 'NIP' | 'POS' | 'CARD' | 'ATM' | 'WEB' | 'WEBHOOK' | 'DIRECT_DEBIT';

export type FinancialStatus =
  | 'detected'
  | 'awaiting'
  | 'investigating'
  | 'approval'
  | 'submitted'
  | 'verifying'
  | 'resolved'
  | 'failed'
  | 'suppressed';

export type Severity = 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';

export type ExceptionType =
  | 'DEBIT_WITHOUT_CREDIT'
  | 'DUPLICATE_DEBIT'
  | 'STATUS_MISMATCH'
  | 'SUSPENSE_BALANCE'
  | 'TIMEOUT_EXCEPTION';

export interface Transaction {
  id: string;
  reference: string;
  rrn: string;
  stan: string;
  amount: string; // Stored as precise string representation of decimal
  currency: string;
  channel: Channel;
  status: FinancialStatus;
  senderAccount: string;
  beneficiaryAccount: string;
  senderBank: string;
  beneficiaryBank: string;
  createdAt: string;
  settledAt?: string;
  maskedPan?: string;
  hasDiscrepancy: boolean;
  exceptionType?: ExceptionType;
  modelConfidence?: number; // 0-100%
  isVerified: boolean;
}

export type ClassificationCode =
  | 'DUP001'
  | 'FAIL001'
  | 'UNK001'
  | 'REV001'
  | 'STL001'
  | 'CUS001'
  | 'ATM001'
  | 'FEE001'
  | 'VND001'
  | 'MIS001'
  | 'INC001';

export type ExceptionSource = 'BOT' | 'CUSTOMER';

export type RootCause =
  | 'SWITCH_TIMEOUT'
  | 'LEDGER_GLITCH'
  | 'NETWORK_SPLIT'
  | 'MERCHANT_DROP'
  | 'CUSTOMER_DOUBLE_TAP'
  | 'SUSPENSE_MISMATCH';

export type Lane =
  | 'FAST_TRACK'
  | 'MANUAL_INVESTIGATION'
  | 'SUPERVISOR_QUEUE'
  | 'COMPLIANCE_HOLD';

export type SLAStatus = 'ON_TRACK' | 'APPROACHING' | 'BREACHED';

export type ExceptionTeam =
  | 'TIER_1_TRIAGE'
  | 'TIER_2_RECON'
  | 'DISPUTE_RESOLUTION'
  | 'FRAUD_OPS';

export interface PriorityFactors {
  amountWeight: number;
  slaWeight: number;
  customerWeight: number;
  anomalyWeight: number;
  rationale: string;
}

export interface ExceptionNote {
  id: string;
  author: string;
  authorRole: Role;
  timestamp: string;
  text: string;
}

export interface ExceptionStateTransition {
  from: FinancialStatus;
  to: FinancialStatus;
  reasonCode: string;
  author: string;
  timestamp: string;
}

export interface FinancialException {
  id: string;
  transactionId: string;
  reference: string;
  type: ExceptionType;
  classificationCode: ClassificationCode;
  source: ExceptionSource;
  rootCause: RootCause;
  severity: Severity;
  amount: string;
  currency: string;
  exposure: string;
  exposureType: 'DIRECT_LOSS' | 'SUSPENSE_HOLD' | 'CUSTOMER_DISPUTE';
  channel: Channel;
  status: FinancialStatus;
  lane: Lane;
  description: string;
  detectedAt: string;
  debitTimestamp: string;
  internalSlaTarget: string; // ISO string
  regulatoryDeadline: string; // ISO string
  slaStatus: SLAStatus;
  modelConfidence: number; // e.g. 96
  modelExplanation: string;
  priorityScore: number; // 0 - 100
  priorityFactors: PriorityFactors;
  assignedTo?: string; // e.g. "analyst.maker@wema.ng"
  ownerName?: string;
  team: ExceptionTeam;
  sourceFreshness: string; // e.g. "Live (2s)" or "Degraded (15m lag)"
  sourceFreshnessState: 'LIVE' | 'DEGRADED' | 'STALE';
  repeatContactCount: number;
  incidentId?: string; // e.g. "INC-8821"
  incidentTitle?: string;
  version: number; // For optimistic concurrency conflict detection
  notes: ExceptionNote[];
  stateHistory: ExceptionStateTransition[];
  maskedPan?: string;
  accountNumber?: string;
  customerName?: string;
  customerPhone?: string;
  rrn?: string;
  stan?: string;
  resolutionCode?: string;
  disputeAbuseProfile?: DisputeAbuseProfile;
  makerCheckerTier?: MakerCheckerTier;
  statutoryFineExposureNgn?: number;
}

export type TimelineSource =
  | 'CBS'
  | 'SWITCH'
  | 'AI_ENGINE'
  | 'CUSTOMER'
  | 'OPS'
  | 'NETWORK';

export interface TimelineEvent {
  id: string;
  source: TimelineSource;
  sourceName: string;
  sourceTimestamp: string;
  receivedTimestamp: string;
  clockSkewMs: number;
  isLateArriving: boolean;
  lateArrivalReason?: string;
  gapDurationMinutes?: number;
  gapMessage?: string;
  title: string;
  description: string;
  payload: Record<string, unknown>;
  linkedEvidenceId?: string;
  severity: 'INFO' | 'WARNING' | 'ERROR' | 'SUCCESS';
}

export type EvidenceFreshness = 'FRESH' | 'DEGRADED' | 'STALE';

export interface EvidenceItem {
  id: string;
  type: 'FACT' | 'INFERRED' | 'MISSING' | 'UNRESOLVED';
  title: string;
  source: string;
  timestamp?: string;
  details: string;
  verified: boolean;
  freshness?: EvidenceFreshness;
  freshnessLabel?: string;
  isStale?: boolean;
  canSupportAction?: boolean;
  missingActionLabel?: string;
  inferredBasis?: string;
  confidence?: number;
}

export interface DuplicateFieldDiff {
  field: string;
  label: string;
  isMatch: boolean;
  originalVal: string;
  suspectVal: string;
}

export interface DuplicateComparisonRecord {
  isDuplicateSuspect: boolean;
  riskGrade: 'LOW' | 'MEDIUM' | 'HIGH';
  riskScore: number; // 0-100
  reasons: string[];
  intervalMs: number;
  ruleOutcome: 'CONFIRMED_DUPLICATE' | 'SUSPECTED_DUPLICATE' | 'LEGITIMATE_RETRY';
  verdictReason: string;
  original: {
    id: string;
    reference: string;
    stan: string;
    rrn: string;
    amount: string;
    currency: string;
    timestamp: string;
    channel: Channel;
    senderAccount: string;
    beneficiaryAccount: string;
    idempotencyKey: string;
    terminalId: string;
    deviceSession: string;
    postingStatus: string;
  };
  suspect: {
    id: string;
    reference: string;
    stan: string;
    rrn: string;
    amount: string;
    currency: string;
    timestamp: string;
    channel: Channel;
    senderAccount: string;
    beneficiaryAccount: string;
    idempotencyKey: string;
    terminalId: string;
    deviceSession: string;
    postingStatus: string;
  };
  fieldDiffs: DuplicateFieldDiff[];
}

export interface CustomerReportedCaseDetails {
  rawAlertMasked: string;
  complaintText: string;
  parsedFields: Array<{ field: string; value: string; confidence: number }>;
  matchCandidates: Array<{
    reference: string;
    amount: string;
    timestamp: string;
    matchConfidence: number;
  }>;
  tamperFlags: Array<{ check: string; passed: boolean; note: string }>;
  repeatContactCount: number;
}

export interface ExternalDependency {
  id: string;
  namedParty: string;
  ticketReference: string;
  expectedBy: string; // ISO
  reason: string;
  status: 'PENDING' | 'RESOLVED' | 'BREACHED';
}

export interface LedgerPosting {
  id: string;
  account: string;
  accountName: string;
  type: 'DEBIT' | 'CREDIT';
  amount: string;
  currency: string;
  description: string;
  timestamp: string;
  isSettled: boolean;
}

export interface RelatedTransactionRecord {
  id: string;
  reference: string;
  amount: string;
  currency: string;
  channel: Channel;
  status: FinancialStatus;
  relation: string;
  timestamp: string;
}

export interface RelatedCaseRecord {
  id: string;
  caseNumber: string;
  similarityScore: number; // 0-100%
  title: string;
  status: FinancialStatus;
}

export interface InvestigationCase {
  id: string;
  caseNumber: string;
  exceptionId: string;
  reference: string;
  status: FinancialStatus;
  priority: Severity;
  assignedAnalyst: string;
  createdAt: string;
  updatedAt: string;
  exception: FinancialException;
  evidence: EvidenceItem[];
  timeline: TimelineEvent[];
  duplicateComparison?: DuplicateComparisonRecord;
  customerDetails?: CustomerReportedCaseDetails;
  externalDependencies: ExternalDependency[];
  confirmedRootCause?: RootCause;
  aiSuggestedRootCause: RootCause;
  isRootCauseConfirmed: boolean;
  ledgerPostings: LedgerPosting[];
  relatedTransactions: RelatedTransactionRecord[];
  relatedCases: RelatedCaseRecord[];
  killSwitchActive: boolean;
  degradedSourceBanner?: string;
  proposedAction?: string;
  aiSuggestedRemedy?: string;
  modelConfidence: number;
}

export interface ReversalProposal {
  id: string;
  transactionId: string;
  reference: string;
  originalAmount: string;
  reversalAmount: string;
  currency: string;
  channel: Channel;
  status: FinancialStatus; // 'approval', 'submitted', 'resolved', 'failed'
  proposedBy: string; // Maker
  proposedAt: string;
  reasonCode: string;
  justification: string;
  authorizedBy?: string; // Checker
  authorizedAt?: string;
  rejectionReason?: string;
  idempotencyKey: string;
  destinationAccount: string;
  destinationBank: string;
}

export interface AuditEvent {
  id: string;
  timestamp: string;
  actorId: string;
  actorRole: Role;
  action: string;
  entityType: 'TRANSACTION' | 'REVERSAL' | 'INVESTIGATION' | 'AUTH' | 'CONFIG';
  entityId: string;
  prevHash: string;
  entryHash: string;
  details: Record<string, unknown>;
  ipAddress: string;
}

export interface ReconciliationSummary {
  cycleDate: string;
  channel: Channel;
  totalTransactions: number;
  matchedCount: number;
  unmatchedCount: number;
  totalVolume: string;
  discrepancyVolume: string;
  reconciliationRate: number; // e.g. 98.4%
  status: 'COMPLETED' | 'IN_PROGRESS' | 'DISCREPANCY_FLAGGED';
}

export type AgentSearchIdentifierType = 'REFERENCE' | 'AMOUNT_DATE_CHANNEL' | 'CUSTOMER_TOKEN';

export interface CustomerIntakeQuery {
  queryType: AgentSearchIdentifierType;
  reference?: string;
  amount?: string;
  date?: string;
  channel?: Channel;
  customerToken?: string; // Account number or phone
}

export type AgentResultState =
  | 'MATCH_EXCEPTION'
  | 'VERIFIED_SUCCESS'
  | 'NO_MATCH'
  | 'MULTIPLE_CANDIDATES';

export interface AgentQueryResult {
  state: AgentResultState;
  query: CustomerIntakeQuery;
  customerSafeStatusText: string;
  approvedWording: string;
  canQuoteRefund: boolean; // Strictly false unless verified resolved
  expectedNextStep?: string;
  timeEstimate?: string;
  incidentBanner?: {
    incidentId: string;
    title: string;
    severity: Severity;
    eta: string;
  };
  evidenceSummary?: {
    coreLedgerDebitRecorded: boolean;
    counterpartyCreditConfirmed: boolean;
    stan: string;
    rrn: string;
    sessionId?: string;
    settledAt?: string;
    beneficiaryBank: string;
    beneficiaryAccount: string;
    quotableSummary: string;
  };
  exception?: FinancialException;
  investigationCase?: InvestigationCase;
  transaction?: Transaction;
  candidates?: Transaction[];
  repeatContactCount: number;
  isThresholdReached: boolean; // >= 3 repeat inquiries
  isSourceStale: boolean;
  sourceStaleMessage?: string;
}

export interface CustomerDisputeIntakePayload {
  customerName: string;
  customerPhone: string;
  customerAccount: string;
  channel: Channel;
  amount: string;
  currency: string;
  transactionDate: string;
  beneficiaryBank: string;
  beneficiaryAccount?: string;
  complaintText: string;
  agentId: string;
  agentRole: Role;
  linkedIncidentId?: string;
}

export interface ReversalSafetyCheck {
  id: string;
  name: string;
  description: string;
  status: 'PASSED' | 'FAILED' | 'WARNING';
  details: string;
}

export interface ReversalDetailRecord extends ReversalProposal {
  exception?: FinancialException;
  investigationCase?: InvestigationCase;
  safetyChecks: ReversalSafetyCheck[];
  allSafetyChecksPassed: boolean;
  exposureAmount: string;
  policyLimit: string;
  isWithinPolicyLimit: boolean;
  abuseProfile?: DisputeAbuseProfile;
  makerCheckerTier?: MakerCheckerTier;
}

export type DisputeAbuseRiskTier =
  | 'LOW_RISK_TRUSTED'
  | 'STANDARD'
  | 'ELEVATED_WATCH'
  | 'HIGH_RISK_RESTRICTED';

export type DisputeAbuseFlagCode =
  | 'VELOCITY_SPIKE'
  | 'SUSPECTED_FRIENDLY_FRAUD'
  | 'ATM_CASH_TRAP_CLAIM'
  | 'REPEAT_DISPUTER'
  | 'CREDIT_ABUSE_RISK'
  | 'BENEFICIARY_COLLUSION_SUSPECT';

export interface DisputeAbuseFlag {
  code: DisputeAbuseFlagCode;
  severity: 'INFO' | 'WARNING' | 'CRITICAL';
  title: string;
  description: string;
}

export interface DisputeAbuseProfile {
  score: number; // 0 - 100
  tier: DisputeAbuseRiskTier;
  disputeVelocity7d: number;
  disputeVelocity30d: number;
  historicalFalseClaimRatePct: number;
  totalDisputesFiled: number;
  totalReversalsApproved: number;
  flags: DisputeAbuseFlag[];
  recommendation:
    | 'AUTO_APPROVE_ELIGIBLE'
    | 'STANDARD_OPS_REVIEW'
    | 'MANDATORY_IN_BRANCH_VERIFICATION'
    | 'REJECT_SUSPECTED_FRAUD';
}

export type MakerCheckerTier = 'TIER_1_AUTO' | 'TIER_2_SINGLE_MAKER' | 'TIER_3_DUAL_CONTROL';

export interface MakerCheckerPolicy {
  tier: MakerCheckerTier;
  requiresChecker: boolean;
  minAmount: number;
  maxAmount: number;
  currency: string;
  policyDescription: string;
}

export interface BatchReversalResult {
  batchId: string;
  dispatchedAt: string;
  totalCount: number;
  successfulCount: number;
  skippedCount: number;
  totalExposure: string;
  currency: string;
  makerId: string;
  checkerId?: string;
  glPostingRef: string;
  itemReferences: string[];
}

