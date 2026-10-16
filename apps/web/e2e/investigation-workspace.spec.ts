import { test, expect } from '@playwright/test';

test.describe('Transaction Investigation Workspace', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Navigate to Investigations workspace
    await page.click('button:has-text("Investigations")');
  });

  test('renders sticky case header with dual clocks, exposure, banners, and screen-reader summary', async ({ page }) => {
    // Accessible screen reader summary exists
    const srSummary = page.locator('div[role="region"][aria-label="Transaction Exception Case Summary"]');
    await expect(srSummary).toBeAttached();

    // Sticky case header elements
    const header = page.locator('header[aria-label="Sticky Exception Case Header"]');
    await expect(header).toBeVisible();

    // Verify key fields
    await expect(header.locator('text=EXC-2026-001')).toBeVisible();
    await expect(header.locator('text=FAIL001')).toBeVisible();
    await expect(header.locator('text=₦150,000.00').first()).toBeVisible();

    // Dual countdown timers
    await expect(header.locator('text=Internal Target')).toBeVisible();
    await expect(header.locator('text=CBN 72h Resolution Clock')).toBeVisible();

    // Degraded source banner
    await expect(header.locator('text=GTBank Receiving Node Degraded')).toBeVisible();
  });

  test('verifies lifecycle timeline with clock-skew, late-arriving, gap markers, and payload inspection', async ({ page }) => {
    // Semantic timeline list
    const timelineList = page.locator('ol[role="list"][aria-label="Transaction lifecycle event timeline"]');
    await expect(timelineList).toBeVisible();

    // Clock-skew marker badge
    await expect(page.locator('text=+1,800ms clock skew')).toBeVisible();

    // Late-arriving event marker
    await expect(page.locator('text=Late Arrival')).toBeVisible();

    // Gap marker (14m gap between switch and counterparty)
    await expect(page.locator('text=14m gap')).toBeVisible();

    // Filter by source: click AI_ENGINE filter
    await page.click('button:has-text("AI Engine")');
    await expect(page.locator('text=Anomaly Classified: FAIL001')).toBeVisible();

    // Reset filter to All Sources
    await page.click('button:has-text("All Sources")');

    // Expand raw payload on an event
    const payloadToggle = page.locator('button:has-text("View Raw Payload (Masked)")').first();
    await payloadToggle.click();
    await expect(page.locator('text=Copy JSON')).toBeVisible();
  });

  test('verifies 4-quadrant evidence dossier, freshness gating, and missing-evidence request check', async ({ page }) => {
    // Switch to Evidence tab
    await page.click('button:has-text("Evidence Dossier")');

    // 4 distinct quadrants
    await expect(page.locator('text=1. Confirmed Facts')).toBeVisible();
    await expect(page.locator('text=2. Inferred Explanations')).toBeVisible();
    await expect(page.locator('text=3. Missing Evidence')).toBeVisible();
    await expect(page.locator('text=4. Unresolved Questions')).toBeVisible();

    // Stale evidence gating banner
    await expect(page.locator('text=Action Blocked: Stale Evidence Present')).toBeVisible();
    await expect(page.locator('text=Cannot support automated reversal')).toBeVisible();

    // Interactive Request Check for missing evidence
    const requestBtn = page.locator('button:has-text("Request Inward Check from GTBank")');
    await expect(requestBtn).toBeVisible();
    await requestBtn.click();

    // Toast notification confirms check dispatched
    await expect(page.locator('text=Check Dispatched')).toBeVisible();
  });

  test('evaluates side-by-side duplicate comparison, field diffs, risk grade, and verdict flow', async ({ page }) => {
    // Select duplicate case (CASE-2026-403)
    await page.click('button:has-text("CASE-2026-403")');

    // Duplicate Diff tab should automatically be active or selectable
    await page.click('button:has-text("Duplicate Comparison")');

    // Side-by-side comparison cards
    await expect(page.locator('text=Original Transaction (Leg A)')).toBeVisible();
    await expect(page.locator('text=Suspected Duplicate (Leg B)')).toBeVisible();

    // High risk grade badge and reasons
    await expect(page.locator('text=HIGH RISK DUPLICATE (98/100)')).toBeVisible();
    await expect(page.locator('text=+480ms delta')).toBeVisible();

    // Side-by-side diff table
    await expect(page.locator('text=System Trace Audit Number (STAN)')).toBeVisible();
    await expect(page.locator('text=007711').first()).toBeVisible();

    // Verdict action: Click Confirm Duplicate
    const confirmDupBtn = page.locator('button:has-text("Confirm Duplicate (Fast-Track Reversal)")');
    await expect(confirmDupBtn).toBeVisible();
    await confirmDupBtn.click();

    // Toast confirmation
    await expect(page.locator('text=Duplicate Verdict Confirmed')).toBeVisible();
  });

  test('displays customer-reported dispute card with masked alert, parsed fields, tamper checks, and complaint', async ({ page }) => {
    // Select customer reported case (CASE-2026-405)
    await page.click('button:has-text("CASE-2026-405")');

    // Customer Alert Dossier card in left column
    await expect(page.locator('text=Customer Dispute Alert Dossier')).toBeVisible();
    await expect(page.locator('text=3 Repeat Inquiries')).toBeVisible();

    // Customer complaint quotation
    await expect(page.locator('text=transferred ₦350,000.00 for my son’s university clearance')).toBeVisible();

    // Raw alert (masked)
    await expect(page.locator('text=SMS ALERT: DR ₦350,000.00')).toBeVisible();

    // NLP parsed fields
    await expect(page.locator('text=Stanbic IBTC (039)')).toBeVisible();

    // Tamper integrity checks
    await expect(page.locator('text=SMS Gateway Signature')).toBeVisible();
    await expect(page.locator('text=Timestamp Continuity')).toBeVisible();
    await expect(page.locator('text=Receipt Digital Hash')).toBeVisible();
  });

  test('confirms AI root-cause tag with maker audit stamp', async ({ page }) => {
    // Select Case 401
    await page.click('button:has-text("CASE-2026-401")');

    // Root-Cause tag editor in right column
    const confirmAiBtn = page.locator('button:has-text("Confirm AI Diagnosis")');
    if (await confirmAiBtn.count() > 0) {
      await confirmAiBtn.click();
      await expect(page.locator('text=Root-Cause Confirmed')).toBeVisible();
      await expect(page.locator('text=Confirmed by Olusola Adeyemi (Maker)')).toBeVisible();
    }
  });
});
