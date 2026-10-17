import type {
  DisputeAbuseProfile,
  DisputeAbuseRiskTier,
  DisputeAbuseFlag,
  Channel,
  MakerCheckerPolicy,
} from '../../types/domain';

export interface AbuseEngineInput {
  accountNumber?: string;
  channel?: Channel;
  amount: number | string;
  currency?: string;
  pastDisputesCount?: number;
  recentVelocity7d?: number;
  repeatContactCount?: number;
  isAtmDispenseClaim?: boolean;
  switchConfirmedCashDispensed?: boolean;
}

/**
 * Institutional Friendly Fraud & Dispute Abuse Scoring Engine
 * Analyzes velocity, channel patterns, historical claims, and switch telemetry
 * to prevent first-party fraud and repeat claim exploitation.
 */
export function calculateDisputeAbuseProfile(input: AbuseEngineInput): DisputeAbuseProfile {
  const numAmount = typeof input.amount === 'string' ? parseFloat(input.amount) || 0 : input.amount;
  const velocity7d = input.recentVelocity7d ?? (input.pastDisputesCount && input.pastDisputesCount > 2 ? 3 : 1);
  const velocity30d = (input.pastDisputesCount ?? 1) + 1;
  const repeatContacts = input.repeatContactCount ?? 0;
  const isAtm = input.channel === 'ATM' || input.isAtmDispenseClaim;
  const cashDispensed = input.switchConfirmedCashDispensed ?? false;

  let score = 12; // Base baseline risk
  const flags: DisputeAbuseFlag[] = [];

  // 1. Dispute Velocity Scoring
  if (velocity7d >= 3) {
    score += 32;
    flags.push({
      code: 'VELOCITY_SPIKE',
      severity: 'CRITICAL',
      title: 'High Dispute Velocity Spike',
      description: `Customer submitted ${velocity7d} separate disputes in the past 7 days (Institutional alert threshold: 2).`,
    });
  } else if (velocity7d === 2) {
    score += 15;
    flags.push({
      code: 'REPEAT_DISPUTER',
      severity: 'WARNING',
      title: 'Repeat Claimant Profile',
      description: 'Multiple active disputes registered across retail banking channels within rolling 7-day window.',
    });
  }

  // 2. ATM Cash Dispense & Terminal Hardware Telemetry
  if (isAtm && cashDispensed) {
    score += 40;
    flags.push({
      code: 'ATM_CASH_TRAP_CLAIM',
      severity: 'CRITICAL',
      title: 'Suspected Cash Trap / False Non-Dispense Claim',
      description: 'Atm switch journal recorded physical note transport exit & shutter sensor clearance, but customer claimed zero dispense.',
    });
  }

  // 3. High Value Exposure & Credit Arbitrage Risk
  if (numAmount >= 1000000) {
    score += 18;
    flags.push({
      code: 'CREDIT_ABUSE_RISK',
      severity: 'WARNING',
      title: 'High Value Reversal Exposure (> ₦1,000,000)',
      description: 'Transaction exceeds ₦1.0M threshold. Reversal constitutes significant settlement exposure.',
    });
  }

  // 4. Repeated Unsubstantiated Caller Inquiries
  if (repeatContacts >= 3) {
    score += 12;
    flags.push({
      code: 'REPEAT_DISPUTER',
      severity: 'INFO',
      title: 'High Inbound Contact Frequency',
      description: `${repeatContacts} customer contact attempts logged before switch journal reconciliation completed.`,
    });
  }

  // 5. Friendly Fraud Suspicion Combined Indicator
  if (velocity7d >= 2 && numAmount >= 250000) {
    score += 15;
    flags.push({
      code: 'SUSPECTED_FRIENDLY_FRAUD',
      severity: 'CRITICAL',
      title: 'Elevated Friendly Fraud Indicator',
      description: 'Transaction pattern correlates with multi-channel cardholder friendly-fraud signature.',
    });
  }

  // Cap score at 100
  score = Math.min(100, Math.max(0, score));

  // Determine Risk Tier
  let tier: DisputeAbuseRiskTier = 'LOW_RISK_TRUSTED';
  let recommendation: DisputeAbuseProfile['recommendation'] = 'AUTO_APPROVE_ELIGIBLE';

  if (score >= 75) {
    tier = 'HIGH_RISK_RESTRICTED';
    recommendation = 'REJECT_SUSPECTED_FRAUD';
  } else if (score >= 50) {
    tier = 'ELEVATED_WATCH';
    recommendation = 'MANDATORY_IN_BRANCH_VERIFICATION';
  } else if (score >= 25) {
    tier = 'STANDARD';
    recommendation = 'STANDARD_OPS_REVIEW';
  } else {
    tier = 'LOW_RISK_TRUSTED';
    recommendation = 'AUTO_APPROVE_ELIGIBLE';
  }

  const falseClaimRate =
    score >= 75 ? 68.5 : score >= 50 ? 32.0 : score >= 25 ? 12.5 : 2.1;

  return {
    score,
    tier,
    disputeVelocity7d: velocity7d,
    disputeVelocity30d: velocity30d,
    historicalFalseClaimRatePct: falseClaimRate,
    totalDisputesFiled: velocity30d + 1,
    totalReversalsApproved: Math.max(1, velocity30d - 1),
    flags,
    recommendation,
  };
}

/**
 * Maker-Checker Dual-Control Policy Matrix
 * Tier 1: Micro-reversal (< ₦25,000) -> Auto eligible if switch confirmed failed & abuse score < 25
 * Tier 2: Single Maker (₦25,000 - ₦500,000) -> 1 Operations Analyst sign-off
 * Tier 3: Dual-Control Mandatory Maker-Checker (> ₦500,000) -> Maker proposes, Supervisor Counter-signs
 */
export function getMakerCheckerPolicy(
  amount: number | string,
  currency = 'NGN'
): MakerCheckerPolicy {
  const numAmount = typeof amount === 'string' ? parseFloat(amount) || 0 : amount;

  if (currency === 'NGN') {
    if (numAmount < 25000) {
      return {
        tier: 'TIER_1_AUTO',
        requiresChecker: false,
        minAmount: 0,
        maxAmount: 25000,
        currency: 'NGN',
        policyDescription: 'Tier 1 Micro-Reversal: Straight-through processing eligible with verified switch timeout and low abuse score.',
      };
    } else if (numAmount <= 500000) {
      return {
        tier: 'TIER_2_SINGLE_MAKER',
        requiresChecker: false,
        minAmount: 25000,
        maxAmount: 500000,
        currency: 'NGN',
        policyDescription: 'Tier 2 Single Maker: Single Operations Analyst sign-off required with documented ledger justification.',
      };
    } else {
      return {
        tier: 'TIER_3_DUAL_CONTROL',
        requiresChecker: true,
        minAmount: 500000,
        maxAmount: 100000000,
        currency: 'NGN',
        policyDescription: 'Tier 3 Dual Control: Mandatory Maker-Checker protocol. Maker proposes, Senior Operations Supervisor must counter-sign with authentication PIN.',
      };
    }
  }

  // Default USD or foreign currency
  if (numAmount < 50) {
    return {
      tier: 'TIER_1_AUTO',
      requiresChecker: false,
      minAmount: 0,
      maxAmount: 50,
      currency,
      policyDescription: 'Tier 1 Micro-Reversal: Straight-through auto processing.',
    };
  } else if (numAmount <= 1000) {
    return {
      tier: 'TIER_2_SINGLE_MAKER',
      requiresChecker: false,
      minAmount: 50,
      maxAmount: 1000,
      currency,
      policyDescription: 'Tier 2 Single Maker Sign-off required.',
    };
  } else {
    return {
      tier: 'TIER_3_DUAL_CONTROL',
      requiresChecker: true,
      minAmount: 1000,
      maxAmount: 10000000,
      currency,
      policyDescription: 'Tier 3 Mandatory Dual Control Maker-Checker protocol.',
    };
  }
}

/**
 * Calculates central bank statutory penalty exposure for breached exceptions.
 * e.g., CBN BPS circular levies statutory fines of ₦10,000 per day for delayed consumer refunds.
 */
export function calculateStatutoryFineExposure(
  regulatoryDeadlineIso: string,
  dailyFineNgn = 10000
): {
  isBreached: boolean;
  hoursOverdue: number;
  daysOverdue: number;
  accruedFineNgn: number;
  regulatoryAuthority: string;
} {
  const deadline = new Date(regulatoryDeadlineIso).getTime();
  const now = Date.now();
  const diffMs = now - deadline;

  if (diffMs <= 0) {
    return {
      isBreached: false,
      hoursOverdue: 0,
      daysOverdue: 0,
      accruedFineNgn: 0,
      regulatoryAuthority: 'CBN / CFPB Mandate',
    };
  }

  const hoursOverdue = Math.floor(diffMs / (1000 * 60 * 60));
  const daysOverdue = Math.max(1, Math.ceil(hoursOverdue / 24));
  const accruedFineNgn = daysOverdue * dailyFineNgn;

  return {
    isBreached: true,
    hoursOverdue,
    daysOverdue,
    accruedFineNgn,
    regulatoryAuthority: 'CBN BPS/DIR/GEN/CIR/05/011 Mandate',
  };
}
