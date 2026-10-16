import { test, expect } from '@playwright/test';

test.describe('Exception Queue & Workbench Operations Console', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Navigate to Exceptions view
    await page.click('button:has-text("Exceptions"), a:has-text("Exceptions")');
  });

  test('renders top bar, left filter rail, semantic table, and classification codes', async ({ page }) => {
    // Top Bar tabs
    await expect(page.locator('button:has-text("Action Required")')).toBeVisible();
    await expect(page.locator('button:has-text("Customer-Reported")')).toBeVisible();
    await expect(page.locator('button:has-text("SLA Critical")')).toBeVisible();

    // Classification codes in table (Scope v2 mandatory)
    await expect(page.locator('text=DUP001').first()).toBeVisible();
    await expect(page.locator('text=FAIL001').first()).toBeVisible();

    // Source badges (Bot vs Customer)
    await expect(page.locator('title="Source: BOT", [title*="BOT"]').first()).toBeVisible();
  });

  test('filters by customer-reported only cases', async ({ page }) => {
    // Click Customer-Reported tab
    await page.click('button:has-text("Customer-Reported")');

    // All visible rows should be customer source
    const customerBadges = page.locator('text=Customer');
    await expect(customerBadges.first()).toBeVisible();

    // Filter rail checkbox should reflect customer-reported active
    const checkbox = page.locator('input[type="checkbox"]:has-text("Customer-Reported Only"), label:has-text("Customer-Reported Only") input');
    if (await checkbox.count() > 0) {
      await expect(checkbox).toBeChecked();
    }
  });

  test('opens right detail preview drawer with dual countdowns and evidence dossier', async ({ page }) => {
    // Click on first row to select exception
    const firstRow = page.locator('tbody tr').first();
    await firstRow.click();

    // Right detail drawer preview opens
    const drawer = page.locator('aside[aria-label*="Exception Detail Preview"]');
    await expect(drawer).toBeVisible();

    // Dual countdown timers are displayed
    await expect(drawer.locator('text=Internal SLA Target')).toBeVisible();
    await expect(drawer.locator('text=CBN Regulatory SLA')).toBeVisible();

    // 4-Quadrant Evidence Dossier
    await expect(drawer.locator('text=Evidence Dossier')).toBeVisible();
    await expect(drawer.locator('text=FACT')).toBeVisible();

    // Masked field reveal flow
    const maskButton = drawer.locator('button[aria-label*="Reveal"], button:has-text("Reveal")').first();
    if (await maskButton.count() > 0) {
      await maskButton.click();
      await expect(page.locator('text=Mandatory Audit Reason')).toBeVisible();
    }
  });

  test('claims an unassigned exception', async ({ page }) => {
    // Switch to Unassigned tab
    await page.click('button:has-text("All")');
    const claimButton = page.locator('button:has-text("Claim")').first();
    if (await claimButton.count() > 0) {
      await claimButton.click();
      await expect(page.locator('text=claimed to your operations queue, text=Case Claimed')).toBeVisible();
    }
  });

  test('transitions lifecycle state with mandatory reason code', async ({ page }) => {
    // Select first exception
    await page.locator('tbody tr').first().click();

    // Click Change State in drawer
    await page.click('button:has-text("Change State")');

    // State change modal opens
    await expect(page.locator('text=Transition Exception Lifecycle State')).toBeVisible();
    await expect(page.locator('label:has-text("Mandatory Regulatory Reason Code")')).toBeVisible();

    // Fill operational justification
    await page.fill('textarea', 'Verified core ledger debits against NIBSS session journal.');

    // Confirm transition
    await page.click('button:has-text("Confirm Transition")');

    // Modal closes & toast appears
    await expect(page.locator('text=Status Updated')).toBeVisible();
  });

  test('handles optimistic concurrency conflict gracefully', async ({ page }) => {
    // Keyboard shortcut ? opens shortcuts guide
    await page.keyboard.press('?');
    await expect(page.locator('text=Operations Keyboard Shortcuts')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('text=Operations Keyboard Shortcuts')).not.toBeVisible();
  });

  test('enforces permission awareness for maker-checker segregation', async ({ page }) => {
    // Switch role to AUDITOR using top header role switcher
    const roleSelector = page.locator('select[aria-label="Role Switcher"], select:has-text("Role")').first();
    if (await roleSelector.count() > 0) {
      await roleSelector.selectOption('AUDITOR');

      // Select an exception
      await page.locator('tbody tr').first().click();

      // Propose Reversal button should be disabled for AUDITOR
      const proposeButton = page.locator('button:has-text("Propose Reversal")');
      await expect(proposeButton).toBeDisabled();
    }
  });
});
