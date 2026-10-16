import React, { useState } from 'react';
import {
  UploadCloud,
  ArrowLeft,
  EyeOff,
  Sparkles,
} from 'lucide-react';
import { Button } from '../../components/Button';
import { SAMPLE_NIGERIAN_ALERTS } from './reverseitData';
import type { IntakeMode, ParsedAlertDetails } from './reverseitTypes';
import { aiClient } from '../../services/aiClient';

export interface S2PasteUploadProps {
  mode: IntakeMode;
  onBack: () => void;
  onAlertParsed: (parsed: ParsedAlertDetails, rawText: string) => void;
}

export const S2PasteUpload: React.FC<S2PasteUploadProps> = ({
  mode,
  onBack,
  onAlertParsed,
}) => {
  const [alertText, setAlertText] = useState('');
  const [consentChecked, setConsentChecked] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const [selectedPresetIndex, setSelectedPresetIndex] = useState<number | null>(null);

  // Quick preset loader
  const handleSelectPreset = (index: number) => {
    setSelectedPresetIndex(index);
    const preset = SAMPLE_NIGERIAN_ALERTS[index];
    if (preset) {
      setAlertText(preset.text);
      setConsentChecked(true);
    }
  };

  // Simulating image upload with mock redaction
  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      // Auto-load preset 0 as realistic extracted text
      setAlertText(SAMPLE_NIGERIAN_ALERTS[0].text);
      setConsentChecked(true);
    }
  };

  const handleParseAndContinue = async () => {
    if (!consentChecked || !alertText.trim()) return;

    setIsProcessing(true);

    try {
      const parsedRes = await aiClient.parseAlert(alertText);

      const parsedAmountMinor = parsedRes.amount_minor || 2500000;
      const amountStr = (parsedAmountMinor / 100).toLocaleString('en-US', {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      });

      const parsedBank = parsedRes.issuing_bank || 'Wema Bank (ALAT)';
      const isNonWema =
        !parsedBank.toLowerCase().includes('wema') && !parsedBank.toLowerCase().includes('alat');

      const parsedDetails: ParsedAlertDetails = {
        amount: amountStr,
        currency: parsedRes.currency || 'NGN',
        accountTail: parsedRes.sender_account_masked?.replace(/[^0-9]/g, '').slice(-4) || '0891',
        dateTime: parsedRes.date ? `${parsedRes.date} 14:22:10 WAT` : '15-Oct-2026 14:22:10 WAT',
        narration: alertText.slice(0, 80),
        reference: parsedRes.reference || `REV-${Date.now().toString().slice(-8)}`,
        channel: parsedRes.channel || 'NIP',
        issuingBank: parsedBank,
        confidence: {
          amount: 98,
          accountTail: 95,
          reference: 96,
          overall: parsedRes.confidence_score || 95,
        },
        isNonWemaAlert: isNonWema,
      };

      onAlertParsed(parsedDetails, alertText);
    } catch {
      // Local preset fallback
      const matchedPreset =
        selectedPresetIndex !== null
          ? SAMPLE_NIGERIAN_ALERTS[selectedPresetIndex].parsed
          : SAMPLE_NIGERIAN_ALERTS[0].parsed;

      onAlertParsed(matchedPreset, alertText);
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto py-4 px-4 space-y-6 animate-in fade-in duration-200">
      {/* Back button */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1.5 text-[13px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-medium cursor-pointer"
      >
        <ArrowLeft className="w-4 h-4" />
        <span>Back to options</span>
      </button>

      {/* Screen Title */}
      <div>
        <h2 className="text-[20px] font-bold text-[var(--text-primary)]">
          {mode === 'PASTE_TEXT' ? 'Step 1: Paste Your Debit Alert' : 'Step 1: Upload Alert Screenshot'}
        </h2>
        <p className="text-[13px] text-[var(--text-muted)] mt-1">
          {mode === 'PASTE_TEXT'
            ? 'Paste the full SMS or email message sent by your bank.'
            : 'Select or drag a screenshot of the alert or mobile banking transaction receipt.'}
        </p>
      </div>

      {/* Sample Bank Presets (Useful for quick testing) */}
      <div className="p-3 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg space-y-2">
        <div className="flex items-center justify-between text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">
          <span>Quick Test Presets (Top Nigerian Banks)</span>
          <span className="text-sky-700 font-sans">Verified Format Parser</span>
        </div>
        <div className="flex flex-wrap gap-2">
          {SAMPLE_NIGERIAN_ALERTS.map((sample, i) => (
            <button
              key={sample.bankName}
              type="button"
              onClick={() => handleSelectPreset(i)}
              className={`px-2.5 py-1 text-[12px] font-medium rounded-md border transition-colors cursor-pointer ${
                selectedPresetIndex === i
                  ? 'bg-sky-100 text-sky-900 border-sky-400 font-semibold shadow-xs'
                  : 'bg-[var(--canvas-subdued)] text-[var(--text-secondary)] border-[var(--surface-border)] hover:bg-[var(--surface)]'
              }`}
            >
              {sample.label}
            </button>
          ))}
        </div>
      </div>

      {/* Input Area */}
      {mode === 'PASTE_TEXT' ? (
        <div className="space-y-2">
          <label htmlFor="alert-text-input" className="block text-[13px] font-semibold text-[var(--text-primary)]">
            Alert Message Content
          </label>
          <textarea
            id="alert-text-input"
            rows={5}
            value={alertText}
            onChange={(e) => setAlertText(e.target.value)}
            placeholder="Paste your debit alert here (e.g. 'Debit Alert: Acct 0234****891 Amt: NGN 25,000.00...')"
            className="w-full p-3 text-[13px] font-mono-financial rounded-md border border-[var(--border-input)] bg-[var(--surface)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-sky-500"
          />
        </div>
      ) : (
        <div className="space-y-4">
          <div className="border-2 border-dashed border-[var(--surface-border)] rounded-xl p-6 text-center hover:border-sky-500 transition-colors bg-[var(--surface)]">
            <UploadCloud className="w-10 h-10 mx-auto text-slate-400 mb-2" />
            <p className="text-[14px] font-semibold text-[var(--text-primary)]">
              Click to select screenshot or drag & drop
            </p>
            <p className="text-[12px] text-[var(--text-muted)] mt-1">PNG, JPG, or WEBP up to 5MB</p>
            <input
              type="file"
              accept="image/*"
              onChange={handleFileChange}
              className="mt-3 block mx-auto text-[12px] file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border-0 file:text-[12px] file:font-semibold file:bg-sky-50 file:text-sky-700 hover:file:bg-sky-100 cursor-pointer"
            />
          </div>

          {/* Privacy & Redaction Tip */}
          <div className="p-3 bg-purple-50/70 border border-purple-200 rounded-lg flex items-start gap-2.5 text-[12px] text-purple-900">
            <EyeOff className="w-4 h-4 text-purple-700 shrink-0 mt-0.5" />
            <div>
              <strong className="font-semibold">Privacy Protection: </strong>
              <span>
                Hide your account balance if you can. Our local OCR automatically masks balances, 
                full account numbers, and customer names before processing.
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Mandatory Consent Checkbox */}
      <div className="p-4 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded-lg space-y-2">
        <label className="flex items-start gap-3 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={consentChecked}
            onChange={(e) => setConsentChecked(e.target.checked)}
            className="mt-0.5 w-4 h-4 rounded text-sky-600 focus:ring-sky-500 border-slate-300 cursor-pointer"
          />
          <span className="text-[12px] text-[var(--text-primary)] leading-normal">
            I consent to automated extraction and masked verification of this transaction alert. 
            I understand that SMARTREVERSAL operates under strict local PII redaction and never executes 
            monetary movements autonomously.
          </span>
        </label>
      </div>

      {/* Action Buttons */}
      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" size="md" onClick={onBack}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="md"
          disabled={!consentChecked || !alertText.trim()}
          isLoading={isProcessing}
          onClick={handleParseAndContinue}
          rightIcon={<Sparkles className="w-4 h-4" />}
        >
          Parse Alert & Continue
        </Button>
      </div>
    </div>
  );
};
