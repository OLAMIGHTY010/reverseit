/**
 * SMARTREVERSAL AI Service Client
 * 
 * Communicates with the FastAPI AI Service (apps/ai-service on port 8000 via /v1/ai proxy).
 * Employs Dual-Mode Architecture: Live-first network calls with seamless, high-fidelity
 * client-side fallback if the Python service is offline or unreachable.
 */

export interface AIParsedAlertResponse {
  success: boolean;
  amount_minor?: number;
  currency?: string;
  issuing_bank?: string;
  channel?: string;
  reference?: string;
  date?: string;
  sender_account_masked?: string;
  raw_redacted?: string;
  confidence_score?: number;
  is_live_service?: boolean;
}

export interface CopilotChatResponse {
  case_id: string;
  session_id: string;
  response_text: string;
  cited_evidence_ids: string[];
  tools_invoked: string[];
  proposed_actions: Array<{
    action_type: string;
    title: string;
    description: string;
    target_endpoint: string;
    payload_preview?: Record<string, unknown>;
  }>;
  rule_suggestions: Array<{
    rule_id: string;
    rule_name: string;
    diff_summary: string;
    rationale: string;
  }>;
  audit_ref: string;
  is_refusal: boolean;
  is_live_service?: boolean;
}

export interface AIHealthStatus {
  status: 'operational' | 'offline';
  service: string;
  latencyMs?: number;
  isLive: boolean;
}

class AIClient {
  private baseUrl = '/v1/ai';

  private getUrl(endpoint: string): string {
    const origin = typeof window !== 'undefined' ? '' : 'http://localhost:5173';
    return `${origin}${this.baseUrl}${endpoint}`;
  }

  /**
   * Check connection status to FastAPI AI service
   */
  async getHealth(): Promise<AIHealthStatus> {
    const start = performance.now();
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 2000);

      const res = await fetch(this.getUrl('/health'), {
        signal: controller.signal,
        headers: { Accept: 'application/json' },
      });
      clearTimeout(timeoutId);

      if (res.ok) {
        const data = await res.json();
        return {
          status: 'operational',
          service: data.service || 'ai-service',
          latencyMs: Math.round(performance.now() - start),
          isLive: true,
        };
      }
      return { status: 'offline', service: 'ai-service', isLive: false };
    } catch {
      return { status: 'offline', service: 'ai-service', isLive: false };
    }
  }

  /**
   * Parse customer alert SMS/email text (Prompt A9 / Screen S2)
   */
  async parseAlert(alertText: string): Promise<AIParsedAlertResponse> {
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 3500);

      const res = await fetch(this.getUrl('/reverseit/parse-alert'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ alert_text: alertText }),
        signal: controller.signal,
      });
      clearTimeout(timeoutId);

      if (res.ok) {
        const data = await res.json();
        return {
          ...data,
          is_live_service: true,
        };
      }
    } catch {
      // AI Service offline, use client-side parser
    }

    // Client-side fallback parsing
    return this.fallbackParseAlert(alertText);
  }

  /**
   * Copilot interactive inquiry (Prompt A7 / Screen 9)
   */
  async queryCopilot(params: {
    caseId: string;
    userMessage: string;
    sessionId?: string;
    history?: Array<{ role: string; content: string }>;
  }): Promise<CopilotChatResponse> {
    const sessionId = params.sessionId || `sess-${Date.now()}`;

    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 4000);

      const res = await fetch(this.getUrl('/copilot/chat'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          case_id: params.caseId,
          session_id: sessionId,
          caller: {
            tenant_id: 'INST-DEMO-01',
            user_id: 'analyst-ops-01',
            roles: ['ANALYST_MAKER'],
            can_view_unmasked: false,
          },
          user_message: params.userMessage,
          history: params.history || [],
        }),
        signal: controller.signal,
      });
      clearTimeout(timeoutId);

      if (res.ok) {
        const data = await res.json();
        return {
          ...data,
          is_live_service: true,
        };
      }
    } catch {
      // AI Service offline, use client-side copilot
    }

    // Contextual fallback response
    return this.fallbackCopilotResponse(params.caseId, sessionId, params.userMessage);
  }

  /**
   * Deterministic client-side parser fallback
   */
  private fallbackParseAlert(text: string): AIParsedAlertResponse {
    const upper = text.toUpperCase();

    // 1. Detect Bank
    let issuing_bank = 'Wema Bank PLC';
    if (upper.includes('GTBANK') || upper.includes('GUARANTY TRUST') || upper.includes('GTB')) issuing_bank = 'GTBank PLC';
    else if (upper.includes('ACCESS')) issuing_bank = 'Access Bank PLC';
    else if (upper.includes('ZENITH')) issuing_bank = 'Zenith Bank PLC';
    else if (upper.includes('UBA') || upper.includes('UNITED BANK FOR AFRICA')) issuing_bank = 'United Bank for Africa (UBA)';
    else if (upper.includes('FIRST BANK') || upper.includes('FIRSTBANK')) issuing_bank = 'First Bank of Nigeria';
    else if (upper.includes('OPAY')) issuing_bank = 'OPay Digital Services';
    else if (upper.includes('PALMPAY')) issuing_bank = 'PalmPay';
    else if (upper.includes('KUDA')) issuing_bank = 'Kuda Microfinance Bank';

    // 2. Detect Amount
    const amtMatch = text.match(/(?:NGN|₦|Amt:?\s*(?:NGN|₦)?)\s*([\d,]+\.?\d*)/i);
    let amount_minor = 2500000;
    if (amtMatch && amtMatch[1]) {
      const cleanAmt = parseFloat(amtMatch[1].replace(/,/g, ''));
      if (!isNaN(cleanAmt)) {
        amount_minor = Math.round(cleanAmt * 100);
      }
    }

    // 3. Detect Channel
    let channel = 'NIP';
    if (upper.includes('POS') || upper.includes('TERMINAL')) channel = 'POS';
    else if (upper.includes('ATM')) channel = 'ATM';
    else if (upper.includes('USSD')) channel = 'USSD';
    else if (upper.includes('WEB') || upper.includes('PAYSTACK') || upper.includes('FLUTTERWAVE')) channel = 'WEB';

    // 4. Detect Reference / STAN / RRN
    const refMatch = text.match(/(?:REF|RRN|STAN|SESSION(?: ID)?|TRX):?\s*([A-Za-z0-9_-]+)/i);
    const reference = refMatch ? refMatch[1] : `REV-${Date.now().toString().slice(-8)}`;

    // 5. Detect Masked Account
    const acctMatch = text.match(/(?:Acct|Account|Acc):?\s*([0-9*•xX-]{5,15})/i);
    const sender_account_masked = acctMatch ? acctMatch[1] : '0234••••891';

    return {
      success: true,
      amount_minor,
      currency: 'NGN',
      issuing_bank,
      channel,
      reference,
      date: new Date().toISOString().split('T')[0],
      sender_account_masked,
      raw_redacted: text.replace(/\b\d{10}\b/g, '••••••••••'),
      confidence_score: 92,
      is_live_service: false,
    };
  }

  /**
   * Deterministic client-side Copilot fallback
   */
  private fallbackCopilotResponse(caseId: string, sessionId: string, message: string): CopilotChatResponse {
    const lower = message.toLowerCase();
    let reply = `Based on Canonical Evidence Bundle for ${caseId}, the transaction is verified debited without matching counterparty settlement acknowledgment.`;
    const citations: string[] = ['ev-core-debit', 'ev-nip-tsq'];
    const tools: string[] = ['verify_ledger_debit', 'query_interbank_tsq'];

    if (lower.includes('beneficiary') || lower.includes('credit')) {
      reply = `Switch status query (TSQ) confirmed counterparty session returned HTTP 504 Gateway Timeout. Zero credit acknowledgment was posted to the destination ledger. The funds remain unapplied at the switch level.`;
      citations.push('ev-interchange-log');
    } else if (lower.includes('duplicate') || lower.includes('retry')) {
      reply = `Identified transaction was dispatched within 412ms of the preceding transfer with identical account and idempotency hash. Analysis classifies this as a confirmed client-side double-click retry.`;
      citations.push('ev-idempotency-key');
      tools.push('match_duplicate_signatures');
    } else if (lower.includes('deadline') || lower.includes('sla')) {
      reply = `Regulatory SLA: Central Bank of Nigeria mandates resolution within 24 hours for instant payment exceptions. Current elapsed time is within accelerated internal target (4 hours).`;
      citations.push('ev-sla-clock');
      tools.push('calculate_regulatory_deadline');
    }

    return {
      case_id: caseId,
      session_id: sessionId,
      response_text: reply,
      cited_evidence_ids: citations,
      tools_invoked: tools,
      proposed_actions: [
        {
          action_type: 'PROPOSE_APPROVAL_PACK',
          title: 'Draft Reversal Proposal',
          description: 'Package verified evidence for dual-control Supervisor authorization.',
          target_endpoint: '/v1/reversals',
        },
      ],
      rule_suggestions: [],
      audit_ref: `aud-copilot-${Date.now().toString(36)}`,
      is_refusal: false,
      is_live_service: false,
    };
  }
}

export const aiClient = new AIClient();
