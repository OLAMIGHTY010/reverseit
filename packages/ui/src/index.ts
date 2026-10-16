/**
 * SMARTREVERSAL Enterprise Design Tokens - Section 9 Specification
 * Visual language: Trust, precision, control, clarity, financial seriousness.
 * Palette: Daylight (soft off-white canvas, white surfaces, deep slate-blue text,
 * clear blue primary, tinted status chips) & Soft Dim (optional mid-tone).
 * Density: Comfortable & Compact.
 * WCAG 2.2 AA / AAA compliant contrast tokens.
 */

export type ThemeMode = 'daylight' | 'soft-dim';
export type DensityMode = 'comfortable' | 'compact';

export type FinancialStatus =
  | 'detected'
  | 'awaiting'
  | 'investigating'
  | 'approval'
  | 'submitted'
  | 'verifying'
  | 'resolved'
  | 'failed'
  | 'suppressed';

export interface StatusTokenConfig {
  id: FinancialStatus;
  label: string;
  icon: string;
  description: string;
  daylight: {
    bg: string;
    border: string;
    text: string;
  };
  softDim: {
    bg: string;
    border: string;
    text: string;
  };
}

export const statusTokens: Record<FinancialStatus, StatusTokenConfig> = {
  detected: {
    id: 'detected',
    label: 'Detected',
    icon: 'Eye',
    description: 'Discrepancy ingested from processor feed or webhook; pending triage',
    daylight: {
      bg: '#f1f5f9',
      border: '#cbd5e1',
      text: '#0f172a', // 15.6:1 AAA
    },
    softDim: {
      bg: '#1e293b',
      border: '#475569',
      text: '#f8fafc',
    },
  },
  awaiting: {
    id: 'awaiting',
    label: 'Awaiting Action',
    icon: 'Clock',
    description: 'Queued for operational intake or counterparty settlement file',
    daylight: {
      bg: '#fef3c7',
      border: '#fcd34d',
      text: '#78350f', // 8.2:1 AAA
    },
    softDim: {
      bg: '#451a03',
      border: '#b45309',
      text: '#fef3c7',
    },
  },
  investigating: {
    id: 'investigating',
    label: 'Investigating',
    icon: 'Search',
    description: 'Active case opened; analyst reviewing transaction telemetry',
    daylight: {
      bg: '#e0f2fe',
      border: '#7dd3fc',
      text: '#0369a1', // 6.5:1 AA+
    },
    softDim: {
      bg: '#082f49',
      border: '#0284c7',
      text: '#e0f2fe',
    },
  },
  approval: {
    id: 'approval',
    label: 'Pending Approval',
    icon: 'ShieldAlert',
    description: 'Maker drafted reversal; awaiting dual-authorization Checker sign-off',
    daylight: {
      bg: '#f3e8ff',
      border: '#d8b4fe',
      text: '#6b21a8', // 7.5:1 AAA
    },
    softDim: {
      bg: '#3b0764',
      border: '#9333ea',
      text: '#f3e8ff',
    },
  },
  submitted: {
    id: 'submitted',
    label: 'Submitted',
    icon: 'Send',
    description: 'Reversal payload dispatched to core banking switch / NIP adapter',
    daylight: {
      bg: '#ccfbf1',
      border: '#5eead4',
      text: '#115e59', // 7.4:1 AAA
    },
    softDim: {
      bg: '#042f2e',
      border: '#0d9488',
      text: '#ccfbf1',
    },
  },
  verifying: {
    id: 'verifying',
    label: 'Verifying',
    icon: 'RefreshCw',
    description: 'Dispatched to gateway; awaiting callback/acknowledgment hook',
    daylight: {
      bg: '#ede9fe',
      border: '#c4b5fd',
      text: '#5b21b6', // 8.0:1 AAA
    },
    softDim: {
      bg: '#2e1065',
      border: '#7c3aed',
      text: '#ede9fe',
    },
  },
  resolved: {
    id: 'resolved',
    label: 'Resolved',
    icon: 'CheckCircle2',
    description: 'Ledger balanced, accounts credited/debited, audit trail sealed',
    daylight: {
      bg: '#dcfce7',
      border: '#86efac',
      text: '#14532d', // 9.1:1 AAA
    },
    softDim: {
      bg: '#052e16',
      border: '#16a34a',
      text: '#dcfce7',
    },
  },
  failed: {
    id: 'failed',
    label: 'Failed',
    icon: 'AlertOctagon',
    description: 'Switch rejected reversal or timeout exceeded; escalated to supervisor',
    daylight: {
      bg: '#fee2e2',
      border: '#fca5a5',
      text: '#991b1b', // 7.2:1 AAA
    },
    softDim: {
      bg: '#450a0a',
      border: '#dc2626',
      text: '#fee2e2',
    },
  },
  suppressed: {
    id: 'suppressed',
    label: 'Suppressed',
    icon: 'ShieldOff',
    description: 'Known simulator noise, verified false positive or business rule exception',
    daylight: {
      bg: '#f1f5f9',
      border: '#cbd5e1',
      text: '#475569', // 4.8:1 AA
    },
    softDim: {
      bg: '#1e293b',
      border: '#334155',
      text: '#94a3b8',
    },
  },
};

export const palette = {
  daylight: {
    canvas: '#f8fafc',
    canvasSubdued: '#f1f5f9',
    surface: '#ffffff',
    surfaceRaised: '#f8fafc',
    surfaceBorder: '#cbd5e1',
    surfaceBorderSubtle: '#e2e8f0',
    primary: '#0284c7', // Sky-600
    primaryHover: '#0369a1',
    primaryActive: '#075985',
    primarySubtle: '#e0f2fe',
    textPrimary: '#0f172a', // Deep slate-blue (never pure black)
    textSecondary: '#334155',
    textMuted: '#64748b',
    focusRing: '#0284c7',
  },
  softDim: {
    canvas: '#0f172a',
    canvasSubdued: '#1e293b',
    surface: '#1e293b',
    surfaceRaised: '#334155',
    surfaceBorder: '#475569',
    surfaceBorderSubtle: '#334155',
    primary: '#38bdf8', // Sky-400
    primaryHover: '#0ea5e9',
    primaryActive: '#0284c7',
    primarySubtle: '#082f49',
    textPrimary: '#f8fafc',
    textSecondary: '#cbd5e1',
    textMuted: '#94a3b8',
    focusRing: '#38bdf8',
  },
} as const;

export const typography = {
  tableDense: { fontSize: '13px', lineHeight: '18px', fontWeight: '400' },
  bodySmall: { fontSize: '13px', lineHeight: '18px', fontWeight: '400' },
  body: { fontSize: '14px', lineHeight: '20px', fontWeight: '400' },
  bodyMedium: { fontSize: '14px', lineHeight: '20px', fontWeight: '500' },
  bodySemibold: { fontSize: '14px', lineHeight: '20px', fontWeight: '600' },
  subheading: { fontSize: '16px', lineHeight: '24px', fontWeight: '600' },
  h3: { fontSize: '18px', lineHeight: '26px', fontWeight: '600' },
  h2: { fontSize: '20px', lineHeight: '28px', fontWeight: '600' },
  h1: { fontSize: '24px', lineHeight: '32px', fontWeight: '700' },
  display: { fontSize: '30px', lineHeight: '36px', fontWeight: '700' },
  mono: { fontFamily: "'JetBrains Mono', 'Fira Code', ui-monospace, monospace" },
} as const;

export const spacing = {
  '2xs': '4px',
  xs: '8px',
  sm: '12px',
  md: '16px',
  lg: '20px',
  xl: '24px',
  '2xl': '32px',
  '3xl': '40px',
  '4xl': '48px',
} as const;

export const breakpoints = {
  sm: '640px',
  md: '768px',
  lg: '1024px',
  xl: '1280px',
  '2xl': '1536px',
} as const;

export const shadows = {
  none: 'none',
  sm: '0 1px 2px 0 rgba(15, 23, 42, 0.05)',
  md: '0 4px 6px -1px rgba(15, 23, 42, 0.08), 0 2px 4px -2px rgba(15, 23, 42, 0.04)',
  lg: '0 10px 15px -3px rgba(15, 23, 42, 0.08), 0 4px 6px -4px rgba(15, 23, 42, 0.03)',
  drawer: '-4px 0 24px rgba(15, 23, 42, 0.12)',
  modal: '0 20px 25px -5px rgba(15, 23, 42, 0.15)',
} as const;

/**
 * Calculates relative luminance for WCAG contrast calculation
 */
export function getLuminance(hex: string): number {
  const cleanHex = hex.replace('#', '');
  const r = parseInt(cleanHex.substring(0, 2), 16) / 255;
  const g = parseInt(cleanHex.substring(2, 4), 16) / 255;
  const b = parseInt(cleanHex.substring(4, 6), 16) / 255;

  const toLinear = (c: number) =>
    c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);

  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
}

/**
 * Calculates WCAG 2.2 contrast ratio between two hex colors
 */
export function getContrastRatio(hex1: string, hex2: string): number {
  const lum1 = getLuminance(hex1);
  const lum2 = getLuminance(hex2);
  const brighter = Math.max(lum1, lum2);
  const darker = Math.min(lum1, lum2);
  return Number(((brighter + 0.05) / (darker + 0.05)).toFixed(2));
}

export function getStatusConfig(status: FinancialStatus): StatusTokenConfig {
  return statusTokens[status] || statusTokens.detected;
}
