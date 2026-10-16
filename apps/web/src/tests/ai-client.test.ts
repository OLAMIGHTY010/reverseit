import { describe, it, expect } from 'vitest';
import { aiClient } from '../services/aiClient';
import { apiClient } from '../services/apiClient';

describe('Dual-Mode 3-Tier AI and Service Clients', () => {
  it('checks AI service health without crashing when offline', async () => {
    const health = await aiClient.getHealth();
    expect(health).toHaveProperty('status');
    expect(health).toHaveProperty('service');
    expect(health).toHaveProperty('isLive');
  });

  it('parses Nigerian bank debit alerts via aiClient (ALAT / Wema)', async () => {
    const sampleAlert = 'Debit Alert: Acct 0234****891 Amt: NGN 25,000.00 Desc: TRF/NIP/NIBSS/00001324101409124401 Date: 14-Oct-2026 Ref: WEMA-TRF-992144';
    const parsed = await aiClient.parseAlert(sampleAlert);

    expect(parsed.success).toBe(true);
    expect(parsed.amount_minor).toBe(2500000);
    expect(parsed.currency).toBe('NGN');
    expect(parsed.reference).toContain('WEMA-TRF-992144');
    expect(parsed.channel).toBe('NIP');
    expect(parsed.confidence_score).toBeGreaterThanOrEqual(80);
  });

  it('parses POS alert with terminal timeout description', async () => {
    const posAlert = 'Acct: 014****321 Amt: NGN 18,500.00 DR Desc: POS/WEB/PAYSTACK/TERM88291 Ref: GTB-POS-882910';
    const parsed = await aiClient.parseAlert(posAlert);

    expect(parsed.success).toBe(true);
    expect(parsed.amount_minor).toBe(1850000);
    expect(parsed.issuing_bank).toContain('GTBank');
    expect(parsed.channel).toBe('POS');
    expect(parsed.reference).toContain('GTB-POS-882910');
  });

  it('queries Copilot Assistant and returns citations and proposed actions', async () => {
    const copilotRes = await aiClient.queryCopilot({
      caseId: 'CASE-2026-401',
      userMessage: 'Did the beneficiary bank confirm receipt of funds?',
    });

    expect(copilotRes.case_id).toBe('CASE-2026-401');
    expect(copilotRes.response_text).toBeTruthy();
    expect(copilotRes.cited_evidence_ids.length).toBeGreaterThan(0);
    expect(copilotRes.tools_invoked.length).toBeGreaterThan(0);
    expect(copilotRes.proposed_actions.length).toBeGreaterThan(0);
  });

  it('returns unified 3-tier status for Go Backend, AI Service, and Frontend', async () => {
    const status = await apiClient.getServiceStatuses();

    expect(status).toHaveProperty('backend');
    expect(status).toHaveProperty('aiService');
    expect(status).toHaveProperty('simulationFallbackActive');
    expect(status.backend).toHaveProperty('isLive');
    expect(status.aiService).toHaveProperty('isLive');
  });
});
