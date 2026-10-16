import React from 'react';
import {
  Server,
  Activity,
  Zap,
  Power,
  RotateCcw,
} from 'lucide-react';
import { Button } from '../../components/Button';
import type { AdapterHealthStatus, CircuitBreakerState } from './rulesTypes';

interface IntegrationHealthTabProps {
  adapters: AdapterHealthStatus[];
  onToggleCircuitBreaker: (adapterId: string, newState: CircuitBreakerState) => void;
  onInjectFault: (type: 'LATENCY_SPIKE' | 'TRIP_BREAKER' | 'RESET') => void;
  isInjecting: boolean;
}

export const IntegrationHealthTab: React.FC<IntegrationHealthTabProps> = ({
  adapters,
  onToggleCircuitBreaker,
  onInjectFault,
  isInjecting,
}) => {
  return (
    <div className="space-y-6">
      {/* Top Header & Fault Injection Bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-lg shadow-xs">
        <div>
          <div className="flex items-center gap-2">
            <Activity className="w-5 h-5 text-emerald-600" />
            <h3 className="text-[16px] font-bold text-[var(--text-primary)]">
              Core Banking & Switch Integration Health Dashboard
            </h3>
            <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-emerald-100 text-emerald-800 font-mono-financial">
              Live Circuit Breaker Matrix
            </span>
          </div>
          <p className="text-[12px] text-[var(--text-muted)] mt-0.5">
            Real-time telemetry, ping latencies, and circuit breaker protection across Finacle,
            NIBSS, Postilion, and Mastercard.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onInjectFault('RESET')}
            leftIcon={<RotateCcw className="w-3.5 h-3.5" />}
          >
            Reset Telemetry
          </Button>

          <Button
            variant="secondary"
            size="sm"
            isLoading={isInjecting}
            onClick={() => onInjectFault('LATENCY_SPIKE')}
            leftIcon={<Zap className="w-3.5 h-3.5 text-amber-600" />}
          >
            Inject NIP Latency Spike
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={() => onInjectFault('TRIP_BREAKER')}
            leftIcon={<Power className="w-3.5 h-3.5 text-rose-300" />}
          >
            Trip Finacle Breaker
          </Button>
        </div>
      </div>

      {/* Grid of Adapter Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {adapters.map((adapter) => {
          const isBreakerOpen = adapter.circuitBreaker === 'OPEN';
          const isBreakerHalf = adapter.circuitBreaker === 'HALF_OPEN';

          return (
            <div
              key={adapter.id}
              className={`p-4 rounded-lg border shadow-xs space-y-3 transition-all ${
                adapter.status === 'DOWN' || isBreakerOpen
                  ? 'bg-rose-50/30 border-rose-400'
                  : adapter.status === 'DEGRADED' || isBreakerHalf
                  ? 'bg-amber-50/20 border-amber-400'
                  : 'bg-[var(--surface)] border-[var(--surface-border)]'
              }`}
            >
              {/* Card Header */}
              <div className="flex items-start justify-between gap-2">
                <div className="space-y-0.5">
                  <div className="font-bold text-[13px] text-[var(--text-primary)] flex items-center gap-1.5">
                    <Server className="w-4 h-4 text-indigo-600 shrink-0" />
                    <span>{adapter.name}</span>
                  </div>
                  <div className="text-[10px] font-mono-financial text-[var(--text-muted)]">
                    {adapter.protocol} • {adapter.channel}
                  </div>
                </div>

                <span
                  className={`text-[10px] font-bold px-2 py-0.5 rounded font-mono-financial ${
                    adapter.status === 'HEALTHY'
                      ? 'bg-emerald-100 text-emerald-800 border border-emerald-300'
                      : adapter.status === 'DEGRADED'
                      ? 'bg-amber-100 text-amber-800 border border-amber-300'
                      : 'bg-rose-100 text-rose-800 border border-rose-300'
                  }`}
                >
                  {adapter.status}
                </span>
              </div>

              {/* Endpoint */}
              <div className="p-2 bg-[var(--canvas-subdued)] border border-[var(--surface-border-subtle)] rounded text-[10px] font-mono-financial text-[var(--text-secondary)] truncate">
                {adapter.endpoint}
              </div>

              {/* Metrics Grid */}
              <div className="grid grid-cols-2 gap-2 text-[11px] font-mono-financial">
                <div className="p-2 bg-[var(--canvas-subdued)] rounded space-y-0.5">
                  <span className="text-[10px] text-[var(--text-muted)] font-sans block">
                    Ping Latency
                  </span>
                  <span
                    className={`font-bold text-[13px] ${
                      adapter.pingLatencyMs > 200
                        ? 'text-rose-700'
                        : adapter.pingLatencyMs > 100
                        ? 'text-amber-700'
                        : 'text-emerald-700'
                    }`}
                  >
                    {adapter.pingLatencyMs} ms
                  </span>
                </div>

                <div className="p-2 bg-[var(--canvas-subdued)] rounded space-y-0.5">
                  <span className="text-[10px] text-[var(--text-muted)] font-sans block">
                    Active Throughput
                  </span>
                  <span className="font-bold text-[13px] text-[var(--text-primary)]">
                    {adapter.activeTps.toLocaleString()} TPS
                  </span>
                </div>

                <div className="p-2 bg-[var(--canvas-subdued)] rounded space-y-0.5">
                  <span className="text-[10px] text-[var(--text-muted)] font-sans block">
                    Uptime SLA (30d)
                  </span>
                  <span className="font-bold text-[12px] text-[var(--text-primary)]">
                    {adapter.uptimePct}%
                  </span>
                </div>

                <div className="p-2 bg-[var(--canvas-subdued)] rounded space-y-0.5">
                  <span className="text-[10px] text-[var(--text-muted)] font-sans block">
                    Error Rate
                  </span>
                  <span
                    className={`font-bold text-[12px] ${
                      adapter.errorRatePct > 1.0 ? 'text-rose-700' : 'text-emerald-700'
                    }`}
                  >
                    {adapter.errorRatePct}%
                  </span>
                </div>
              </div>

              {/* Circuit Breaker Controls */}
              <div className="pt-2 border-t border-[var(--surface-border-subtle)] flex items-center justify-between">
                <div className="flex items-center gap-1.5 text-[11px] font-mono-financial">
                  <span className="text-[var(--text-muted)]">Circuit:</span>
                  <span
                    className={`font-bold text-[10px] px-1.5 py-0.2 rounded ${
                      adapter.circuitBreaker === 'CLOSED'
                        ? 'bg-emerald-100 text-emerald-800'
                        : adapter.circuitBreaker === 'HALF_OPEN'
                        ? 'bg-amber-100 text-amber-800'
                        : 'bg-rose-100 text-rose-800'
                    }`}
                  >
                    {adapter.circuitBreaker}
                  </span>
                </div>

                <div className="flex items-center gap-1">
                  {adapter.circuitBreaker !== 'CLOSED' && (
                    <button
                      onClick={() => onToggleCircuitBreaker(adapter.id, 'CLOSED')}
                      className="px-2 py-0.5 text-[10px] font-bold rounded bg-emerald-600 text-white hover:bg-emerald-700"
                    >
                      Close Breaker
                    </button>
                  )}
                  {adapter.circuitBreaker !== 'OPEN' && (
                    <button
                      onClick={() => onToggleCircuitBreaker(adapter.id, 'OPEN')}
                      className="px-2 py-0.5 text-[10px] font-bold rounded bg-rose-600 text-white hover:bg-rose-700"
                    >
                      Trip
                    </button>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
