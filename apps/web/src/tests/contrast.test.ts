import { describe, it, expect } from 'vitest';
import {
  palette,
  statusTokens,
  getContrastRatio,
  type FinancialStatus,
} from '@smartreversal/ui';

describe('WCAG 2.2 Token Contrast CI Test Suite (Section 9 Palette)', () => {
  describe('Daylight Theme (Default)', () => {
    it('primary text meets WCAG AAA standard (>= 7:1) on white surface', () => {
      const ratio = getContrastRatio(palette.daylight.textPrimary, palette.daylight.surface);
      expect(ratio).toBeGreaterThanOrEqual(7.0);
    });

    it('secondary text meets WCAG AAA standard (>= 7:1) on white surface', () => {
      const ratio = getContrastRatio(palette.daylight.textSecondary, palette.daylight.surface);
      expect(ratio).toBeGreaterThanOrEqual(7.0);
    });

    it('muted text meets WCAG AA standard (>= 4.5:1) on white surface', () => {
      const ratio = getContrastRatio(palette.daylight.textMuted, palette.daylight.surface);
      expect(ratio).toBeGreaterThanOrEqual(4.5);
    });

    it('primary blue action text satisfies graphical / component contrast (>= 3:1)', () => {
      const ratio = getContrastRatio('#ffffff', palette.daylight.primary);
      expect(ratio).toBeGreaterThanOrEqual(3.0);
    });
  });

  describe('Soft Dim Theme (Mid-Tone Optional)', () => {
    it('primary text meets WCAG AAA standard (>= 7:1) on dark surface', () => {
      const ratio = getContrastRatio(palette.softDim.textPrimary, palette.softDim.surface);
      expect(ratio).toBeGreaterThanOrEqual(7.0);
    });

    it('secondary text meets WCAG AAA standard (>= 7:1) on dark surface', () => {
      const ratio = getContrastRatio(palette.softDim.textSecondary, palette.softDim.surface);
      expect(ratio).toBeGreaterThanOrEqual(7.0);
    });
  });

  describe('Semantic Status Chips (All 9 States in Daylight)', () => {
    const statuses = Object.keys(statusTokens) as FinancialStatus[];

    statuses.forEach((status) => {
      const config = statusTokens[status];
      it(`chip "${config.label}" text meets minimum WCAG AA ratio (>= 4.5:1) against chip background`, () => {
        const ratio = getContrastRatio(config.daylight.text, config.daylight.bg);
        expect(ratio).toBeGreaterThanOrEqual(4.5);
      });
    });
  });
});
