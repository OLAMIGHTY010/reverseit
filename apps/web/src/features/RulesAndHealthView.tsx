import React, { useState } from 'react';
import { Sliders, Activity, Server } from 'lucide-react';
import { useApp } from '../context/AppContext';
import {
  DEFAULT_SAFETY_GATE_CONFIG,
  INITIAL_ADAPTER_HEALTH,
} from './rules/rulesData';
import type {
  SafetyGateRulesConfig,
  AdapterHealthStatus,
  CircuitBreakerState,
} from './rules/rulesTypes';
import { RulesEditorTab } from './rules/RulesEditorTab';
import { IntegrationHealthTab } from './rules/IntegrationHealthTab';
import { SwitchPacketAndCBSInspector } from './rules/SwitchPacketAndCBSInspector';

export const RulesAndHealthView: React.FC = () => {
  const { addToast } = useApp();

  const [activeTab, setActiveTab] = useState<'RULES' | 'HEALTH' | 'SWITCH_CBS'>('RULES');
  const [config, setConfig] = useState<SafetyGateRulesConfig>(DEFAULT_SAFETY_GATE_CONFIG);
  const [adapters, setAdapters] = useState<AdapterHealthStatus[]>(INITIAL_ADAPTER_HEALTH);
  const [isInjecting, setIsInjecting] = useState(false);

  const handleSaveProposedConfig = (
    newCfg: SafetyGateRulesConfig,
    justification: string
  ) => {
    setConfig(newCfg);
    addToast({
      type: 'info',
      title: 'Rules Proposal Registered (Maker Submission)',
      message: `Change proposal queued for Checker approval: "${justification.slice(0, 50)}..."`,
    });
  };

  const handleResetDefaults = () => {
    setConfig(DEFAULT_SAFETY_GATE_CONFIG);
    addToast({
      type: 'info',
      title: 'Defaults Restored',
      message: 'Safety gate parameters reset to central bank baseline.',
    });
  };

  const handleToggleCircuitBreaker = (adapterId: string, newState: CircuitBreakerState) => {
    setAdapters((prev) =>
      prev.map((a) =>
        a.id === adapterId
          ? {
              ...a,
              circuitBreaker: newState,
              status: newState === 'OPEN' ? 'DOWN' : newState === 'HALF_OPEN' ? 'DEGRADED' : 'HEALTHY',
            }
          : a
      )
    );
    addToast({
      type: newState === 'OPEN' ? 'warning' : 'success',
      title: 'Circuit Breaker State Transitioned',
      message: `Adapter ${adapterId} transitioned to ${newState}.`,
    });
  };

  const handleInjectFault = (type: 'LATENCY_SPIKE' | 'TRIP_BREAKER' | 'RESET') => {
    if (type === 'RESET') {
      setAdapters(INITIAL_ADAPTER_HEALTH);
      addToast({
        type: 'info',
        title: 'Telemetry Reset',
        message: 'All integration endpoints restored to baseline health.',
      });
      return;
    }

    setIsInjecting(true);
    setTimeout(() => {
      setIsInjecting(false);
      if (type === 'LATENCY_SPIKE') {
        setAdapters((prev) =>
          prev.map((a) =>
            a.id === 'adapter-nibss-nip'
              ? {
                  ...a,
                  pingLatencyMs: 840,
                  status: 'DEGRADED',
                  circuitBreaker: 'HALF_OPEN',
                  queuedRequests: 142,
                  errorRatePct: 4.8,
                }
              : a
          )
        );
        addToast({
          type: 'warning',
          title: 'Simulated NIP Gateway Latency Injected',
          message: 'NIBSS NIP TSQ query latency spiked to 840ms. Circuit breaker tripped to HALF_OPEN.',
        });
      } else if (type === 'TRIP_BREAKER') {
        setAdapters((prev) =>
          prev.map((a) =>
            a.id === 'adapter-finacle'
              ? {
                  ...a,
                  circuitBreaker: 'OPEN',
                  status: 'DOWN',
                  errorRatePct: 12.5,
                }
              : a
          )
        );
        addToast({
          type: 'error',
          title: 'Finacle Circuit Breaker Tripped (OPEN)',
          message: 'Inbound reversal postings diverted to fallback durable queue.',
        });
      }
    }, 600);
  };

  return (
    <div className="space-y-6">
      {/* Top Tab Switcher */}
      <div className="flex items-center gap-2 border-b border-[var(--surface-border)] pb-2">
        <button
          onClick={() => setActiveTab('RULES')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 ${
            activeTab === 'RULES'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <Sliders className="w-4 h-4" />
          <span>Business Rules & SLA Thresholds</span>
        </button>

        <button
          onClick={() => setActiveTab('HEALTH')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 ${
            activeTab === 'HEALTH'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <Activity className="w-4 h-4" />
          <span>Core Banking & Switch Integration Health</span>
        </button>

        <button
          onClick={() => setActiveTab('SWITCH_CBS')}
          className={`px-4 py-2 text-[13px] font-bold rounded-md transition-colors flex items-center gap-2 ${
            activeTab === 'SWITCH_CBS'
              ? 'bg-slate-900 text-white shadow-xs'
              : 'bg-[var(--surface)] border border-[var(--surface-border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          <Server className="w-4 h-4" />
          <span>Switch & CBS Packet Simulator</span>
        </button>
      </div>

      {/* Tab Contents */}
      {activeTab === 'RULES' ? (
        <RulesEditorTab
          config={config}
          onSaveProposedConfig={handleSaveProposedConfig}
          onResetDefaults={handleResetDefaults}
        />
      ) : activeTab === 'HEALTH' ? (
        <IntegrationHealthTab
          adapters={adapters}
          onToggleCircuitBreaker={handleToggleCircuitBreaker}
          onInjectFault={handleInjectFault}
          isInjecting={isInjecting}
        />
      ) : (
        <SwitchPacketAndCBSInspector />
      )}
    </div>
  );
};
