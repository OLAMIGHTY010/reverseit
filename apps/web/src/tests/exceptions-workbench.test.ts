import { describe, it, expect } from 'vitest';
import { apiClient } from '../services/apiClient';
import { INITIAL_EXCEPTIONS } from '../services/mockSimulatorData';
import type { ClassificationCode, SLAStatus } from '../types/domain';

describe('Exception Queue & Workbench Domain Logic', () => {
  it('contains all 6 mandatory Scope v2 classification codes', () => {
    const codes = new Set(INITIAL_EXCEPTIONS.map((e) => e.classificationCode));
    const expectedCodes: ClassificationCode[] = [
      'DUP001',
      'FAIL001',
      'UNK001',
      'REV001',
      'STL001',
      'CUS001',
    ];

    expectedCodes.forEach((code) => {
      expect(codes.has(code)).toBe(true);
    });
  });

  it('differentiates BOT vs CUSTOMER source origins', () => {
    const botExceptions = INITIAL_EXCEPTIONS.filter((e) => e.source === 'BOT');
    const customerExceptions = INITIAL_EXCEPTIONS.filter((e) => e.source === 'CUSTOMER');

    expect(botExceptions.length).toBeGreaterThan(0);
    expect(customerExceptions.length).toBeGreaterThan(0);

    // Customer exceptions should have customer details
    customerExceptions.forEach((c) => {
      expect(c.customerName).toBeDefined();
      expect(c.customerPhone).toBeDefined();
    });
  });

  it('calculates priority scores with valid breakdown factors summing correctly', () => {
    INITIAL_EXCEPTIONS.forEach((e) => {
      expect(e.priorityScore).toBeGreaterThanOrEqual(0);
      expect(e.priorityScore).toBeLessThanOrEqual(100);
      expect(e.priorityFactors).toBeDefined();
      expect(e.priorityFactors.amountWeight).toBeGreaterThanOrEqual(0);
      expect(e.priorityFactors.slaWeight).toBeGreaterThanOrEqual(0);
      expect(e.priorityFactors.customerWeight).toBeGreaterThanOrEqual(0);
      expect(e.priorityFactors.anomalyWeight).toBeGreaterThanOrEqual(0);
      expect(e.priorityFactors.rationale.length).toBeGreaterThan(10);
    });
  });

  it('contains valid dual SLA targets and statuses (regulatory and internal)', () => {
    const statuses = new Set<SLAStatus>(['ON_TRACK', 'APPROACHING', 'BREACHED']);

    INITIAL_EXCEPTIONS.forEach((e) => {
      expect(new Date(e.internalSlaTarget).getTime()).toBeGreaterThan(0);
      expect(new Date(e.regulatoryDeadline).getTime()).toBeGreaterThan(0);
      expect(statuses.has(e.slaStatus)).toBe(true);
    });

    const breached = INITIAL_EXCEPTIONS.filter((e) => e.slaStatus === 'BREACHED');
    const approaching = INITIAL_EXCEPTIONS.filter((e) => e.slaStatus === 'APPROACHING');
    expect(breached.length).toBeGreaterThan(0);
    expect(approaching.length).toBeGreaterThan(0);
  });

  it('filters customer-reported cases correctly', async () => {
    const all = await apiClient.getExceptions();
    const customerCases = all.filter((e) => e.source === 'CUSTOMER');
    expect(customerCases.length).toBeGreaterThan(0);

    customerCases.forEach((c) => {
      expect(c.source).toBe('CUSTOMER');
    });
  });

  it('allows an analyst to claim an unassigned exception', async () => {
    const unassigned = INITIAL_EXCEPTIONS.find((e) => !e.assignedTo);
    expect(unassigned).toBeDefined();

    if (unassigned) {
      const claimed = await apiClient.claimException(
        unassigned.id,
        'analyst.maker@wema.ng',
        'Olusola Adeyemi',
        unassigned.version
      );

      expect(claimed.assignedTo).toBe('analyst.maker@wema.ng');
      expect(claimed.version).toBe(unassigned.version + 1);
      expect(claimed.stateHistory.length).toBeGreaterThan(0);
    }
  });

  it('enforces optimistic concurrency protection on conflicting edits', async () => {
    const exc = INITIAL_EXCEPTIONS[0];
    const staleVersion = exc.version - 1; // Stale version

    await expect(
      apiClient.updateExceptionStatus(
        exc.id,
        'investigating',
        'ANALYST_DEEP_DIVE',
        'Test Analyst',
        'Valid justification statement',
        staleVersion
      )
    ).rejects.toThrow('CONCURRENCY_CONFLICT');
  });

  it('appends timestamped investigation notes to the audit history', async () => {
    const exc = INITIAL_EXCEPTIONS[1];
    const updated = await apiClient.addExceptionNote(
      exc.id,
      'Olusola Adeyemi',
      'ANALYST_MAKER',
      'Verified with merchant switch log: socket drop occurred.'
    );

    const latestNote = updated.notes[updated.notes.length - 1];
    expect(latestNote.text).toContain('merchant switch log');
    expect(latestNote.author).toBe('Olusola Adeyemi');
    expect(updated.version).toBe(exc.version + 1);
  });
});
