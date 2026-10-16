import { describe, it, expect, beforeEach } from 'vitest';
import { apiClient } from '../services/apiClient';
import type { CustomerIntakeQuery, CustomerDisputeIntakePayload } from '../types/domain';

describe('First-Contact Agent Intake & Reversal Governance (Screens 7, 8, 11)', () => {
  beforeEach(() => {
    apiClient.resetSimulator();
  });

  describe('Permitted Identifier Search & 4 Result States', () => {
    it('State A: resolves active recon exception with safe status, next step, SLA, and INC-8821 banner', async () => {
      const query: CustomerIntakeQuery = {
        queryType: 'REFERENCE',
        reference: 'TXN-9901',
      };

      const result = await apiClient.searchCustomerQuery(query, 'CONTACT_CENTRE_AGENT');
      expect(result.state).toBe('MATCH_EXCEPTION');
      expect(result.exception).toBeDefined();
      expect(result.exception?.id).toBe('exc-001');

      // Customer-safe status text
      expect(result.customerSafeStatusText).toContain('settlement reconciliation');

      // Next step & SLA
      expect(result.expectedNextStep).toContain('NIBSS settlement journal sync');
      expect(result.timeEstimate).toContain('Target resolution within 4 hours');

      // Incident banner
      expect(result.incidentBanner).toBeDefined();
      expect(result.incidentBanner?.incidentId).toBe('INC-8821');
      expect(result.incidentBanner?.title).toContain('Intermittent NIBSS Outward Gateway timeout');

      // Approved wording strict compliance check: NEVER say "refunded" unless verified
      expect(result.canQuoteRefund).toBe(false);
      expect(result.approvedWording.toLowerCase()).not.toContain('refunded');
      expect(result.approvedWording).toContain('tracking session NIP9901');
    });

    it('State B: resolves verified successful transaction with quotable settlement proof', async () => {
      const query: CustomerIntakeQuery = {
        queryType: 'REFERENCE',
        reference: 'TXN-9902',
      };

      const result = await apiClient.searchCustomerQuery(query, 'CONTACT_CENTRE_AGENT');
      expect(result.state).toBe('VERIFIED_SUCCESS');
      expect(result.transaction).toBeDefined();
      expect(result.transaction?.status).toBe('resolved');

      // Evidence summary that agent can quote
      expect(result.evidenceSummary).toBeDefined();
      expect(result.evidenceSummary?.coreLedgerDebitRecorded).toBe(true);
      expect(result.evidenceSummary?.counterpartyCreditConfirmed).toBe(true);
      expect(result.evidenceSummary?.sessionId).toContain('999014261015');

      // Approved wording confirms delivery with Session ID
      expect(result.approvedWording).toContain('successfully delivered');
      expect(result.approvedWording).toContain(result.evidenceSummary?.sessionId ?? '');
    });

    it('State C: handles unknown transaction by preparing structured dispute intake (CUS001)', async () => {
      const query: CustomerIntakeQuery = {
        queryType: 'REFERENCE',
        reference: 'TXN-UNKNOWN-999',
      };

      const result = await apiClient.searchCustomerQuery(query, 'CONTACT_CENTRE_AGENT');
      expect(result.state).toBe('NO_MATCH');
      expect(result.customerSafeStatusText).toContain('No matching transaction record found');

      // Agent files a structured dispute intake
      const payload: CustomerDisputeIntakePayload = {
        customerName: 'Amina Bello',
        customerPhone: '+234 802 000 1122',
        customerAccount: '0123456789',
        channel: 'POS',
        amount: '25000.00',
        currency: 'NGN',
        transactionDate: '2026-10-15',
        beneficiaryBank: 'First Bank of Nigeria',
        complaintText: 'Customer physically at Victoria Island branch with ATM slip.',
        agentId: 'AGT-019',
        agentRole: 'BRANCH_AGENT',
      };

      const createdCase = await apiClient.createCustomerReportedIntake(payload);
      expect(createdCase.caseNumber).toMatch(/^CASE-CUS-\d{4}$/);
      expect(createdCase.exception.classificationCode).toBe('CUS001');
      expect(createdCase.exception.status).toBe('detected');
      expect(createdCase.exception.regulatoryDeadline).toBeDefined();
    });

    it('State D: returns disambiguation candidates when query matches multiple transactions', async () => {
      const query: CustomerIntakeQuery = {
        queryType: 'AMOUNT_DATE_CHANNEL',
        amount: '45000',
        channel: 'POS',
        date: '2026-10-15',
      };

      const result = await apiClient.searchCustomerQuery(query, 'CONTACT_CENTRE_AGENT');
      expect(result.state).toBe('MULTIPLE_CANDIDATES');
      expect(result.candidates).toBeDefined();
      expect(result.candidates?.length).toBeGreaterThanOrEqual(2);

      // Verify candidates have matching amounts
      result.candidates?.forEach((cand) => {
        expect(Number(cand.amount)).toBe(45000);
      });
    });
  });

  describe('Repeat Contact Tracking & Supervisor Escalation Threshold', () => {
    it('detects repeat contact threshold (>= 3 inquiries) and allows escalation to supervisor queue', async () => {
      const query: CustomerIntakeQuery = {
        queryType: 'CUSTOMER_TOKEN',
        customerToken: '+234 809 111 2233',
      };

      const result = await apiClient.searchCustomerQuery(query, 'CONTACT_CENTRE_AGENT');
      expect(result.repeatContactCount).toBeGreaterThanOrEqual(3);
      expect(result.isThresholdReached).toBe(true);

      // Agent logs a call and requests supervisor escalation
      const updatedExc = await apiClient.logAgentContact(
        result.exception?.id || 'exc-001',
        '+234 809 111 2233',
        'Customer agitated, 3rd call in 24 hours. Escalating to supervisor.',
        true,
        'Chioma Nwosu (Contact Centre)'
      );

      expect(updatedExc).not.toBeNull();
      expect(updatedExc?.lane).toBe('SUPERVISOR_QUEUE');
      expect(updatedExc?.repeatContactCount).toBeGreaterThanOrEqual(2);

      // Verify audit trail entry was recorded
      const logs = await apiClient.getAuditLogs();
      const escalationLog = logs.find((l) => l.action === 'SUPERVISOR_ESCALATION_TRIGGERED');
      expect(escalationLog).toBeDefined();
      expect(escalationLog?.actorRole).toBe('CONTACT_CENTRE_AGENT');
    });
  });

  describe('Audited Data Reveal for Masked Financial Attributes', () => {
    it('logs an audit entry when sensitive masked data is unmasked with reason code', async () => {
      await apiClient.logDataReveal(
        'accountNumber',
        '0123456789',
        'AGENT-LAGOS-01',
        'CUSTOMER_IDENTITY_VERIFICATION'
      );

      const logs = await apiClient.getAuditLogs();
      const revealLog = logs.find((l) => l.action === 'SENSITIVE_DATA_REVEALED');
      expect(revealLog).toBeDefined();
      expect(revealLog?.entityId).toBe('0123456789');
      expect(revealLog?.details).toMatchObject({
        field: 'accountNumber',
        justification: 'CUSTOMER_IDENTITY_VERIFICATION',
      });
    });
  });

  describe('F4 Reversal Queue, Detail and Approval (Screens 7, 8, 11)', () => {
    it('Screen 7: retrieves proposal queue with SLA countdowns and exposures', async () => {
      const proposals = await apiClient.getReversals();
      expect(proposals.length).toBeGreaterThanOrEqual(2);

      const pendingProposal = proposals.find((p) => p.status === 'approval');
      expect(pendingProposal).toBeDefined();
      expect(Number(pendingProposal?.reversalAmount)).toBeGreaterThan(0);
      expect(pendingProposal?.idempotencyKey).toBeDefined();
    });

    it('Screen 8: performs 8 Financial Integrity safety checks (S1-S8 / R51)', async () => {
      const detail = await apiClient.getReversalDetail('rev-prop-8801');
      expect(detail).toBeDefined();
      expect(detail?.safetyChecks.length).toBe(8);

      const checkNames = detail?.safetyChecks.map((c) => c.name);
      expect(checkNames?.some((n) => n.startsWith('S1'))).toBe(true); // S1: Transactional Idempotency Key
      expect(checkNames?.some((n) => n.startsWith('S2'))).toBe(true); // S2: Duplicate Reversal Prevention
      expect(checkNames?.some((n) => n.startsWith('S3'))).toBe(true); // S3: Core Ledger Single Debit Verified
      expect(checkNames?.some((n) => n.startsWith('S4'))).toBe(true); // S4: Counterparty Non-Credit Proof
      expect(checkNames?.some((n) => n.startsWith('S5'))).toBe(true); // S5: Dual-Control Policy Threshold
      expect(checkNames?.some((n) => n.startsWith('S6'))).toBe(true); // S6: Separation of Duties (Maker != Checker)
      expect(checkNames?.some((n) => n.startsWith('S7'))).toBe(true); // S7: Reversal Gateway Kill Switch Inactive
      expect(checkNames?.some((n) => n.startsWith('S8'))).toBe(true); // S8: Evidence Freshness Gate

      // All safety checks in mock are verified passed
      detail?.safetyChecks.forEach((check) => {
        expect(check.status).toBe('PASSED');
      });
      expect(detail?.allSafetyChecksPassed).toBe(true);
    });

    it('Screen 11: enforces dual-control approval protocol (Maker cannot be Checker)', async () => {
      const detail = await apiClient.getReversalDetail('rev-prop-8801');
      expect(detail).toBeDefined();

      const maker = detail?.proposedBy; // 'Olusola Adeyemi (Analyst Maker)'
      const checker = 'Folake Balogun (Supervisor Checker)';

      // Separation of duties rule: Checker must be different from Maker
      expect(maker).not.toBe(checker);

      const approvalRes = await apiClient.authorizeReversal({
        reversalId: 'rev-prop-8801',
        authorizedBy: checker,
        decision: 'APPROVE',
        notes: 'Verified S1-S8 financial safety checks; approved for settlement.',
      });

      expect(approvalRes.status).toBe('resolved');
      expect(approvalRes.authorizedBy).toBe(checker);

      // Verify audit trail logged the approval
      const logs = await apiClient.getAuditLogs();
      const authLog = logs.find((l) => l.action === 'REVERSAL_AUTHORIZED_AND_DISPATCHED');
      expect(authLog).toBeDefined();
      expect(authLog?.actorId).toBe(checker);
    });
  });
});
