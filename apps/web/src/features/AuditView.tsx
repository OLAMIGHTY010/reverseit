import React, { useState } from 'react';
import { Lock } from 'lucide-react';
import { DataTable, type ColumnDef } from '../components/DataTable';
import { INITIAL_AUDIT_TRAIL } from '../services/mockSimulatorData';
import type { AuditEvent } from '../types/domain';

export const AuditView: React.FC = () => {
  const [logs] = useState<AuditEvent[]>(() => INITIAL_AUDIT_TRAIL);

  const columns: ColumnDef<AuditEvent>[] = [
    {
      id: 'timestamp',
      header: 'Timestamp (UTC)',
      accessorKey: 'timestamp',
      cell: ({ row }) => (
        <span className="font-mono-financial text-[12px] text-[var(--text-muted)]">
          {new Date(row.timestamp).toISOString()}
        </span>
      ),
      minWidth: 190,
    },
    {
      id: 'action',
      header: 'Action Taken',
      accessorKey: 'action',
      cell: ({ row }) => (
        <span className="font-bold text-[12px] text-[var(--text-primary)]">
          {row.action}
        </span>
      ),
      minWidth: 200,
    },
    {
      id: 'actorId',
      header: 'Operator / System',
      accessorKey: 'actorId',
      cell: ({ row }) => (
        <span className="font-mono-financial text-[12px]">{row.actorId}</span>
      ),
      minWidth: 160,
    },
    {
      id: 'actorRole',
      header: 'Role',
      accessorKey: 'actorRole',
      cell: ({ row }) => (
        <span className="px-1.5 py-0.5 rounded bg-[var(--canvas-subdued)] border border-[var(--surface-border)] font-mono-financial text-[11px]">
          {row.actorRole}
        </span>
      ),
      minWidth: 150,
    },
    {
      id: 'entity',
      header: 'Target Entity',
      cell: ({ row }) => (
        <span className="font-mono-financial text-[12px]">
          {row.entityType}: {row.entityId}
        </span>
      ),
      minWidth: 180,
    },
    {
      id: 'entryHash',
      header: 'Cryptographic SHA-256 Hash',
      accessorKey: 'entryHash',
      cell: ({ row }) => (
        <span className="font-mono-financial text-[11px] text-[var(--text-muted)] truncate max-w-xs block" title={row.entryHash}>
          {row.entryHash}
        </span>
      ),
      minWidth: 220,
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4 p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xs">
        <div>
          <h2 className="text-[16px] font-bold text-[var(--text-primary)] flex items-center gap-2">
            <Lock className="w-5 h-5 text-emerald-700" />
            <span>Immutable Regulatory Audit Trail</span>
          </h2>
          <p className="text-[13px] text-[var(--text-muted)] mt-0.5">
            Tamper-evident hash chain recording every investigation, dual-control sign-off, and sensitive field access.
          </p>
        </div>
      </div>

      <DataTable
        data={logs}
        columns={columns}
        pageSize={10}
        exportFilename="smartreversal_audit_trail.csv"
      />
    </div>
  );
};
