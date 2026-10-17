import React, { useState } from 'react';
import {
  Server,
  RotateCcw,
  Send,
  Zap,
  Building2,
  ShieldCheck,
  FileCode,
} from 'lucide-react';
import clsx from 'clsx';
import { Button } from '../../components/Button';

interface SwitchScenario {
  id: string;
  name: string;
  switchName: string;
  protocol: 'ISO 8583' | 'REST / ISO 20022' | 'JSON-RPC';
  mti: string;
  stan: string;
  rrn: string;
  amount: string;
  currency: string;
  terminalId: string;
  processingCode: string;
  responseCode: string;
  drGlAccount: string;
  drGlName: string;
  crAccount: string;
  crAccountName: string;
  cbsBatchId: string;
  rawHexPayload: string;
}

const PRESET_SCENARIOS: SwitchScenario[] = [
  {
    id: 'scen-nip-timeout',
    name: 'NIBSS NIP Settlement Timeout (Direct Reversal)',
    switchName: 'NIBSS Instant Payment (NIP) Engine',
    protocol: 'REST / ISO 20022',
    mti: '0420 (Reversal Advice)',
    stan: '772910',
    rrn: '000013230914',
    amount: '45,000.00',
    currency: 'NGN',
    terminalId: 'WEB_NIP_PORTAL_01',
    processingCode: '200000 (Account Re-Credit)',
    responseCode: '00 (Approved / Completed)',
    drGlAccount: 'GL-100299-NIP-SUSPENSE',
    drGlName: 'NIP Outbound Clearing Dispute Suspense GL',
    crAccount: '0128394910',
    crAccountName: 'Olusola Adeyemi (Savings)',
    cbsBatchId: 'CBS-REV-2026-NIP-9921',
    rawHexPayload: '04207038000008000000020000000000450000101704502277291000001323091400',
  },
  {
    id: 'scen-atm-dispense',
    name: 'ATM Cash Retract / Sensor Trap (Postilion Switch)',
    switchName: 'Postilion Switch Cluster (ATM Network)',
    protocol: 'ISO 8583',
    mti: '0420 (Reversal Advice)',
    stan: '482019',
    rrn: '992019204812',
    amount: '20,000.00',
    currency: 'NGN',
    terminalId: 'ATM_0921_MARINA_LAGOS',
    processingCode: '010000 (Cash Dispense Reversal)',
    responseCode: '00 (Approved / Completed)',
    drGlAccount: 'GL-100840-ATM-VAULT-CLEARING',
    drGlName: 'ATM Cash Vault Discrepancy Suspense',
    crAccount: '0281928410',
    crAccountName: 'Chioma Okonkwo (Current)',
    cbsBatchId: 'CBS-REV-2026-ATM-8812',
    rawHexPayload: '04203238000000000000010000000000200000101704502248201999201920481200',
  },
  {
    id: 'scen-pos-duplicate',
    name: 'POS Duplicate Charge Reversal (Interswitch)',
    switchName: 'Interswitch Transaction Processing Switching',
    protocol: 'ISO 8583',
    mti: '0420 (Reversal Advice)',
    stan: '918234',
    rrn: '281920381029',
    amount: '12,500.00',
    currency: 'NGN',
    terminalId: 'POS_2048_IKEJA_MALL',
    processingCode: '200000 (Purchase Void / Inversion)',
    responseCode: '00 (Approved / Completed)',
    drGlAccount: 'GL-100412-POS-MERCHANT-ACQUIRING',
    drGlName: 'Interswitch Acquiring Settlement Account',
    crAccount: '0391827461',
    crAccountName: 'Ibrahim Danladi (Savings)',
    cbsBatchId: 'CBS-REV-2026-POS-7741',
    rawHexPayload: '04207038000008000000020000000000125000101704502291823428192038102900',
  },
  {
    id: 'scen-switch-fail',
    name: 'Switch Timeout / Malfunction (Response Code 91)',
    switchName: 'Mastercard International Debit Gateway',
    protocol: 'ISO 8583',
    mti: '0420 (Reversal Advice)',
    stan: '339102',
    rrn: '558192039182',
    amount: '85,000.00',
    currency: 'NGN',
    terminalId: 'MC_GW_ONLINE_ECOM',
    processingCode: '200000 (Cardholder Credit Reversal)',
    responseCode: '91 (System Malfunction / Timeout)',
    drGlAccount: 'GL-100990-CARD-DISPUTE-HOLD',
    drGlName: 'Mastercard Dispute Buffer Account',
    crAccount: '0182736450',
    crAccountName: 'Funke Akindele (Current)',
    cbsBatchId: 'CBS-REV-2026-CARD-PENDING',
    rawHexPayload: '04207038000008000000020000000000850000101704502233910255819203918291',
  },
];

export const SwitchPacketAndCBSInspector: React.FC = () => {
  const [selectedScenario, setSelectedScenario] = useState<SwitchScenario>(PRESET_SCENARIOS[0]);
  const [isTransmitting, setIsTransmitting] = useState(false);
  const [transmissionStep, setTransmissionStep] = useState<number>(0);
  const [dispatchStatus, setDispatchStatus] = useState<'IDLE' | 'PROGRESS' | 'SUCCESS' | 'FAILED'>('IDLE');

  const handleTransmit = () => {
    setIsTransmitting(true);
    setDispatchStatus('PROGRESS');
    setTransmissionStep(1);

    setTimeout(() => {
      setTransmissionStep(2);
      setTimeout(() => {
        setTransmissionStep(3);
        setTimeout(() => {
          setTransmissionStep(4);
          setTimeout(() => {
            setIsTransmitting(false);
            if (selectedScenario.responseCode.startsWith('91')) {
              setDispatchStatus('FAILED');
            } else {
              setDispatchStatus('SUCCESS');
            }
          }, 600);
        }, 600);
      }, 600);
    }, 600);
  };

  const handleReset = () => {
    setIsTransmitting(false);
    setTransmissionStep(0);
    setDispatchStatus('IDLE');
  };

  return (
    <div className="space-y-6">
      {/* Top Banner & Scenario Select */}
      <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-md bg-indigo-100 text-indigo-900 dark:bg-indigo-950 dark:text-indigo-300">
              <Server className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
                  Switch Gateway & Core Banking (CBS) Simulator
                </h3>
                <span className="text-[10px] font-mono-financial font-extrabold uppercase px-2 py-0.5 rounded bg-indigo-100 text-indigo-900 dark:bg-indigo-950 dark:text-indigo-300">
                  Part 3: Institutional Connectors
                </span>
              </div>
              <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
                Inspect raw ISO 8583 packet telemetry, test Switch handshake responses, and verify double-entry CBS journal postings.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={handleReset}
              disabled={isTransmitting}
              leftIcon={<RotateCcw className="w-3.5 h-3.5" />}
            >
              Reset
            </Button>
            <Button
              variant="primary"
              size="sm"
              isLoading={isTransmitting}
              onClick={handleTransmit}
              leftIcon={<Send className="w-3.5 h-3.5" />}
            >
              Transmit Reversal Packet
            </Button>
          </div>
        </div>

        {/* Preset Selector */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-2.5 pt-2">
          {PRESET_SCENARIOS.map((scen) => (
            <button
              key={scen.id}
              onClick={() => {
                setSelectedScenario(scen);
                handleReset();
              }}
              className={clsx(
                'text-left p-3 rounded border transition-all text-[12px]',
                selectedScenario.id === scen.id
                  ? 'border-indigo-500 bg-indigo-50/40 dark:bg-indigo-950/30 ring-1 ring-indigo-500'
                  : 'border-[var(--surface-border-subtle)] bg-[var(--canvas-subdued)] hover:border-[var(--surface-border)]'
              )}
            >
              <div className="font-bold text-[var(--text-primary)] leading-tight">{scen.name}</div>
              <div className="text-[11px] text-[var(--text-muted)] font-mono-financial mt-1">
                {scen.switchName.split(' ')[0]} • {scen.currency} {scen.amount}
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* Live Transmit Progress Pipeline */}
      {dispatchStatus !== 'IDLE' && (
        <div
          className={clsx(
            'p-4 rounded-lg border text-[12px] transition-all',
            dispatchStatus === 'FAILED'
              ? 'bg-rose-50/60 border-rose-300 dark:bg-rose-950/30'
              : dispatchStatus === 'SUCCESS'
              ? 'bg-emerald-50/60 border-emerald-300 dark:bg-emerald-950/30'
              : 'bg-indigo-50/60 border-indigo-300 dark:bg-indigo-950/30'
          )}
        >
          <div className="flex items-center justify-between mb-3">
            <span className="font-bold uppercase tracking-wider text-[11px] flex items-center gap-1.5">
              <Zap className="w-4 h-4 text-indigo-600 animate-pulse" /> Reversal Protocol Execution Sequence
            </span>
            <span className="font-mono-financial font-extrabold text-[11px]">
              {dispatchStatus === 'SUCCESS'
                ? 'COMPLETED: 200 OK • GL JOURNAL COMMITTED'
                : dispatchStatus === 'FAILED'
                ? 'FAILED: RC 91 SYSTEM MALFUNCTION'
                : `EXECUTING STEP ${transmissionStep} OF 4...`}
            </span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-4 gap-2 text-[11px] font-mono-financial">
            <div
              className={clsx(
                'p-2 rounded border',
                transmissionStep >= 1
                  ? 'bg-white dark:bg-zinc-900 border-indigo-500 text-indigo-700 dark:text-indigo-300 font-bold'
                  : 'bg-transparent border-zinc-200 dark:border-zinc-800 text-zinc-400'
              )}
            >
              1. ISO 8583 Assembly ✓
            </div>
            <div
              className={clsx(
                'p-2 rounded border',
                transmissionStep >= 2
                  ? 'bg-white dark:bg-zinc-900 border-indigo-500 text-indigo-700 dark:text-indigo-300 font-bold'
                  : 'bg-transparent border-zinc-200 dark:border-zinc-800 text-zinc-400'
              )}
            >
              2. Switch Handshake ✓
            </div>
            <div
              className={clsx(
                'p-2 rounded border',
                transmissionStep >= 3
                  ? 'bg-white dark:bg-zinc-900 border-indigo-500 text-indigo-700 dark:text-indigo-300 font-bold'
                  : 'bg-transparent border-zinc-200 dark:border-zinc-800 text-zinc-400'
              )}
            >
              3. Response RC {selectedScenario.responseCode.slice(0, 2)} ✓
            </div>
            <div
              className={clsx(
                'p-2 rounded border',
                transmissionStep >= 4
                  ? dispatchStatus === 'FAILED'
                    ? 'bg-rose-100 dark:bg-rose-900 border-rose-500 text-rose-800 dark:text-rose-200 font-bold'
                    : 'bg-emerald-100 dark:bg-emerald-900 border-emerald-500 text-emerald-800 dark:text-emerald-200 font-bold'
                  : 'bg-transparent border-zinc-200 dark:border-zinc-800 text-zinc-400'
              )}
            >
              4. CBS GL Journal {dispatchStatus === 'FAILED' ? 'Rolled Back' : 'Posted ✓'}
            </div>
          </div>
        </div>
      )}

      {/* Main Inspection Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: ISO 8583 Bitfield Decoder */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
          <div className="flex items-center justify-between pb-3 border-b border-[var(--surface-border-subtle)]">
            <div className="flex items-center gap-2">
              <FileCode className="w-4 h-4 text-indigo-600" />
              <h4 className="text-[14px] font-bold text-[var(--text-primary)]">
                ISO 8583 / Switch Message Bitfield Inspector
              </h4>
            </div>
            <span className="text-[10px] font-mono-financial font-extrabold uppercase px-1.5 py-0.5 rounded bg-zinc-100 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300">
              {selectedScenario.protocol}
            </span>
          </div>

          {/* Hex Raw Stream */}
          <div>
            <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)] block mb-1">
              Raw Network Wire Frame (Hex Dump)
            </span>
            <div className="p-2.5 rounded bg-zinc-950 text-emerald-400 font-mono text-[11px] break-all border border-zinc-800 shadow-inner">
              {selectedScenario.rawHexPayload}
            </div>
          </div>

          {/* Decoded Bitfields Table */}
          <div className="space-y-1.5 text-[12px] font-mono-financial">
            <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
              Decoded Primary & Secondary Bitfields
            </span>
            <div className="border border-[var(--surface-border)] rounded overflow-hidden divide-y divide-[var(--surface-border-subtle)]">
              <div className="p-2.5 flex items-center justify-between bg-[var(--canvas-subdued)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 0 (MTI)</span>
                <span className="font-extrabold text-indigo-600 dark:text-indigo-400">{selectedScenario.mti}</span>
              </div>
              <div className="p-2.5 flex items-center justify-between bg-[var(--surface)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 3 (Processing Code)</span>
                <span className="text-[var(--text-primary)]">{selectedScenario.processingCode}</span>
              </div>
              <div className="p-2.5 flex items-center justify-between bg-[var(--canvas-subdued)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 4 (Transaction Amount)</span>
                <span className="font-extrabold text-[var(--text-primary)]">
                  {selectedScenario.currency} {selectedScenario.amount}
                </span>
              </div>
              <div className="p-2.5 flex items-center justify-between bg-[var(--surface)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 11 (STAN Audit Number)</span>
                <span className="text-[var(--text-primary)]">{selectedScenario.stan}</span>
              </div>
              <div className="p-2.5 flex items-center justify-between bg-[var(--canvas-subdued)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 37 (Retrieval Ref / RRN)</span>
                <span className="text-[var(--text-primary)]">{selectedScenario.rrn}</span>
              </div>
              <div className="p-2.5 flex items-center justify-between bg-[var(--surface)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 39 (Action / Response Code)</span>
                <span
                  className={clsx(
                    'font-bold px-1.5 py-0.5 rounded text-[11px]',
                    selectedScenario.responseCode.startsWith('00')
                      ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                      : 'bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-300'
                  )}
                >
                  {selectedScenario.responseCode}
                </span>
              </div>
              <div className="p-2.5 flex items-center justify-between bg-[var(--canvas-subdued)]">
                <span className="font-bold text-[var(--text-secondary)]">Field 41 (Terminal ID)</span>
                <span className="text-[var(--text-primary)]">{selectedScenario.terminalId}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Right Column: Core Banking System (CBS) GL Journal Simulator */}
        <div className="p-5 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs space-y-4">
          <div className="flex items-center justify-between pb-3 border-b border-[var(--surface-border-subtle)]">
            <div className="flex items-center gap-2">
              <Building2 className="w-4 h-4 text-emerald-600" />
              <h4 className="text-[14px] font-bold text-[var(--text-primary)]">
                Core Banking System (CBS) Double-Entry GL Journal
              </h4>
            </div>
            <span className="text-[10px] font-mono-financial font-extrabold uppercase px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-950 text-emerald-800 dark:text-emerald-300">
              Finacle / Temenos GL
            </span>
          </div>

          {/* Batch Meta */}
          <div className="grid grid-cols-2 gap-2 text-[11px] font-mono-financial p-3 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)]">
            <div>
              <span className="text-[var(--text-muted)] block">Batch Journal Reference:</span>
              <span className="font-bold text-[var(--text-primary)]">{selectedScenario.cbsBatchId}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)] block">Value Date:</span>
              <span className="font-bold text-[var(--text-primary)]">2026-10-17 (Same-Day Immediate)</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)] block">Posting Engine:</span>
              <span className="font-bold text-[var(--text-primary)]">Finacle 11 Core API / ISO 20022 camt.053</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)] block">Reversal Lock Status:</span>
              <span className="font-bold text-emerald-700 dark:text-emerald-300">Acquired (Idempotent 120s)</span>
            </div>
          </div>

          {/* Double-Entry Legs */}
          <div className="space-y-2">
            <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)] block">
              Balanced Double-Entry Posting Legs
            </span>

            {/* Debit Leg */}
            <div className="p-3 rounded border border-rose-200 dark:border-rose-900/50 bg-rose-50/30 dark:bg-rose-950/20 text-[12px] font-mono-financial flex items-start justify-between">
              <div>
                <div className="flex items-center gap-1.5 font-bold text-rose-900 dark:text-rose-300">
                  <span className="px-1.5 py-0.2 bg-rose-200 dark:bg-rose-900 rounded text-[10px]">DEBIT (Dr)</span>
                  <span>{selectedScenario.drGlAccount}</span>
                </div>
                <div className="text-[11px] text-[var(--text-muted)] mt-0.5">{selectedScenario.drGlName}</div>
              </div>
              <div className="text-right">
                <span className="font-extrabold text-rose-800 dark:text-rose-300 text-[13px]">
                  {selectedScenario.currency} {selectedScenario.amount}
                </span>
                <span className="block text-[10px] text-[var(--text-muted)]">Reversal Outflow</span>
              </div>
            </div>

            {/* Credit Leg */}
            <div className="p-3 rounded border border-emerald-200 dark:border-emerald-900/50 bg-emerald-50/30 dark:bg-emerald-950/20 text-[12px] font-mono-financial flex items-start justify-between">
              <div>
                <div className="flex items-center gap-1.5 font-bold text-emerald-900 dark:text-emerald-300">
                  <span className="px-1.5 py-0.2 bg-emerald-200 dark:bg-emerald-900 rounded text-[10px]">CREDIT (Cr)</span>
                  <span>Account {selectedScenario.crAccount}</span>
                </div>
                <div className="text-[11px] text-[var(--text-muted)] mt-0.5">{selectedScenario.crAccountName}</div>
              </div>
              <div className="text-right">
                <span className="font-extrabold text-emerald-800 dark:text-emerald-300 text-[13px]">
                  {selectedScenario.currency} {selectedScenario.amount}
                </span>
                <span className="block text-[10px] text-[var(--text-muted)]">Customer Restored</span>
              </div>
            </div>
          </div>

          {/* Audit & Compliance Seal */}
          <div className="p-3 bg-zinc-50 dark:bg-zinc-900 border border-[var(--surface-border-subtle)] rounded flex items-center gap-2.5 text-[11px] text-[var(--text-muted)]">
            <ShieldCheck className="w-4 h-4 text-emerald-600 shrink-0" />
            <span>
              Compliant with Central Bank BPS/CIR/GEN/02/007 dispute settlement circular and ISO 20022 pacs.004 reversal messaging standards.
            </span>
          </div>
        </div>
      </div>
    </div>
  );
};
