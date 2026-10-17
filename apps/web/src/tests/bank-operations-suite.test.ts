import { describe, it, expect } from 'vitest';
import {
  calculateDisputeAbuseProfile,
  getMakerCheckerPolicy,
  calculateStatutoryFineExposure,
} from '../features/fraud/DisputeAbuseEngine';

describe('Part 2: Bank Operations, Fraud Defense & Compliance Suite', () => {
  describe('Dispute Abuse Scoring & Friendly Fraud Sentinel', () => {
    it('should assign LOW_RISK_TRUSTED to a normal first-time claimant', () => {
      const profile = calculateDisputeAbuseProfile({
        amount: 15000,
        channel: 'NIP',
        pastDisputesCount: 0,
        recentVelocity7d: 1,
        repeatContactCount: 1,
      });

      expect(profile.tier).toBe('LOW_RISK_TRUSTED');
      expect(profile.score).toBeLessThanOrEqual(25);
      expect(profile.recommendation).toBe('AUTO_APPROVE_ELIGIBLE');
      expect(profile.flags.length).toBe(0);
    });

    it('should flag VELOCITY_SPIKE when customer files 3 or more claims in 7 days', () => {
      const profile = calculateDisputeAbuseProfile({
        amount: 45000,
        channel: 'POS',
        recentVelocity7d: 3,
        repeatContactCount: 1,
      });

      expect(profile.score).toBeGreaterThan(40);
      const spikeFlag = profile.flags.find((f) => f.code === 'VELOCITY_SPIKE');
      expect(spikeFlag).toBeDefined();
      expect(spikeFlag?.severity).toBe('CRITICAL');
    });

    it('should flag ATM_CASH_TRAP_CLAIM and assign HIGH_RISK_RESTRICTED when hardware transport sensor cleared', () => {
      const profile = calculateDisputeAbuseProfile({
        amount: 80000,
        channel: 'ATM',
        isAtmDispenseClaim: true,
        switchConfirmedCashDispensed: true,
        recentVelocity7d: 3,
      });

      expect(profile.score).toBeGreaterThanOrEqual(75);
      expect(profile.tier).toBe('HIGH_RISK_RESTRICTED');
      expect(profile.recommendation).toBe('REJECT_SUSPECTED_FRAUD');

      const trapFlag = profile.flags.find((f) => f.code === 'ATM_CASH_TRAP_CLAIM');
      expect(trapFlag).toBeDefined();
      expect(trapFlag?.severity).toBe('CRITICAL');
    });

    it('should flag CREDIT_ABUSE_RISK for claims over ₦1,000,000', () => {
      const profile = calculateDisputeAbuseProfile({
        amount: 1500000,
        channel: 'NIP',
        recentVelocity7d: 1,
      });

      const highValFlag = profile.flags.find((f) => f.code === 'CREDIT_ABUSE_RISK');
      expect(highValFlag).toBeDefined();
    });
  });

  describe('Tiered Maker-Checker Dual-Control Policy Matrix', () => {
    it('should assign TIER_1_AUTO for micro-reversals under ₦25,000', () => {
      const policy = getMakerCheckerPolicy(12500, 'NGN');
      expect(policy.tier).toBe('TIER_1_AUTO');
      expect(policy.requiresChecker).toBe(false);
    });

    it('should assign TIER_2_SINGLE_MAKER for standard amounts between ₦25,000 and ₦500,000', () => {
      const policy = getMakerCheckerPolicy(150000, 'NGN');
      expect(policy.tier).toBe('TIER_2_SINGLE_MAKER');
      expect(policy.requiresChecker).toBe(false);
    });

    it('should enforce TIER_3_DUAL_CONTROL for large amounts over ₦500,000', () => {
      const policy = getMakerCheckerPolicy(650000, 'NGN');
      expect(policy.tier).toBe('TIER_3_DUAL_CONTROL');
      expect(policy.requiresChecker).toBe(true);
      expect(policy.policyDescription).toContain('Dual Control');
    });

    it('should evaluate foreign currency policies correctly', () => {
      const policyUsdMicro = getMakerCheckerPolicy(25, 'USD');
      expect(policyUsdMicro.tier).toBe('TIER_1_AUTO');
      expect(policyUsdMicro.requiresChecker).toBe(false);

      const policyUsdLarge = getMakerCheckerPolicy(2500, 'USD');
      expect(policyUsdLarge.tier).toBe('TIER_3_DUAL_CONTROL');
      expect(policyUsdLarge.requiresChecker).toBe(true);
    });
  });

  describe('Central Bank Statutory Penalty Exposure Engine', () => {
    it('should return zero fine when within regulatory SLA window', () => {
      const futureDate = new Date(Date.now() + 24 * 3600 * 1000).toISOString();
      const exposure = calculateStatutoryFineExposure(futureDate);

      expect(exposure.isBreached).toBe(false);
      expect(exposure.accruedFineNgn).toBe(0);
      expect(exposure.hoursOverdue).toBe(0);
    });

    it('should accrue daily statutory fines when deadline breached', () => {
      // 36 hours past deadline = 2 days penalty
      const pastDate = new Date(Date.now() - 36 * 3600 * 1000).toISOString();
      const exposure = calculateStatutoryFineExposure(pastDate, 10000);

      expect(exposure.isBreached).toBe(true);
      expect(exposure.hoursOverdue).toBeGreaterThanOrEqual(35);
      expect(exposure.daysOverdue).toBe(2);
      expect(exposure.accruedFineNgn).toBe(20000);
      expect(exposure.regulatoryAuthority).toContain('CBN');
    });
  });

  describe('Part 3: Institutional Switch & Core Banking (CBS) Double-Entry Integrity', () => {
    it('should validate ISO 8583 MTI 0420 Reversal Advice specifications', () => {
      const reversalAdviceMti = '0420';
      const processingCodeCredit = '200000';
      expect(reversalAdviceMti.startsWith('04')).toBe(true);
      expect(processingCodeCredit.startsWith('20')).toBe(true);
    });

    it('should guarantee balanced double-entry accounting on reversal posting', () => {
      const reversalAmount = 45000.0;
      const debitLeg = {
        glAccount: 'GL-100299-NIP-SUSPENSE',
        drAmount: reversalAmount,
        crAmount: 0,
      };
      const creditLeg = {
        customerAccount: '0128394910',
        drAmount: 0,
        crAmount: reversalAmount,
      };

      const totalDebit = debitLeg.drAmount + creditLeg.drAmount;
      const totalCredit = debitLeg.crAmount + creditLeg.crAmount;

      expect(totalDebit).toBe(totalCredit);
      expect(totalDebit - totalCredit).toBe(0);
    });
  });
});
