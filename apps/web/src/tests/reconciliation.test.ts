import { describe, it, expect } from 'vitest';
import {
  INITIAL_RECONCILIATION_BREAKS,
  SETTLEMENT_CYCLES,
  DEFAULT_TOLERANCE_CONFIG,
} from '../features/reconciliation/reconciliationData';
import type { ReconciliationBreak } from '../features/reconciliation/reconciliationTypes';

describe('Screen 6: Multi-Source Reconciliation & Settlement Workspace Tests', () => {
  describe('Settlement Batch Cycles', () => {
    it('provides multi-channel settlement cycles for NIP, POS, and CARD', () => {
      const channels = SETTLEMENT_CYCLES.map((c) => c.channel);
      expect(channels).toContain('NIP');
      expect(channels).toContain('POS');
      expect(channels).toContain('CARD');
    });

    it('calculates reconciliation accuracy rates consistently', () => {
      SETTLEMENT_CYCLES.forEach((cycle) => {
        expect(cycle.totalLedgerRecords).toBeGreaterThan(0);
        expect(cycle.matchedCount).toBeLessThanOrEqual(cycle.totalLedgerRecords);
        expect(cycle.unmatchedCount).toBeGreaterThanOrEqual(0);
        const matchPct = (cycle.matchedCount / cycle.totalLedgerRecords) * 100;
        expect(matchPct).toBeGreaterThan(95); // High baseline matching
      });
    });
  });

  describe('Reconciliation Break Typologies', () => {
    it('covers all major Nigerian banking settlement break typologies', () => {
      const breakTypes = INITIAL_RECONCILIATION_BREAKS.map((b) => b.breakType);
      expect(breakTypes).toContain('LEDGER_DEBIT_NO_SWITCH');
      expect(breakTypes).toContain('SWITCH_RECORD_NO_LEDGER');
      expect(breakTypes).toContain('AMOUNT_MISMATCH');
      expect(breakTypes).toContain('TIMING_CUTOFF_BREAK');
    });

    it('correctly captures dual-sided records for breaks', () => {
      const nipBreak = INITIAL_RECONCILIATION_BREAKS.find((b) => b.id === 'break-nip-001')!;
      expect(nipBreak.ledgerRecord).toBeDefined();
      expect(nipBreak.ledgerRecord?.coreSystem).toBe('Finacle Core 11');
      expect(nipBreak.switchRecord).toBeUndefined(); // Missing switch clearing record
      expect(nipBreak.amountDifference).toBe(25000);
    });

    it('correctly models ghost switch credit settlements missing ledger entries', () => {
      const ghostBreak = INITIAL_RECONCILIATION_BREAKS.find((b) => b.id === 'break-ghost-005')!;
      expect(ghostBreak.ledgerRecord).toBeUndefined(); // Customer ledger missed credit
      expect(ghostBreak.switchRecord).toBeDefined();
      expect(ghostBreak.switchRecord?.switchPartner).toBe('NIBSS Instant Payment');
      expect(ghostBreak.switchRecord?.clearingAmount).toBe('35,000.00');
    });
  });

  describe('Tolerance Rule Engine', () => {
    it('identifies FX precision rounding differences within ±₦5.00', () => {
      const fxBreak = INITIAL_RECONCILIATION_BREAKS.find((b) => b.id === 'break-tol-004')!;
      expect(fxBreak.toleranceApplicable).toBe(true);
      expect(fxBreak.amountDifference).toBeLessThanOrEqual(DEFAULT_TOLERANCE_CONFIG.amountToleranceNgn);
      expect(fxBreak.status).toBe('TOLERANCE_MATCHED');
    });

    it('excludes breaks outside tolerance threshold from auto-match', () => {
      const feeBreak = INITIAL_RECONCILIATION_BREAKS.find((b) => b.id === 'break-amt-003')!;
      expect(feeBreak.amountDifference).toBe(52.5);
      expect(feeBreak.amountDifference).toBeGreaterThan(DEFAULT_TOLERANCE_CONFIG.amountToleranceNgn);
      expect(feeBreak.toleranceApplicable).toBe(false);
      expect(feeBreak.status).toBe('UNMATCHED');
    });
  });

  describe('Dual-Control Manual Pairing & Sign-Off', () => {
    it('supports timing cutoff boundary pairing with checker approval tracking', () => {
      const cutOffBreak = INITIAL_RECONCILIATION_BREAKS.find((b) => b.id === 'break-cut-006')!;
      expect(cutOffBreak.breakType).toBe('TIMING_CUTOFF_BREAK');
      expect(cutOffBreak.status).toBe('MANUALLY_PAIRED');
      expect(cutOffBreak.checkerSignoff).toBeDefined();
      expect(cutOffBreak.checkerSignoff?.status).toBe('APPROVED');
      expect(cutOffBreak.checkerSignoff?.approvedBy).toContain('Checker');
    });

    it('enforces that breaks escalated to investigation have proper references', () => {
      const caseLinkedBreaks = INITIAL_RECONCILIATION_BREAKS.filter((b) => b.caseReferenceId);
      expect(caseLinkedBreaks.length).toBeGreaterThan(0);
      caseLinkedBreaks.forEach((b: ReconciliationBreak) => {
        expect(b.caseReferenceId).toMatch(/^CUS-\d{4}-\d+/);
      });
    });
  });
});
