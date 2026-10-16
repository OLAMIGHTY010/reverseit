import { test, expect } from '@playwright/test';

test.describe('First-Contact Agent Intake & Reversal Governance (F4 Screens 7, 8, 11)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Switch to Contact Centre persona to ensure representative view
    const personaSelect = page.locator('select').first();
    if (await personaSelect.isVisible()) {
      await personaSelect.selectOption('CONTACT_CENTRE_AGENT');
    }
    // Navigate to Agent Intake panel
    await page.click('button:has-text("Agent Intake")');
  });

  test('verifies agent panel header, identifier search tabs, and simulated 403 permission denied toggle', async ({ page }) => {
    // Header information
    await expect(page.locator('text=First-Contact Agent Intake & Query Resolution Panel')).toBeVisible();
    await expect(page.locator('text=Contact-Centre / Branch')).toBeVisible();

    // 3 Permitted Identifier Search Tabs
    await expect(page.locator('button:has-text("Reference / Session ID")')).toBeVisible();
    await expect(page.locator('button:has-text("Date / Amount / Channel")')).toBeVisible();
    await expect(page.locator('button:has-text("Tokenised Customer")')).toBeVisible();

    // Permission Denied Toggle
    await page.click('button:has-text("Simulate 403 Forbidden")');
    await expect(page.locator('text=Permission Denied: Agent View Restricted')).toBeVisible();

    // Reset Permission
    await page.click('button:has-text("Reset Permission State")');
    await expect(page.locator('text=First-Contact Agent Intake & Query Resolution Panel')).toBeVisible();
  });

  test('State A: resolves active recon exception with safe status, SLA timer, incident banner, and non-refund wording', async ({ page }) => {
    // Click rapid preset for TXN-9901
    await page.click('button:has-text("TXN-9901")');
    await page.click('button:has-text("Query Switch Journal")');

    // Result header & safe status
    await expect(page.locator('text=Active Reconciliation Exception Match')).toBeVisible();
    await expect(page.locator('text=Target resolution within 4 hours')).toBeVisible();

    // Incident Banner INC-8821
    await expect(page.locator('text=INC-8821')).toBeVisible();
    await expect(page.locator('text=Intermittent NIBSS Outward Gateway timeout')).toBeVisible();

    // Approved Wording Card
    const wordingBlock = page.locator('div:has-text("Approved Customer Speech Wording")').first();
    await expect(wordingBlock).toBeVisible();
    // Compliance check: MUST NOT contain "refunded"
    const wordingText = await wordingBlock.innerText();
    expect(wordingText.toLowerCase()).not.toContain('refunded');

    // 1-click Copy button
    await expect(page.locator('button:has-text("Copy Approved Wording")')).toBeVisible();
  });

  test('State B: resolves verified successful transaction with quotable settlement proof and session ID', async ({ page }) => {
    // Click rapid preset for TXN-9902
    await page.click('button:has-text("TXN-9902")');
    await page.click('button:has-text("Query Switch Journal")');

    // Verified Success Card
    await expect(page.locator('text=Transaction Verified Successful & Delivered')).toBeVisible();
    await expect(page.locator('text=Core Ledger Single Debit Confirmed')).toBeVisible();
    await expect(page.locator('text=Beneficiary Credit Confirmed at Counterparty')).toBeVisible();
    await expect(page.locator('text=99901426101509294288420')).toBeVisible();

    // Copy Quotable Evidence button
    await expect(page.locator('button:has-text("Copy Quotable Settlement Evidence")')).toBeVisible();
  });

  test('State D: resolves ambiguous criteria into disambiguation candidates and drills into selected candidate', async ({ page }) => {
    // Click rapid preset for ₦45,000 POS
    await page.click('button:has-text("₦45,000 POS")');
    await page.click('button:has-text("Query Switch Journal")');

    // Multiple candidates disambiguation card
    await expect(page.locator('text=Multiple Candidate Transactions Found')).toBeVisible();
    await expect(page.locator('text=TXN-20261015-POS-8801')).toBeVisible();
    await expect(page.locator('text=TXN-20261015-POS-8802')).toBeVisible();

    // Drill into candidate 1
    const drillButton = page.locator('button:has-text("Select & Investigate Candidate")').first();
    await drillButton.click();
  });

  test('State C: handles unknown reference with structured customer dispute intake form (CUS001)', async ({ page }) => {
    // Click rapid preset for TXN-UNKNOWN
    await page.click('button:has-text("TXN-UNKNOWN")');
    await page.click('button:has-text("Query Switch Journal")');

    // No Match card & Intake Form
    await expect(page.locator('text=No Matching Transaction Located')).toBeVisible();
    await expect(page.locator('text=Customer Dispute Intake Form (CUS001)')).toBeVisible();

    // Fill intake form
    await page.fill('input[placeholder="e.g. 0123456789"]', '0199887766');
    await page.fill('input[placeholder="e.g. +234 803 123 4567"]', '+234 809 999 8888');
    await page.fill('input[placeholder="e.g. 50000.00"]', '35000.00');
    await page.fill('textarea[placeholder*="Describe the customer\'s dispute"]', 'Customer attempted off-us ATM withdrawal in Abuja. Debited but no cash dispensed.');

    // Submit dispute docket
    await page.click('button:has-text("Create Customer Dispute Docket (CUS001)")');

    // Success toast confirmation
    await expect(page.locator('text=Customer Dispute Docket Created')).toBeVisible();
  });

  test('Repeat Contact Tracking: detects threshold (>= 3 inquiries) and allows priority supervisor escalation', async ({ page }) => {
    // Search repeat customer phone
    await page.click('button:has-text("+234 809 111 2233 (3x Repeat)")');
    await page.click('button:has-text("Query Switch Journal")');

    // Repeat contact indicator
    await expect(page.locator('text=Repeat Contacts in 24h')).toBeVisible();
    await expect(page.locator('text=Supervisor Escalation Required')).toBeVisible();

    // Click Escalate to Supervisor Queue
    await page.click('button:has-text("Escalate to Supervisor Queue")');

    // Modal dialog pops up
    await expect(page.locator('text=Escalate Dispute to Tier-2 Supervisor Queue')).toBeVisible();
    await page.fill('textarea[placeholder*="Provide context for the supervisor"]', 'Third call in 24 hours. Customer requesting urgent supervisor clearance.');
    await page.click('button:has-text("Confirm Escalation & Log Contact")');

    // Escalated toast confirmation
    await expect(page.locator('text=Escalated to Supervisor Queue')).toBeVisible();
  });

  test('F4 Reversal Queue, Detail and Approval (Screens 7, 8, 11)', async ({ page }) => {
    // Switch to Supervisor Checker persona to access approval features
    const personaSelect = page.locator('select').first();
    if (await personaSelect.isVisible()) {
      await personaSelect.selectOption('SUPERVISOR_CHECKER');
    }

    // Navigate to Reversals View
    await page.click('button:has-text("Reversals")');

    // Screen 7: Proposals Queue & Controls Bar
    await expect(page.locator('text=Reversal Queue, Detail and Approval')).toBeVisible();
    await expect(page.locator('text=TXN-20261015-NIP-9901')).toBeVisible();

    // Click proposal to open Screen 8 Reversal Proposal Audit Dossier
    await page.click('button:has-text("Audit 8 Safety Checks")');

    // Screen 8 Detail Drawer with 8 Financial Integrity safety checks
    await expect(page.locator('text=Reversal Proposal Audit Dossier')).toBeVisible();
    await expect(page.locator('text=S1: Transactional Idempotency Key')).toBeVisible();
    await expect(page.locator('text=S2: Duplicate Reversal Prevention')).toBeVisible();
    await expect(page.locator('text=S3: Core Ledger Single Debit Verified')).toBeVisible();
    await expect(page.locator('text=S4: Counterparty Non-Credit Proof')).toBeVisible();
    await expect(page.locator('text=S5: Dual-Control Policy Threshold')).toBeVisible();
    await expect(page.locator('text=S6: Separation of Duties (Maker != Checker)')).toBeVisible();
    await expect(page.locator('text=S7: Reversal Gateway Kill Switch Inactive')).toBeVisible();
    await expect(page.locator('text=S8: Evidence Freshness Gate')).toBeVisible();

    // Screen 11: Authorize Reversal Action
    await page.click('button:has-text("Authorize Reversal")');

    // Dual-control approval dialog
    await expect(page.locator('text=Dual-Authorization Required')).toBeVisible();
    await page.fill('textarea[placeholder*="Provide audit notes"]', 'Verified all 8 safety checks passed. Counterparty non-credit confirmed.');
    await page.click('button:has-text("Authorize & Dispatch Reversal")');

    // Irreversible confirmation modal
    await expect(page.locator('text=Irreversible Financial Action Confirmation')).toBeVisible();
    await page.click('button:has-text("Yes, Dispatch Reversal")');

    // Reversal dispatched toast
    await expect(page.locator('text=Reversal Dispatched & Settled')).toBeVisible();
  });
});
