import { describe, it, expect } from 'vitest';
import { apiClient } from '../services/apiClient';
import { INITIAL_INVESTIGATIONS } from '../services/mockSimulatorData';
import type { InvestigationCase, RootCause } from '../types/domain';

describe('Transaction Investigation Workspace - Domain & Integration Logic', () => {
  it('loads all mock investigation dossiers including duplicate and customer-reported cases', async () => {
    const cases = await apiClient.getInvestigationCases();
    expect(cases.length).toBeGreaterThanOrEqual(3);

    const dupCase = cases.find((c) => c.id === 'inv-case-403');
    expect(dupCase).toBeDefined();
    expect(dupCase?.duplicateComparison).toBeDefined();

    const customerCase = cases.find((c) => c.id === 'inv-case-405');
    expect(customerCase).toBeDefined();
    expect(customerCase?.customerDetails).toBeDefined();
  });

  describe('Lifecycle Timeline Component Domain Logic', () => {
    it('orders events chronologically and includes all sources (CBS, SWITCH, AI_ENGINE, CUSTOMER, OPS)', () => {
      const nipCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-401') as InvestigationCase;
      expect(nipCase).toBeDefined();

      const sources = new Set(nipCase.timeline.map((e) => e.source));
      expect(sources.has('CBS')).toBe(true);
      expect(sources.has('SWITCH')).toBe(true);
      expect(sources.has('AI_ENGINE')).toBe(true);

      // Verify timestamp ordering
      for (let i = 0; i < nipCase.timeline.length - 1; i++) {
        const t1 = new Date(nipCase.timeline[i].sourceTimestamp).getTime();
        const t2 = new Date(nipCase.timeline[i + 1].sourceTimestamp).getTime();
        expect(t2).toBeGreaterThanOrEqual(t1);
      }
    });

    it('identifies clock-skew markers and late-arriving events', () => {
      const nipCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-401') as InvestigationCase;
      const skewedEvent = nipCase.timeline.find((e) => Math.abs(e.clockSkewMs) > 1000);
      expect(skewedEvent).toBeDefined();
      expect(skewedEvent?.clockSkewMs).toBe(1800);

      const lateEvent = nipCase.timeline.find((e) => e.isLateArriving);
      expect(lateEvent).toBeDefined();
      expect(lateEvent?.lateArrivalReason).toContain('Late-arriving event: Settlement journal batch');
    });

    it('detects communication gaps between switch and core banking (14m gap marker)', () => {
      const nipCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-401') as InvestigationCase;
      const gapEvent = nipCase.timeline.find((e) => e.gapDurationMinutes && e.gapDurationMinutes >= 14);
      expect(gapEvent).toBeDefined();
      expect(gapEvent?.gapMessage).toContain('for 14m');
    });
  });

  describe('4-Quadrant Evidence Dossier & Stale Action Gate', () => {
    it('separates evidence into Confirmed Facts, Inferred Explanations, Missing Evidence, and Unresolved Questions', () => {
      const nipCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-401') as InvestigationCase;
      const facts = nipCase.evidence.filter((e) => e.type === 'FACT');
      const inferred = nipCase.evidence.filter((e) => e.type === 'INFERRED');
      const missing = nipCase.evidence.filter((e) => e.type === 'MISSING');
      const unresolved = nipCase.evidence.filter((e) => e.type === 'UNRESOLVED');

      expect(facts.length).toBeGreaterThan(0);
      expect(inferred.length).toBeGreaterThan(0);
      expect(missing.length).toBeGreaterThan(0);
      expect(unresolved.length).toBeGreaterThan(0);

      // Facts must be verified
      facts.forEach((f) => expect(f.verified).toBe(true));

      // Inferred items must have model basis and confidence
      inferred.forEach((inf) => {
        expect(inf.inferredBasis).toBeDefined();
        expect(inf.confidence).toBeGreaterThan(0);
      });
    });

    it('enforces that stale evidence CANNOT support reversal actions', () => {
      const nipCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-401') as InvestigationCase;
      const staleItems = nipCase.evidence.filter((e) => e.isStale);
      expect(staleItems.length).toBeGreaterThan(0);

      staleItems.forEach((stale) => {
        expect(stale.canSupportAction).toBe(false);
        expect(stale.freshness).toBe('STALE');
      });
    });

    it('handles interactive request-check flow for missing evidence', async () => {
      const updated = await apiClient.requestMissingEvidenceCheck(
        'inv-case-401',
        'ev-4',
        'NIBSS Settlement Bridge'
      );

      const requestedItem = updated.evidence.find((e) => e.id === 'ev-4');
      expect(requestedItem?.details).toContain('DISPATCHED to NIBSS Settlement Bridge');
    });
  });

  describe('Duplicate Comparison & Side-by-Side Field Diffing', () => {
    it('calculates millisecond interval delta and high risk grade for double tap debits', () => {
      const dupCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-403') as InvestigationCase;
      expect(dupCase.duplicateComparison).toBeDefined();
      const dup = dupCase.duplicateComparison!;

      expect(dup.isDuplicateSuspect).toBe(true);
      expect(dup.intervalMs).toBe(480);
      expect(dup.riskGrade).toBe('HIGH');
      expect(dup.reasons.length).toBeGreaterThanOrEqual(3);
      expect(dup.ruleOutcome).toBe('CONFIRMED_DUPLICATE');
    });

    it('includes side-by-side field diffs highlighting matching vs mismatching attributes', () => {
      const dupCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-403') as InvestigationCase;
      const diffs = dupCase.duplicateComparison!.fieldDiffs;

      const stanDiff = diffs.find((d) => d.field === 'stan');
      expect(stanDiff).toBeDefined();
      expect(stanDiff?.isMatch).toBe(true);

      const timestampDiff = diffs.find((d) => d.field === 'timestamp');
      expect(timestampDiff).toBeDefined();
      expect(timestampDiff?.isMatch).toBe(false); // +480ms interval mismatch
    });

    it('executes duplicate confirmation and legitimate repeat verdicts via API client', async () => {
      const confirmedCase = await apiClient.confirmDuplicateVerdict(
        'inv-case-403',
        'CONFIRMED_DUPLICATE',
        'Identical STAN and cardholder hash within 480ms interval.'
      );
      expect(confirmedCase.duplicateComparison?.ruleOutcome).toBe('CONFIRMED_DUPLICATE');

      const legitCase = await apiClient.confirmDuplicateVerdict(
        'inv-case-403',
        'LEGITIMATE_RETRY',
        'Customer deliberately re-initiated transfer after merchant prompt.'
      );
      expect(legitCase.duplicateComparison?.ruleOutcome).toBe('LEGITIMATE_RETRY');
    });
  });

  describe('Customer-Reported Dispute Dossier (Scope v2 Additions)', () => {
    it('contains masked alert, NLP parsed fields with confidence, and complaint text', () => {
      const customerCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-405') as InvestigationCase;
      expect(customerCase.customerDetails).toBeDefined();
      const alert = customerCase.customerDetails!;

      expect(alert.rawAlertMasked).toContain('₦350,000.00');
      expect(alert.complaintText).toContain('transferred ₦350,000.00');
      expect(alert.repeatContactCount).toBe(3);

      // Parsed fields
      expect(alert.parsedFields.length).toBeGreaterThan(0);
      alert.parsedFields.forEach((field) => {
        expect(field.field).toBeDefined();
        expect(field.value).toBeDefined();
        expect(field.confidence).toBeGreaterThan(0.8);
      });
    });

    it('verifies tamper flags (SMS gateway integrity, timestamp alignment, receipt hash check)', () => {
      const customerCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-405') as InvestigationCase;
      const flags = customerCase.customerDetails!.tamperFlags;

      expect(flags.length).toBe(3);
      flags.forEach((f) => {
        expect(f.passed).toBe(true);
      });
    });

    it('lists match candidates with similarity confidence', () => {
      const customerCase = INITIAL_INVESTIGATIONS.find((c) => c.id === 'inv-case-405') as InvestigationCase;
      const candidates = customerCase.customerDetails!.matchCandidates;

      expect(candidates.length).toBeGreaterThan(0);
      expect(candidates[0].matchConfidence).toBeGreaterThanOrEqual(0.95);
      expect(candidates[0].reference).toContain('TXN-20261015-NIP-9905');
    });
  });

  describe('Scope v2 Root-Cause Confirmation & Governance Actions', () => {
    it('allows analyst to confirm AI suggested root-cause tag and records maker audit stamp', async () => {
      const updated = await apiClient.confirmRootCause(
        'inv-case-401',
        'SWITCH_TIMEOUT' as RootCause,
        'Olusola Adeyemi (Maker)'
      );

      expect(updated.isRootCauseConfirmed).toBe(true);
      expect(updated.confirmedRootCause).toBe('SWITCH_TIMEOUT');

      // Check timeline was appended
      const lastTl = updated.timeline[updated.timeline.length - 1];
      expect(lastTl.title).toContain('Root Cause Confirmed: SWITCH_TIMEOUT');
    });

    it('tracks external dependencies with named counterparty and SLA deadline', async () => {
      const updated = await apiClient.addExternalDependency('inv-case-401', {
        namedParty: 'NIBSS Central Switch Recon Operations',
        expectedBy: new Date(Date.now() + 3600000).toISOString(),
        ticketReference: 'NIBSS-TKT-99201',
        reason: 'Awaiting end-of-cycle batch reconciliation report.',
        status: 'PENDING',
      });

      expect(updated.externalDependencies.length).toBeGreaterThan(0);
      const dep = updated.externalDependencies.find((d) => d.ticketReference === 'NIBSS-TKT-99201');
      expect(dep).toBeDefined();
      expect(dep?.status).toBe('PENDING');
    });

    it('closes investigation case with official CBN resolution code', async () => {
      const closed = await apiClient.closeInvestigationCase(
        'inv-case-401',
        'AUTO_REVERSED_SUCCESS',
        'Funds successfully credited back to customer account under CBN 72h circular.',
        'Olusola Adeyemi (Maker)'
      );

      expect(closed.exception.status).toBe('resolved');
      expect(closed.exception.resolutionCode).toBe('AUTO_REVERSED_SUCCESS');
    });
  });
});
