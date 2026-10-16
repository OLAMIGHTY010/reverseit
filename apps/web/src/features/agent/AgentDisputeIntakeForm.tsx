import React, { useState } from 'react';
import {
  FilePlus2,
  CheckCircle2,
  Send,
  Upload,
  User,
  Phone,
  Building,
  CreditCard,
  DollarSign,
  Calendar,
  Hash,
} from 'lucide-react';
import { Button } from '../../components/Button';
import { useApp } from '../../context/AppContext';
import { apiClient } from '../../services/apiClient';
import type { Channel, CustomerIntakeQuery, CustomerDisputeIntakePayload } from '../../types/domain';

export interface AgentDisputeIntakeFormProps {
  initialQuery?: CustomerIntakeQuery;
  onCaseCreated: (caseNumber: string, reference: string) => void;
}

export const AgentDisputeIntakeForm: React.FC<AgentDisputeIntakeFormProps> = ({
  initialQuery,
  onCaseCreated,
}) => {
  const { role, addToast } = useApp();

  const [customerName, setCustomerName] = useState('Chinedu Okafor');
  const [customerPhone, setCustomerPhone] = useState(
    initialQuery?.customerToken?.includes('+') ? initialQuery.customerToken : '+234 803 555 8899'
  );
  const [customerAccount, setCustomerAccount] = useState(
    initialQuery?.customerToken && !initialQuery.customerToken.includes('+')
      ? initialQuery.customerToken
      : '0188994422'
  );
  const [channel, setChannel] = useState<Channel>(initialQuery?.channel || 'POS');
  const [amount, setAmount] = useState(initialQuery?.amount || '75000.00');
  const [currency] = useState('NGN');
  const [transactionDate, setTransactionDate] = useState(initialQuery?.date || '2026-10-15');
  const [beneficiaryBank, setBeneficiaryBank] = useState('Zenith Bank (057)');
  const [beneficiaryAccount, setBeneficiaryAccount] = useState('2019948211');
  const [complaintText, setComplaintText] = useState(
    'Customer was debited at merchant POS terminal but merchant receipt showed Decline Code 91. Funds not returned after 2 hours.'
  );
  const [hasReceiptUploaded, setHasReceiptUploaded] = useState(true);

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [createdCaseInfo, setCreatedCaseInfo] = useState<{ caseNumber: string; reference: string } | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!customerName.trim() || !customerAccount.trim() || !amount.trim()) {
      addToast({
        type: 'error',
        title: 'Validation Error',
        message: 'Please fill in all mandatory customer and transaction fields.',
      });
      return;
    }

    setIsSubmitting(true);
    try {
      const payload: CustomerDisputeIntakePayload = {
        customerName: customerName.trim(),
        customerPhone: customerPhone.trim(),
        customerAccount: customerAccount.trim(),
        channel,
        amount: amount.trim(),
        currency,
        transactionDate,
        beneficiaryBank,
        beneficiaryAccount: beneficiaryAccount.trim(),
        complaintText: complaintText.trim(),
        agentId: 'Agent Chioma Nwosu',
        agentRole: role,
      };

      const { exception, caseNumber } = await apiClient.createCustomerReportedIntake(payload);

      setCreatedCaseInfo({ caseNumber, reference: exception.reference });
      onCaseCreated(caseNumber, exception.reference);

      addToast({
        type: 'success',
        title: 'Dispute Docket Opened',
        message: `Case ${caseNumber} created with reference ${exception.reference}. Escalated to Recon Operations.`,
      });
    } catch {
      addToast({
        type: 'error',
        title: 'Submission Error',
        message: 'Could not create dispute case. Please retry.',
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="border border-[var(--surface-border)] rounded-md bg-[var(--surface)] shadow-xs divide-y divide-[var(--surface-border)]">
      {/* Header Banner */}
      <div className="p-4 bg-[var(--canvas-subdued)] flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="p-1 rounded bg-sky-100 text-sky-800">
            <FilePlus2 className="w-5 h-5 text-sky-600" />
          </span>
          <div>
            <h3 className="text-[14px] font-bold text-[var(--text-primary)]">
              No Record Match: Open Customer Dispute Intake Form
            </h3>
            <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
              Structured intake captures dispute payload to route directly into Tier-1 Recon queue under code CUS001.
            </p>
          </div>
        </div>

        <div className="text-[11px] font-mono-financial px-2 py-0.5 rounded bg-[var(--surface)] border border-[var(--surface-border)] text-sky-800 font-semibold">
          Classification: CUS001
        </div>
      </div>

      {createdCaseInfo ? (
        <div className="p-6 text-center space-y-3 bg-emerald-500/10 text-emerald-950">
          <CheckCircle2 className="w-10 h-10 text-emerald-600 mx-auto" />
          <h4 className="text-[16px] font-bold">Dispute Docket Successfully Registered</h4>
          <p className="text-[13px] max-w-md mx-auto">
            Case number <strong className="font-mono-financial text-emerald-900">{createdCaseInfo.caseNumber}</strong> has been assigned under reference <strong className="font-mono-financial">{createdCaseInfo.reference}</strong>.
          </p>
          <div className="p-3 bg-[var(--surface)] border border-emerald-300 rounded font-mono-financial text-[12px] max-w-sm mx-auto">
            Provide this tracking number to caller: <strong>{createdCaseInfo.caseNumber}</strong>
          </div>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => setCreatedCaseInfo(null)}
          >
            File Another Customer Intake
          </Button>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="p-4 space-y-4 text-[13px]">
          {/* Row 1: Customer Personal Info */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label htmlFor="intake-name-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <User className="w-3 h-3 inline mr-1 text-sky-600" /> Customer Full Name *
              </label>
              <input
                id="intake-name-input"
                type="text"
                required
                value={customerName}
                onChange={(e) => setCustomerName(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            <div>
              <label htmlFor="intake-phone-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <Phone className="w-3 h-3 inline mr-1 text-sky-600" /> Phone Number *
              </label>
              <input
                id="intake-phone-input"
                type="text"
                required
                value={customerPhone}
                onChange={(e) => setCustomerPhone(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            <div>
              <label htmlFor="intake-account-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <CreditCard className="w-3 h-3 inline mr-1 text-sky-600" /> Originating Account Number *
              </label>
              <input
                id="intake-account-input"
                type="text"
                required
                value={customerAccount}
                onChange={(e) => setCustomerAccount(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          {/* Row 2: Transaction Specifics */}
          <div className="grid grid-cols-1 sm:grid-cols-4 gap-3">
            <div>
              <label htmlFor="intake-channel-select" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                Channel / Payment Rail
              </label>
              <select
                id="intake-channel-select"
                value={channel}
                onChange={(e) => setChannel(e.target.value as Channel)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              >
                <option value="POS">POS Terminal</option>
                <option value="NIP">NIP Instant Payment</option>
                <option value="CARD">Card Contactless / ATM</option>
                <option value="WEBHOOK">Online Webhook</option>
                <option value="DIRECT_DEBIT">Direct Debit</option>
              </select>
            </div>

            <div>
              <label htmlFor="intake-amount-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <DollarSign className="w-3 h-3 inline mr-1 text-sky-600" /> Amount (NGN) *
              </label>
              <input
                id="intake-amount-input"
                type="text"
                required
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            <div>
              <label htmlFor="intake-date-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <Calendar className="w-3 h-3 inline mr-1 text-sky-600" /> Date of Transaction
              </label>
              <input
                id="intake-date-input"
                type="date"
                value={transactionDate}
                onChange={(e) => setTransactionDate(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            <div>
              <label htmlFor="intake-bank-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <Building className="w-3 h-3 inline mr-1 text-sky-600" /> Beneficiary Institution
              </label>
              <input
                id="intake-bank-input"
                type="text"
                value={beneficiaryBank}
                onChange={(e) => setBeneficiaryBank(e.target.value)}
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            <div>
              <label htmlFor="intake-beneficiary-account-input" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
                <Hash className="w-3 h-3 inline mr-1 text-sky-600" /> Beneficiary Account
              </label>
              <input
                id="intake-beneficiary-account-input"
                type="text"
                value={beneficiaryAccount}
                onChange={(e) => setBeneficiaryAccount(e.target.value)}
                placeholder="e.g. 2019948211"
                className="w-full px-3 py-1.5 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] font-mono-financial focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          {/* Row 3: Customer Complaint Narrative */}
          <div>
            <label htmlFor="intake-complaint-textarea" className="block text-[11px] font-semibold text-[var(--text-secondary)] mb-1">
              Customer Statement / Complaint Details *
            </label>
            <textarea
              id="intake-complaint-textarea"
              rows={3}
              required
              value={complaintText}
              onChange={(e) => setComplaintText(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border)] rounded text-[13px] focus:outline-none focus:ring-1 focus:ring-sky-500 placeholder:text-[var(--text-muted)]"
              placeholder="Describe exact customer statement, merchant name, terminal error code, or transaction context..."
            />
          </div>

          {/* Upload Simulation Checkbox */}
          <div className="flex items-center gap-2 text-[12px] text-[var(--text-secondary)]">
            <input
              id="receipt-upload-check"
              type="checkbox"
              checked={hasReceiptUploaded}
              onChange={(e) => setHasReceiptUploaded(e.target.checked)}
              className="rounded border-[var(--surface-border)] text-sky-600 focus:ring-sky-500 cursor-pointer"
            />
            <label htmlFor="receipt-upload-check" className="cursor-pointer flex items-center gap-1.5">
              <Upload className="w-3.5 h-3.5 text-sky-600" />
              <span>Customer attached debit SMS alert screenshot or POS decline slip (simulated)</span>
            </label>
          </div>

          <div className="flex items-center justify-between pt-2 border-t border-[var(--surface-border-subtle)]">
            <span className="text-[11px] text-[var(--text-muted)]">
              Creates audited case in <strong>Tier-1 Recon Dispute Queue</strong> with 4h internal acknowledgment SLA.
            </span>

            <Button
              type="submit"
              variant="primary"
              disabled={isSubmitting}
              leftIcon={<Send className="w-4 h-4" />}
            >
              {isSubmitting ? 'Registering Docket...' : 'Submit Customer Intake Ticket'}
            </Button>
          </div>
        </form>
      )}
    </div>
  );
};
