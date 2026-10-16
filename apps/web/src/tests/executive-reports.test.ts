import { describe, it, expect } from 'vitest';
import {
  EXECUTIVE_KPI_SUMMARY,
  CBN_REGULATORY_DISPUTE_RETURN,
  MERCHANT_RISK_HOTSPOTS,
} from '../features/reports/reportsData';

describe('Screens 14, 15, 16: Executive Reporting, Regulatory Return & Heatmap Tests (Prompt F6)', () => {
  describe('Screen 14: Executive KPI Summary', () => {
    it('verifies executive financial metrics meet corporate targets', () => {
      expect(EXECUTIVE_KPI_SUMMARY.totalValueRecoveredNgn).toContain('₦');
      expect(EXECUTIVE_KPI_SUMMARY.slaCompliancePct).toBeGreaterThan(95.0);
      expect(EXECUTIVE_KPI_SUMMARY.stpResolutionRatePct).toBeGreaterThan(80.0);
      expect(EXECUTIVE_KPI_SUMMARY.meanTimeToResolveMinutes).toBeLessThan(60); // Under an hour
      expect(EXECUTIVE_KPI_SUMMARY.regulatoryPenaltiesAvoidedNgn).toContain('₦');
    });
  });

  describe('Screen 15: CBN Regulatory Dispute Return Generator', () => {
    it('contains all mandatory CBN reporting channels and metadata', () => {
      const returnDoc = CBN_REGULATORY_DISPUTE_RETURN;
      expect(returnDoc.institutionCode).toBe('035');
      expect(returnDoc.auditCertificationStatus).toBe('CERTIFIED');
      expect(returnDoc.channels.length).toBeGreaterThanOrEqual(4);

      const channelNames = returnDoc.channels.map((c) => c.channel);
      expect(channelNames).toContain('NIP');
      expect(channelNames).toContain('POS');
      expect(channelNames).toContain('ATM');
      expect(channelNames).toContain('CARD');
    });

    it('validates dispute math across channels', () => {
      CBN_REGULATORY_DISPUTE_RETURN.channels.forEach((c) => {
        expect(c.disputesReceived).toBeGreaterThan(0);
        expect(c.resolvedWithinSla).toBeLessThanOrEqual(c.disputesReceived);
        expect(c.complianceRatePct).toBeGreaterThanOrEqual(95.0);
        expect(c.outstandingDisputes).toBeGreaterThanOrEqual(0);
      });
    });
  });

  describe('Screen 16: Merchant & Terminal Risk Heatmap', () => {
    it('identifies terminal failure modes and acquirer partners', () => {
      expect(MERCHANT_RISK_HOTSPOTS.length).toBeGreaterThanOrEqual(5);

      const criticalHotspots = MERCHANT_RISK_HOTSPOTS.filter((h) => h.riskRating === 'CRITICAL');
      expect(criticalHotspots.length).toBeGreaterThan(0);

      const acquirers = MERCHANT_RISK_HOTSPOTS.map((h) => h.acquirerBank);
      expect(acquirers.length).toBe(MERCHANT_RISK_HOTSPOTS.length);
    });

    it('ensures each hotspot has valid dispute metrics', () => {
      MERCHANT_RISK_HOTSPOTS.forEach((h) => {
        expect(h.terminalId).toBeDefined();
        expect(h.location).toBeDefined();
        expect(h.disputeCount30d).toBeGreaterThan(0);
        expect(h.totalDisputedVolumeNgn).toContain('₦');
      });
    });
  });
});
