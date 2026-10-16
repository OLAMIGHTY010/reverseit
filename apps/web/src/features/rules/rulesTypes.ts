export interface SafetyGateRulesConfig {
  tier1AutoReversalMaxNgn: number; // e.g. 50,000
  tier2SupervisorThresholdNgn: number; // e.g. 500,000
  tier3CommitteeThresholdNgn: number; // e.g. 5,000,000
  minimumModelConfidencePct: number; // e.g. 95
  maxDuplicateWindowSeconds: number; // e.g. 180
  fxRoundingToleranceNgn: number; // e.g. 5.0
  regulatoryUrgentAlertHours: number; // e.g. 4
  requireCounterpartyTsqQuery: boolean;
  enableIdempotencyLock: boolean;
  blockBlacklistedBeneficiaries: boolean;
}

export type CircuitBreakerState = 'CLOSED' | 'HALF_OPEN' | 'OPEN';

export interface AdapterHealthStatus {
  id: string;
  name: string;
  protocol: 'ISO 8583' | 'REST / HTTPS' | 'AS2 / SFTP' | 'gRPC';
  channel: 'CORE_LEDGER' | 'NIP' | 'POS' | 'CARD' | 'ATM';
  endpoint: string;
  status: 'HEALTHY' | 'DEGRADED' | 'DOWN';
  pingLatencyMs: number;
  uptimePct: number; // e.g. 99.98
  circuitBreaker: CircuitBreakerState;
  lastHeartbeat: string;
  activeTps: number;
  queuedRequests: number;
  errorRatePct: number;
}
