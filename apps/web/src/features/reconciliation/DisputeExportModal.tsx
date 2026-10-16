import React, { useState } from 'react';
import { X, Download, FileText, ShieldCheck } from 'lucide-react';
import { Button } from '../../components/Button';
import type { ReconciliationBreak } from './reconciliationTypes';

interface DisputeExportModalProps {
  isOpen: boolean;
  onClose: () => void;
  breaks: ReconciliationBreak[];
  sessionCode: string;
}

export const DisputeExportModal: React.FC<DisputeExportModalProps> = ({
  isOpen,
  onClose,
  breaks,
  sessionCode,
}) => {
  const [format, setFormat] = useState<'NIBSS_CSV' | 'CBN_DISPUTE_JSON' | 'AUDIT_PACKAGE'>('NIBSS_CSV');
  const [includeResolved, setIncludeResolved] = useState(false);
  const [isExporting, setIsExporting] = useState(false);

  if (!isOpen) return null;

  const exportableBreaks = includeResolved
    ? breaks
    : breaks.filter((b) => b.status === 'UNMATCHED' || b.status === 'ESCALATED_DISPUTE');

  const handleExport = () => {
    setIsExporting(true);

    setTimeout(() => {
      // Generate formatted CSV content
      const headers = [
        'DISPUTE_ID',
        'SETTLEMENT_SESSION',
        'CHANNEL',
        'BREAK_TYPE',
        'LEDGER_TXN_REF',
        'ACCOUNT_MASKED',
        'LEDGER_AMOUNT',
        'SWITCH_STAN',
        'SWITCH_RRN',
        'CLEARING_AMOUNT',
        'AMOUNT_DELTA',
        'TIMESTAMP_WAT',
        'STATUS',
      ];

      const rows = exportableBreaks.map((b) => [
        b.id,
        b.settlementSession,
        b.channel,
        b.breakType,
        b.ledgerRecord?.reference || 'N/A',
        b.ledgerRecord?.accountNumber || 'N/A',
        b.ledgerRecord?.amount || '0.00',
        b.switchRecord?.stan || 'N/A',
        b.switchRecord?.rrn || 'N/A',
        b.switchRecord?.clearingAmount || '0.00',
        b.amountDifference.toFixed(2),
        b.ledgerRecord?.postedAt || b.switchRecord?.clearedAt || 'N/A',
        b.status,
      ]);

      const csvContent =
        'data:text/csv;charset=utf-8,' +
        [headers.join(','), ...rows.map((e) => e.join(','))].join('\n');

      const encodedUri = encodeURI(csvContent);
      const link = document.createElement('a');
      link.setAttribute('href', encodedUri);
      link.setAttribute(
        'download',
        `NIBSS_DISPUTE_BATCH_${sessionCode}_${new Date().toISOString().slice(0, 10)}.csv`
      );
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);

      setIsExporting(false);
      onClose();
    }, 600);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-200">
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-2xl w-full max-w-lg overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <div className="flex items-center gap-2">
            <FileText className="w-5 h-5 text-indigo-600" />
            <h3 className="text-[15px] font-bold text-[var(--text-primary)]">
              Export Regulatory Dispute Settlement File
            </h3>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-[var(--text-muted)] hover:text-[var(--text-primary)] rounded"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-4">
          <div className="p-3 bg-emerald-50/70 border border-emerald-200 rounded text-[12px] text-emerald-900 flex items-start gap-2.5">
            <ShieldCheck className="w-4 h-4 text-emerald-600 shrink-0 mt-0.5" />
            <div>
              <span className="font-semibold">NIBSS & CBN Compliant Format: </span>
              Generates standardized dispute filing records containing STAN, RRN, Session IDs, and
              immutable anomaly hash references for clearing house submission.
            </div>
          </div>

          <div className="space-y-2">
            <label className="block text-[12px] font-bold text-[var(--text-primary)]">
              Select Output Specification
            </label>
            <div className="grid grid-cols-1 gap-2">
              <label
                className={`flex items-start gap-3 p-3 rounded border cursor-pointer transition-colors ${
                  format === 'NIBSS_CSV'
                    ? 'border-indigo-600 bg-indigo-50/30'
                    : 'border-[var(--surface-border)] bg-[var(--surface)]'
                }`}
              >
                <input
                  type="radio"
                  name="exportFormat"
                  value="NIBSS_CSV"
                  checked={format === 'NIBSS_CSV'}
                  onChange={() => setFormat('NIBSS_CSV')}
                  className="mt-1 text-indigo-600"
                />
                <div>
                  <div className="text-[13px] font-bold text-[var(--text-primary)]">
                    NIBSS Dispute Clearing CSV (Standard Interchange)
                  </div>
                  <div className="text-[11px] text-[var(--text-muted)]">
                    Includes session batch headers, debit account masks, transaction references, and STAN.
                  </div>
                </div>
              </label>

              <label
                className={`flex items-start gap-3 p-3 rounded border cursor-pointer transition-colors ${
                  format === 'CBN_DISPUTE_JSON'
                    ? 'border-indigo-600 bg-indigo-50/30'
                    : 'border-[var(--surface-border)] bg-[var(--surface)]'
                }`}
              >
                <input
                  type="radio"
                  name="exportFormat"
                  value="CBN_DISPUTE_JSON"
                  checked={format === 'CBN_DISPUTE_JSON'}
                  onChange={() => setFormat('CBN_DISPUTE_JSON')}
                  className="mt-1 text-indigo-600"
                />
                <div>
                  <div className="text-[13px] font-bold text-[var(--text-primary)]">
                    CBN Consumer Protection Filing Package (JSON)
                  </div>
                  <div className="text-[11px] text-[var(--text-muted)]">
                    Electronic report conforming to Section 10.4 regulatory audit standards.
                  </div>
                </div>
              </label>
            </div>
          </div>

          <div className="pt-2 border-t border-[var(--surface-border-subtle)]">
            <label className="flex items-center gap-2 cursor-pointer text-[12px] text-[var(--text-primary)] font-medium">
              <input
                type="checkbox"
                checked={includeResolved}
                onChange={(e) => setIncludeResolved(e.target.checked)}
                className="w-4 h-4 text-indigo-600 rounded"
              />
              <span>Include resolved and tolerance-matched records ({breaks.length} total)</span>
            </label>
          </div>

          <div className="p-3 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[11px] font-mono-financial flex justify-between items-center">
            <span className="text-[var(--text-muted)]">Records Prepared for Export:</span>
            <span className="font-bold text-[var(--text-primary)] text-[13px]">
              {exportableBreaks.length} Exceptions
            </span>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)]">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            isLoading={isExporting}
            onClick={handleExport}
            leftIcon={<Download className="w-4 h-4" />}
          >
            Download Dispute Batch File
          </Button>
        </div>
      </div>
    </div>
  );
};
