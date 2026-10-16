import React, { useState } from 'react';
import {
  FileText,
  Download,
  ShieldCheck,
  CheckCircle2,
  Building,
  Calendar,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { CbnRegulatoryDisputeReturn } from './reportsTypes';

interface RegulatoryGeneratorTabProps {
  report: CbnRegulatoryDisputeReturn;
}

export const RegulatoryGeneratorTab: React.FC<RegulatoryGeneratorTabProps> = ({ report }) => {
  const [isExporting, setIsExporting] = useState(false);

  const handleExportReturn = () => {
    setIsExporting(true);
    setTimeout(() => {
      const headers = [
        'CHANNEL',
        'DISPUTES_RECEIVED',
        'RESOLVED_WITHIN_SLA',
        'RESOLVED_OUTSIDE_SLA',
        'OUTSTANDING_DISPUTES',
        'TOTAL_DISPUTED_NGN',
        'TOTAL_REVERSED_NGN',
        'COMPLIANCE_RATE_PCT',
      ];

      const rows = report.channels.map((c) => [
        c.channel,
        c.disputesReceived,
        c.resolvedWithinSla,
        c.resolvedOutsideSla,
        c.outstandingDisputes,
        c.totalAmountDisputedNgn,
        c.totalAmountReversedNgn,
        `${c.complianceRatePct}%`,
      ]);

      const csvContent =
        'data:text/csv;charset=utf-8,' +
        [
          `# CBN MONTHLY DISPUTED TRANSACTIONS RETURN - ${report.reportingPeriod}`,
          `# INSTITUTION: ${report.institutionName} (CODE: ${report.institutionCode})`,
          `# AUDIT STATUS: ${report.auditCertificationStatus}`,
          headers.join(','),
          ...rows.map((r) => r.join(',')),
        ].join('\n');

      const encodedUri = encodeURI(csvContent);
      const link = document.createElement('a');
      link.setAttribute('href', encodedUri);
      link.setAttribute(
        'download',
        `CBN_DISPUTE_RETURN_${report.institutionCode}_${new Date().toISOString().slice(0, 10)}.csv`
      );
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      setIsExporting(false);
    }, 600);
  };

  return (
    <div className="space-y-6">
      {/* Report Header Card */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs">
        <div>
          <div className="flex items-center gap-2">
            <FileText className="w-5 h-5 text-indigo-600" />
            <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
              CBN Monthly Disputed Transactions Compliance Return
            </h3>
            <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-blue-100 text-blue-800 font-mono-financial">
              CBN CPD Form 04
            </span>
          </div>
          <div className="mt-1 flex items-center gap-3 text-[12px] text-[var(--text-muted)] font-mono-financial">
            <span className="flex items-center gap-1">
              <Building className="w-3.5 h-3.5" />
              <span>{report.institutionName}</span>
            </span>
            <span>•</span>
            <span className="flex items-center gap-1">
              <Calendar className="w-3.5 h-3.5" />
              <span>Period: {report.reportingPeriod}</span>
            </span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <div className="flex items-center gap-1.5 px-3 py-1.5 rounded bg-emerald-50 border border-emerald-200 text-emerald-900 text-[11px] font-mono-financial font-bold">
            <ShieldCheck className="w-4 h-4 text-emerald-600" />
            <span>Audit Status: CERTIFIED</span>
          </div>

          <Button
            variant="primary"
            size="sm"
            isLoading={isExporting}
            onClick={handleExportReturn}
            leftIcon={<Download className="w-4 h-4" />}
          >
            Export Official Return (CSV)
          </Button>
        </div>
      </div>

      {/* Official Return Table */}
      <div className="bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs overflow-hidden">
        <div className="px-5 py-3 border-b border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] flex items-center justify-between">
          <span className="font-bold text-[13px] text-[var(--text-primary)]">
            Schedule of Disputed Electronic Payment Transactions by Channel
          </span>
          <span className="text-[11px] font-mono-financial text-[var(--text-muted)]">
            In accordance with CBN Circular BPS/DIR/GEN/CIR/05/011
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-[12px] font-mono-financial">
            <thead>
              <tr className="border-b border-[var(--surface-border)] bg-[var(--canvas-subdued)] text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-sans">
                <th className="py-2.5 px-4">Payment Channel</th>
                <th className="py-2.5 px-4 text-right">Disputes Received</th>
                <th className="py-2.5 px-4 text-right">Within SLA</th>
                <th className="py-2.5 px-4 text-right">Outside SLA</th>
                <th className="py-2.5 px-4 text-right">Outstanding</th>
                <th className="py-2.5 px-4 text-right">Amount Disputed (₦)</th>
                <th className="py-2.5 px-4 text-right">Amount Reversed (₦)</th>
                <th className="py-2.5 px-4 text-right">Compliance %</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--surface-border-subtle)]">
              {report.channels.map((c) => (
                <tr key={c.channel} className="hover:bg-[var(--canvas-subdued)] transition-colors">
                  <td className="py-3 px-4 font-bold text-[var(--text-primary)] font-sans">
                    {c.channel} ({c.channel === 'NIP' ? 'Instant Transfer' : c.channel === 'POS' ? 'Terminal Acquirer' : c.channel === 'ATM' ? 'Cash Dispenser' : 'Cross-Border'})
                  </td>
                  <td className="py-3 px-4 text-right">{c.disputesReceived.toLocaleString()}</td>
                  <td className="py-3 px-4 text-right text-emerald-700 font-bold">
                    {c.resolvedWithinSla.toLocaleString()}
                  </td>
                  <td className="py-3 px-4 text-right text-amber-700 font-bold">
                    {c.resolvedOutsideSla}
                  </td>
                  <td className="py-3 px-4 text-right text-rose-700 font-bold">
                    {c.outstandingDisputes}
                  </td>
                  <td className="py-3 px-4 text-right">{c.totalAmountDisputedNgn}</td>
                  <td className="py-3 px-4 text-right text-emerald-700 font-bold">
                    {c.totalAmountReversedNgn}
                  </td>
                  <td className="py-3 px-4 text-right">
                    <span className="px-2 py-0.5 rounded bg-emerald-100 text-emerald-800 font-bold text-[11px]">
                      {c.complianceRatePct}%
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Statutory Attestation Note */}
      <div className="p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-2 text-[12px]">
        <div className="font-bold text-[var(--text-primary)] flex items-center gap-2">
          <CheckCircle2 className="w-4 h-4 text-emerald-600" />
          <span>Statutory Executive Attestation</span>
        </div>
        <p className="text-[var(--text-secondary)] leading-relaxed">
          I hereby certify that the above return represents a true, accurate, and complete record of
          all customer and inter-bank transaction exceptions received, processed, and resolved by Wema
          Bank Plc during the reporting period, in conformity with the Central Bank of Nigeria Consumer
          Protection Regulations and NIBSS Operating Standards.
        </p>
        <div className="pt-2 flex items-center justify-between text-[11px] font-mono-financial text-[var(--text-muted)] border-t border-[var(--surface-border-subtle)]">
          <span>Signatory: Head of Operations & Chief Compliance Officer</span>
          <span>Digital Anomaly Hash: sha256:7f991b4408a2...</span>
        </div>
      </div>
    </div>
  );
};
