import { describe, it, expect } from 'vitest';
import {
  DEFAULT_SAFETY_GATE_CONFIG,
  INITIAL_ADAPTER_HEALTH,
} from '../features/rules/rulesData';

describe('Screens 12 & 13: Rules Engine & Switch Integration Health Tests (Prompt F7)', () => {
  describe('Screen 12: Business Rules & SLA Thresholds', () => {
    it('enforces multi-tier authorization ceilings in ascending order', () => {
      const cfg = DEFAULT_SAFETY_GATE_CONFIG;
      expect(cfg.tier1AutoReversalMaxNgn).toBeLessThan(cfg.tier2SupervisorThresholdNgn);
      expect(cfg.tier2SupervisorThresholdNgn).toBeLessThan(cfg.tier3CommitteeThresholdNgn);
      expect(cfg.tier1AutoReversalMaxNgn).toBe(50000);
      expect(cfg.tier2SupervisorThresholdNgn).toBe(500000);
      expect(cfg.tier3CommitteeThresholdNgn).toBe(5000000);
    });

    it('mandates high-confidence thresholds for automated zero-touch actions', () => {
      const cfg = DEFAULT_SAFETY_GATE_CONFIG;
      expect(cfg.minimumModelConfidencePct).toBeGreaterThanOrEqual(90);
      expect(cfg.minimumModelConfidencePct).toBe(95);
    });

    it('enforces 8-check safety pre-flight assertions by default', () => {
      const cfg = DEFAULT_SAFETY_GATE_CONFIG;
      expect(cfg.requireCounterpartyTsqQuery).toBe(true);
      expect(cfg.enableIdempotencyLock).toBe(true);
      expect(cfg.blockBlacklistedBeneficiaries).toBe(true);
    });

    it('enforces regulatory CBN alert window before deadline expiry', () => {
      const cfg = DEFAULT_SAFETY_GATE_CONFIG;
      expect(cfg.regulatoryUrgentAlertHours).toBe(4); // Warn 4 hours ahead of 24h statutory cliff
      expect(cfg.fxRoundingToleranceNgn).toBe(5.0); // Central Bank FX 4-decimal tolerance
    });
  });

  describe('Screen 13: Switch & Core Banking Integration Health', () => {
    it('monitors all critical banking switch partners and core systems', () => {
      const adapterNames = INITIAL_ADAPTER_HEALTH.map((a) => a.name);
      expect(adapterNames.some((n) => n.includes('Finacle'))).toBe(true);
      expect(adapterNames.some((n) => n.includes('NIBSS'))).toBe(true);
      expect(adapterNames.some((n) => n.includes('Postilion'))).toBe(true);
      expect(adapterNames.some((n) => n.includes('Mastercard'))).toBe(true);
      expect(adapterNames.some((n) => n.includes('Interswitch'))).toBe(true);
    });

    it('tracks circuit breaker states across all adapters', () => {
      INITIAL_ADAPTER_HEALTH.forEach((adapter) => {
        expect(['CLOSED', 'HALF_OPEN', 'OPEN']).toContain(adapter.circuitBreaker);
        expect(adapter.uptimePct).toBeGreaterThan(99.0);
        expect(adapter.activeTps).toBeGreaterThan(0);
        expect(adapter.pingLatencyMs).toBeGreaterThan(0);
      });
    });

    it('correctly tags degraded adapters with elevated latency or error rates', () => {
      const degraded = INITIAL_ADAPTER_HEALTH.filter((a) => a.status === 'DEGRADED');
      expect(degraded.length).toBeGreaterThan(0);
      degraded.forEach((adapter) => {
        expect(adapter.circuitBreaker === 'HALF_OPEN' || adapter.pingLatencyMs > 100).toBe(true);
      });
    });
  });
});
