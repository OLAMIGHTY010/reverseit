import { describe, it, expect } from 'vitest';
import {
  MODEL_REGISTRY,
  FEATURE_DRIFT_METRICS,
  SHADOW_PREDICTIONS,
} from '../features/models/modelsData';

describe('Screens 17 & 18: Model Performance, Concept Drift & Shadow Matrix (Prompt F9)', () => {
  describe('Screen 17: Model Registry & Concept Drift Monitor', () => {
    it('contains baseline rule engine and candidate ML models', () => {
      const activeProd = MODEL_REGISTRY.find((m) => m.status === 'ACTIVE_PRODUCTION');
      expect(activeProd).toBeDefined();
      expect(activeProd?.modelName).toContain('Rule Engine');

      const shadowCandidates = MODEL_REGISTRY.filter((m) => m.status === 'SHADOW_CANDIDATE');
      expect(shadowCandidates.length).toBeGreaterThanOrEqual(2);
      expect(shadowCandidates.some((m) => m.modelName.includes('XGBoost'))).toBe(true);
    });

    it('validates ML metrics meet production candidate thresholds', () => {
      const xgboost = MODEL_REGISTRY.find((m) => m.modelName.includes('XGBoost'))!;
      expect(xgboost.rocAuc).toBeGreaterThan(0.95);
      expect(xgboost.accuracyPct).toBeGreaterThan(95.0);
      expect(xgboost.f1Score).toBeGreaterThan(0.95);
    });

    it('monitors Population Stability Index (PSI) drift boundaries', () => {
      FEATURE_DRIFT_METRICS.forEach((f) => {
        expect(f.psiScore).toBeGreaterThanOrEqual(0);
        if (f.psiScore < 0.1) {
          expect(f.driftStatus).toBe('STABLE');
        } else if (f.psiScore < 0.25) {
          expect(f.driftStatus).toBe('MODERATE_DRIFT');
        } else {
          expect(f.driftStatus).toBe('SIGNIFICANT_DRIFT');
        }
      });
    });
  });

  describe('Screen 18: Shadow Mode Tri-Model Comparison Matrix', () => {
    it('compares production and shadow predictions across real transactions', () => {
      expect(SHADOW_PREDICTIONS.length).toBeGreaterThanOrEqual(5);

      const consensusPredictions = SHADOW_PREDICTIONS.filter((p) => p.isConsensus);
      expect(consensusPredictions.length).toBeGreaterThan(0);

      // Verify consensus logic
      consensusPredictions.forEach((p) => {
        expect(p.ruleEngineVerdict).toBe(p.randomForestVerdict);
        expect(p.randomForestVerdict).toBe(p.xgboostVerdict);
      });
    });

    it('identifies divergent predictions where ML confidence exceeds manual ceiling', () => {
      const divergent = SHADOW_PREDICTIONS.filter((p) => !p.isConsensus);
      expect(divergent.length).toBeGreaterThan(0);

      const sampleDivergent = divergent[0];
      expect(sampleDivergent.divergenceNotes).toBeDefined();
      expect(sampleDivergent.confidenceScorePct).toBeGreaterThan(90);
    });
  });
});
