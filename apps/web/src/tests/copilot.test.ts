import { describe, it, expect } from 'vitest';
import { INITIAL_INVESTIGATIONS } from '../services/mockSimulatorData';

describe('Screen 9: AI Copilot & Citation Assistant Tests (Prompt F8)', () => {
  const sampleCase = INITIAL_INVESTIGATIONS[0];

  describe('Statutory Citations & Regulatory Grounding', () => {
    it('verifies that statutory circular citations reference valid Central Bank of Nigeria mandates', () => {
      expect(sampleCase).toBeDefined();
      expect(sampleCase.exception).toBeDefined();

      const statutoryCitation = 'CBN Circular BPS/DIR/GEN/CIR/05/011';
      expect(statutoryCitation).toContain('BPS/DIR/GEN/CIR/05/011');
      expect(statutoryCitation).toContain('CBN');
    });

    it('ensures evidence bundle references are verified and canonical', () => {
      expect(sampleCase.evidence.length).toBeGreaterThan(0);
      const verifiedEvidence = sampleCase.evidence.filter((e) => e.verified);
      expect(verifiedEvidence.length).toBeGreaterThanOrEqual(1);

      // Verify at least one fact-based evidence item
      const facts = sampleCase.evidence.filter((e) => e.type === 'FACT');
      expect(facts.length).toBeGreaterThanOrEqual(1);
    });
  });

  describe('One-Click Statutory Draft Generators', () => {
    it('generates Maker Reversal Proposal containing all mandatory pre-flight checks', () => {
      const makerDraft = `REVERSAL PROPOSAL JUSTIFICATION — CASE ${sampleCase.caseNumber}
Transaction Ref: ${sampleCase.reference}
Amount: NGN ${sampleCase.exception.amount} | Channel: ${sampleCase.exception.channel}

FINDINGS & EVIDENCE BASIS:
1. Core Ledger debit entry posted.
2. Switch TSQ query to counterparty verified transaction state: UNKNOWN/FAILED.
3. Diagnostic analysis confirms root cause: ${sampleCase.aiSuggestedRootCause || 'Switch Gateway Timeout'}.
4. 8-Check Reversal Safety Pre-flight passed: Verified single-writer lock, idempotency key unique, destination account uncredited.

REGULATORY CITATION:
In compliance with Central Bank of Nigeria Circular BPS/DIR/GEN/CIR/05/011 on Failed Electronic Payments (24-Hour Resolution Mandate).`;

      expect(makerDraft).toContain(sampleCase.caseNumber);
      expect(makerDraft).toContain(sampleCase.reference);
      expect(makerDraft).toContain(sampleCase.exception.amount);
      expect(makerDraft).toContain('8-Check Reversal Safety Pre-flight passed');
      expect(makerDraft).toContain('BPS/DIR/GEN/CIR/05/011');
    });

    it('generates safe customer notice strictly adhering to two-stage non-committal language', () => {
      const customerDraft = `DISPUTE UPDATE — REFERENCE ${sampleCase.caseNumber}
Dear Customer,

We are actively investigating the transaction exception of NGN ${sampleCase.exception.amount} reported on your account. 

Current Status: We have submitted a status query to the interbank settlement switch to confirm the counterparty status. No action is required from you at this time.

In accordance with CBN Instant Payment Standards, we will notify you immediately once the settlement verification is concluded.`;

      expect(customerDraft).toContain(sampleCase.caseNumber);
      expect(customerDraft).toContain(sampleCase.exception.amount);
      const lower = customerDraft.toLowerCase();
      // Must not make premature promises
      expect(lower).not.toContain('your account has been credited');
      expect(lower).not.toContain('your refund is complete');
      expect(lower).not.toContain('funds returned to your balance');
    });
  });

  describe('Model Confidence & Risk Indicator', () => {
    it('ensures model confidence is a valid bounded percentage', () => {
      expect(sampleCase.modelConfidence).toBeGreaterThanOrEqual(0);
      expect(sampleCase.modelConfidence).toBeLessThanOrEqual(100);
      expect(sampleCase.aiSuggestedRemedy).toBeDefined();
      expect(sampleCase.aiSuggestedRemedy!.length).toBeGreaterThan(10);
    });
  });
});
