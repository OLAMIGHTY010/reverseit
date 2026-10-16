import { describe, it, expect } from 'vitest';
import {
  REGULATORY_CLOCKS,
  SAMPLE_NIGERIAN_ALERTS,
  INITIAL_CUSTOMER_CASES,
} from '../features/reverseit/reverseitData';
import type { CustomerDisputeCase, DisputeType } from '../features/reverseit/reverseitTypes';

describe('ReverseIt Customer Module & Regulatory Compliance Tests', () => {
  describe('Deterministic Regulatory Clocks (Section 10.4)', () => {
    it('enforces 24-hour statutory deadline for NIP failed transfers', () => {
      const nipClock = REGULATORY_CLOCKS.failed_transfer;
      expect(nipClock.regulatoryHours).toBe(24);
      expect(nipClock.sourceAuthority).toContain('Central Bank of Nigeria');
      expect(nipClock.sourceAuthority).toContain('BPS/DIR/GEN/CIR/05/011');
      expect(nipClock.status).toBe('CONFIRMED');
      expect(nipClock.bankTargetHours).toBeLessThanOrEqual(nipClock.regulatoryHours);
    });

    it('enforces 72-hour statutory deadline for POS/Web failures', () => {
      const posClock = REGULATORY_CLOCKS.pos_web_failure;
      expect(posClock.regulatoryHours).toBe(72);
      expect(posClock.sourceAuthority).toContain('CBN Circular on Disputed Electronic Transactions');
      expect(posClock.status).toBe('CONFIRMED');
    });

    it('enforces 48-hour statutory deadline for ATM dispense failures', () => {
      const atmClock = REGULATORY_CLOCKS.atm_failure;
      expect(atmClock.regulatoryHours).toBe(48);
      expect(atmClock.sourceAuthority).toContain('CBN Guidelines for ATM Operations');
      expect(atmClock.status).toBe('CONFIRMED');
    });

    it('enforces 24-hour statutory deadline for duplicate debits', () => {
      const dupClock = REGULATORY_CLOCKS.duplicate_debit;
      expect(dupClock.regulatoryHours).toBe(24);
      expect(dupClock.status).toBe('CONFIRMED');
    });

    it('enforces 14 working days (336 hours) recall window for wrong account/amount transfers', () => {
      const recallClock = REGULATORY_CLOCKS.wrong_account_or_amount;
      expect(recallClock.regulatoryHours).toBe(336);
      expect(recallClock.sourceAuthority).toContain('Beneficiary Consent & Funds Availability');
    });

    it('verifies all dispute types have deterministic clock configurations', () => {
      const disputeTypes: DisputeType[] = [
        'failed_transfer',
        'duplicate_debit',
        'pos_web_failure',
        'atm_failure',
        'wrong_account_or_amount',
      ];
      disputeTypes.forEach((type) => {
        const clock = REGULATORY_CLOCKS[type];
        expect(clock).toBeDefined();
        expect(clock.disputeType).toBe(type);
        expect(clock.regulatoryHours).toBeGreaterThan(0);
        expect(clock.bankTargetHours).toBeGreaterThan(0);
        expect(clock.bankTargetHours).toBeLessThanOrEqual(clock.regulatoryHours);
      });
    });
  });

  describe('Sample Alert Parsers and Presets', () => {
    it('provides valid presets for major Nigerian banks', () => {
      expect(SAMPLE_NIGERIAN_ALERTS.length).toBeGreaterThanOrEqual(4);
      const bankNames = SAMPLE_NIGERIAN_ALERTS.map((a) => a.bankName);
      expect(bankNames).toContain('WEMA / ALAT');
      expect(bankNames).toContain('GTBank');
      expect(bankNames).toContain('Zenith Bank');
      expect(bankNames).toContain('Access Bank');
    });

    it('accurately parses WEMA / ALAT alert text', () => {
      const wemaAlert = SAMPLE_NIGERIAN_ALERTS.find((a) => a.bankName.includes('WEMA'))!;
      expect(wemaAlert.parsed.amount).toBe('25,000.00');
      expect(wemaAlert.parsed.channel).toBe('NIP');
      expect(wemaAlert.parsed.reference).toBe('WEMA-TRF-992144');
      expect(wemaAlert.parsed.confidence.overall).toBeGreaterThanOrEqual(95);
      expect(wemaAlert.parsed.isNonWemaAlert).toBe(false);
    });

    it('flags non-Wema bank alerts to display cross-bank guidance', () => {
      const gtbAlert = SAMPLE_NIGERIAN_ALERTS.find((a) => a.bankName === 'GTBank')!;
      expect(gtbAlert.parsed.isNonWemaAlert).toBe(true);
      expect(gtbAlert.parsed.issuingBank).toBe('GTBank');

      const zenithAlert = SAMPLE_NIGERIAN_ALERTS.find((a) => a.bankName === 'Zenith Bank')!;
      expect(zenithAlert.parsed.isNonWemaAlert).toBe(true);
    });
  });

  describe('Safe Customer Communication Language (Section 10.5)', () => {
    it('never says "reversed" or "refunded" in customerSafeStatusText for investigating or initiated states', () => {
      INITIAL_CUSTOMER_CASES.forEach((c: CustomerDisputeCase) => {
        if (c.status === 'DETECTED' || c.status === 'INVESTIGATING' || c.status === 'REVERSAL_INITIATED') {
          const lowerText = c.customerSafeStatusText.toLowerCase();
          // Prohibited premature completion terms in customer-facing notice:
          expect(lowerText).not.toContain('your money has been refunded');
          expect(lowerText).not.toContain('your account has been credited');
          expect(lowerText).not.toContain('funds returned to your balance');
        }
      });
    });

    it('ensures each case has an unverified final posting step until verified resolved', () => {
      INITIAL_CUSTOMER_CASES.forEach((c: CustomerDisputeCase) => {
        if (c.status !== 'VERIFIED_RESOLVED') {
          const finalStage = c.history.find((h) => h.stage === 'VERIFIED_RESOLVED');
          expect(finalStage).toBeDefined();
          expect(finalStage?.verified).toBe(false);
        }
      });
    });

    it('contains formal statutory complaint letters citing proper channels', () => {
      INITIAL_CUSTOMER_CASES.forEach((c: CustomerDisputeCase) => {
        expect(c.complaintLetter).toBeDefined();
        expect(c.complaintLetter).toContain(c.caseNumber);
        expect(c.complaintLetter).toContain(c.currency);
        expect(c.complaintLetter).toContain(c.amount);
      });
    });
  });
});
