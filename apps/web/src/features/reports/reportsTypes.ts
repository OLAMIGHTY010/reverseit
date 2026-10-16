import type { Channel } from '../../types/domain';

export interface ExecutiveKpiSummary {
  totalValueRecoveredNgn: string; // e.g. ₦418,290,000.00
  slaCompliancePct: number; // e.g. 99.4%
  stpResolutionRatePct: number; // e.g. 84.2%
  meanTimeToResolveMinutes: number; // e.g. 18.4 mins
  regulatoryPenaltiesAvoidedNgn: string; // e.g. ₦84,500,000.00
  customerRetentionImpactPct: number; // e.g. +9.2%
}

export interface CbnRegulatoryDisputeReturn {
  reportingPeriod: string; // e.g. "October 2026"
  institutionName: string; // "Wema Bank Plc / ALAT"
  institutionCode: string; // "035"
  submissionDeadline: string; // "31-Oct-2026"
  channels: {
    channel: Channel;
    disputesReceived: number;
    resolvedWithinSla: number;
    resolvedOutsideSla: number;
    outstandingDisputes: number;
    totalAmountDisputedNgn: string;
    totalAmountReversedNgn: string;
    complianceRatePct: number;
  }[];
  regulatoryEscalationsToCpd: number;
  auditCertificationStatus: 'CERTIFIED' | 'PENDING_SIGNOFF';
}

export interface MerchantRiskHotspot {
  id: string;
  terminalId: string;
  merchantName: string;
  location: string;
  channel: Channel;
  disputeCount30d: number;
  totalDisputedVolumeNgn: string;
  primaryFailureMode: string;
  riskRating: 'CRITICAL' | 'HIGH' | 'ELEVATED' | 'LOW';
  acquirerBank: string;
}
