export interface ModelMetricSnapshot {
  modelName: string;
  version: string;
  deployedAt: string;
  accuracyPct: number;
  precisionPct: number;
  recallPct: number;
  f1Score: number;
  rocAuc: number;
  status: 'ACTIVE_PRODUCTION' | 'SHADOW_CANDIDATE' | 'RETIRED';
}

export interface FeatureDriftPsi {
  featureName: string;
  category: 'FINANCIAL' | 'TEMPORAL' | 'NETWORK' | 'COUNTERPARTY';
  baselineDistributionMean: number;
  currentDistributionMean: number;
  psiScore: number;
  driftStatus: 'STABLE' | 'MODERATE_DRIFT' | 'SIGNIFICANT_DRIFT';
  shapImportancePct: number;
}

export interface ShadowPredictionComparison {
  id: string;
  transactionRef: string;
  channel: string;
  amountNgn: string;
  timestamp: string;
  ruleEngineVerdict: 'AUTO_REVERSE' | 'MANUAL_INVESTIGATION' | 'BLOCK_DECLINE';
  randomForestVerdict: 'AUTO_REVERSE' | 'MANUAL_INVESTIGATION' | 'BLOCK_DECLINE';
  xgboostVerdict: 'AUTO_REVERSE' | 'MANUAL_INVESTIGATION' | 'BLOCK_DECLINE';
  isConsensus: boolean;
  divergenceNotes?: string;
  confidenceScorePct: number;
}
