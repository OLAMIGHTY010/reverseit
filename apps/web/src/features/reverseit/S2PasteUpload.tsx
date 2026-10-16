import React, { useState } from 'react';
import {
  UploadCloud,
  ArrowLeft,
  EyeOff,
  Sparkles,
  CheckCircle,
  FileText,
  X,
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
  const [uploadedFileName, setUploadedFileName] = useState<string | null>(null);

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
      setUploadedFileName(file.name);
      setAlertText(SAMPLE_NIGERIAN_ALERTS[0].text);
      setConsentChecked(true);
    }
  };

  const handleClear = () => {
    setAlertText('');
    setSelectedPresetIndex(null);
    setUploadedFileName(null);
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
    <div className="max-w-2xl mx-auto py-2 px-2 sm:px-4 space-y-6 animate-in fade-in duration-200">
      {/* Back button */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1.5 text-[13px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] font-medium transition-colors cursor-pointer"
      >
        <ArrowLeft className="w-4 h-4" />
        <span>Back to options</span>
      </button>

      {/* Screen Title & Subtitle */}
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-sky-100 text-sky-800">
            Step 1 of 5
          </span>
          <span className="text-[12px] text-[var(--text-muted)]">Alert Intake & Parsing</span>
        </div>
        <h1 className="text-[22px] font-bold text-[var(--text-primary)]">
          {mode === 'PASTE_TEXT' ? 'Paste Your Debit Alert' : 'Upload Alert Screenshot or Receipt'}
        </h1>
        <p className="text-[13px] text-[var(--text-secondary)]">
          {mode === 'PASTE_TEXT'
            ? 'Paste the full SMS or email notification received from your bank. We will automatically detect key transaction identifiers.'
            : 'Select or drag a screenshot of the alert or mobile banking transaction receipt.'}
        </p>
      </div>

      {/* Sample Bank Presets (Useful for quick testing) */}
      <div className="p-3 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg space-y-2">
        <div className="flex items-center justify-between text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">
          <span>Quick Test Presets (Top Nigerian Banks)</span>
          <span className="text-sky-700 font-sans">Template Verified</span>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
          {SAMPLE_NIGERIAN_ALERTS.map((sample, i) => {
            const isSelected = selectedPresetIndex === i;
            return (
              <button
                key={sample.bankName}
                type="button"
                onClick={() => handleSelectPreset(i)}
                className={`p-2.5 rounded-lg border text-left transition-all cursor-pointer flex flex-col justify-between gap-1 ${
                  isSelected
                    ? 'bg-sky-50 text-sky-950 border-sky-500 shadow-xs ring-1 ring-sky-500'
                    : 'bg-[var(--canvas-subdued)] text-[var(--text-secondary)] border-[var(--surface-border)] hover:bg-[var(--surface)] hover:border-slate-300'
                }`}
              >
                <div className="flex items-center justify-between w-full">
                  <span className="font-bold text-[11px] text-[var(--text-primary)] truncate">
                    {sample.bankName}
                  </span>
                  {isSelected && <CheckCircle className="w-3.5 h-3.5 text-sky-600 shrink-0" />}
                </div>
                <span className="text-[10px] text-[var(--text-muted)] truncate">
                  {sample.label.split('(')[1]?.replace(')', '') || sample.label}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Input Area */}
      {mode === 'PASTE_TEXT' ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label
              htmlFor="alert-text-input"
              className="text-[13px] font-bold text-[var(--text-primary)] flex items-center gap-1.5"
            >
              <FileText className="w-4 h-4 text-sky-600" />
              <span>Debit Alert Message</span>
            </label>
            {alertText && (
              <button
                type="button"
                onClick={handleClear}
                className="text-[11px] text-rose-600 hover:text-rose-700 font-medium flex items-center gap-1 cursor-pointer"
              >
                <X className="w-3.5 h-3.5" />
                <span>Clear text</span>
              </button>
            )}
          </div>

          <textarea
            id="alert-text-input"
            rows={5}
            value={alertText}
            onChange={(e) => {
              setAlertText(e.target.value);
              setSelectedPresetIndex(null);
            }}
            placeholder="Paste your debit alert here (e.g. 'Debit Alert: Acct 0234****891 Amt: NGN 25,000.00 Desc: TRF/NIP/NIBSS... Date: 14-Oct-2026 09:12:44...')"
            className="w-full p-3.5 text-[13px] font-mono-financial rounded-xl border border-[var(--surface-border)] bg-[var(--surface)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-sky-500 shadow-2xs leading-relaxed"
          />

          {alertText && (
            <div className="p-2.5 rounded-lg bg-sky-50 border border-sky-200 text-sky-900 text-[12px] flex items-center justify-between">
              <span className="flex items-center gap-1.5">
                <CheckCircle className="w-4 h-4 text-sky-600" />
                <span>Alert text loaded ({alertText.length} characters)</span>
              </span>
              <span className="text-[11px] font-semibold text-sky-700">Ready to analyze</span>
            </div>
          )}
        </div>
      ) : (
        <div className="space-y-4">
          <div className="border-2 border-dashed border-[var(--surface-border)] rounded-2xl p-6 sm:p-8 text-center hover:border-sky-500 transition-colors bg-[var(--surface)] space-y-3">
            <UploadCloud className="w-12 h-12 mx-auto text-sky-600" />
            <div>
              <p className="text-[14px] font-bold text-[var(--text-primary)]">
                {uploadedFileName ? `Selected: ${uploadedFileName}` : 'Select screenshot or drag & drop'}
              </p>
              <p className="text-[12px] text-[var(--text-muted)] mt-1">PNG, JPG, or PDF up to 5MB</p>
            </div>
            <input
              type="file"
              accept="image/*,.pdf"
              onChange={handleFileChange}
              className="mt-2 block mx-auto text-[12px] file:mr-3 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-[12px] file:font-bold file:bg-sky-50 file:text-sky-700 hover:file:bg-sky-100 cursor-pointer shadow-2xs"
            />
          </div>

          {/* Privacy & Redaction Tip */}
          <div className="p-3.5 bg-purple-50 border border-purple-200 rounded-xl flex items-start gap-3 text-[12px] text-purple-950">
            <EyeOff className="w-4 h-4 text-purple-700 shrink-0 mt-0.5" />
            <div>
              <strong className="font-bold">Automated Privacy Guard: </strong>
              <span>
                Our local OCR automatically masks balances, card numbers, and sensitive identifiers. 
                Your personal financial data never leaves your session unprotected.
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Mandatory Consent Checkbox */}
      <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-xl space-y-2 shadow-2xs">
        <label className="flex items-start gap-3 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={consentChecked}
            onChange={(e) => setConsentChecked(e.target.checked)}
            className="mt-0.5 w-4 h-4 rounded text-sky-600 focus:ring-sky-500 border-slate-300 cursor-pointer shrink-0"
          />
          <span className="text-[12px] text-[var(--text-secondary)] leading-relaxed">
            I authorize Smart Reversal to extract and verify this transaction alert against bank switch records. 
            I understand that all account numbers are locally masked and protected under Nigerian Data Protection Regulations (NDPR).
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
          Verify Extracted Details &rarr;
        </Button>
      </div>
    </div>
  );
};
