import React, { useState } from 'react';
import clsx from 'clsx';

export interface DataPoint {
  label: string;
  value: number;
  secondaryValue?: number;
  formattedValue: string;
}

export interface ChartWrapperProps {
  title: string;
  subtitle?: string;
  data: DataPoint[];
  metricLabel: string;
  currency?: string;
  height?: number;
  className?: string;
}

export const ChartWrapper: React.FC<ChartWrapperProps> = ({
  title,
  subtitle,
  data,
  metricLabel,
  height = 140,
  className,
}) => {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);

  if (data.length === 0) return null;

  const values = data.map((d) => d.value);
  const minVal = Math.min(...values);
  const maxVal = Math.max(...values);
  const range = maxVal - minVal || 1;

  const width = 500;
  const paddingX = 20;
  const paddingY = 20;
  const chartWidth = width - paddingX * 2;
  const chartHeight = height - paddingY * 2;

  // Calculate coordinates
  const points = data.map((d, i) => {
    const x = paddingX + (i / (data.length - 1)) * chartWidth;
    const y = height - paddingY - ((d.value - minVal) / range) * chartHeight;
    return { x, y, data: d };
  });

  const pathD = points.reduce((acc, curr, i) => {
    return i === 0 ? `M ${curr.x} ${curr.y}` : `${acc} L ${curr.x} ${curr.y}`;
  }, '');

  const activePoint = hoveredIndex !== null ? points[hoveredIndex] : points[points.length - 1];

  return (
    <div
      className={clsx(
        'p-4 bg-[var(--surface)] border border-[var(--surface-border)] rounded-md shadow-xs flex flex-col justify-between',
        className
      )}
      role="region"
      aria-label={`${title} Trend Chart`}
    >
      {/* Header with Title and Current Value */}
      <div className="flex items-start justify-between gap-4 mb-2">
        <div>
          <h4 className="text-[13px] font-semibold text-[var(--text-primary)]">{title}</h4>
          {subtitle && <p className="text-[11px] text-[var(--text-muted)]">{subtitle}</p>}
        </div>
        {activePoint && (
          <div className="text-right">
            <div className="text-[11px] text-[var(--text-muted)] font-medium">
              {activePoint.data.label}
            </div>
            <div className="text-[16px] font-bold font-mono-financial text-[var(--text-primary)]">
              {activePoint.data.formattedValue}
            </div>
          </div>
        )}
      </div>

      {/* SVG Financial Line Chart (No gradients, high contrast clean lines) */}
      <div className="relative w-full overflow-hidden">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          className="w-full h-auto overflow-visible select-none"
          preserveAspectRatio="none"
        >
          {/* Baseline Gridlines */}
          <line
            x1={paddingX}
            y1={height - paddingY}
            x2={width - paddingX}
            y2={height - paddingY}
            stroke="var(--surface-border)"
            strokeWidth="1"
          />
          <line
            x1={paddingX}
            y1={paddingY}
            x2={width - paddingX}
            y2={paddingY}
            stroke="var(--surface-border-subtle)"
            strokeWidth="1"
            strokeDasharray="3 3"
          />

          {/* Precise Solid Metric Line */}
          <path
            d={pathD}
            fill="none"
            stroke="var(--primary)"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />

          {/* Interactive Data Point Dots */}
          {points.map((p, idx) => {
            const isHovered = hoveredIndex === idx;
            return (
              <g key={idx} onMouseEnter={() => setHoveredIndex(idx)}>
                <circle
                  cx={p.x}
                  cy={p.y}
                  r={isHovered ? 4.5 : 2.5}
                  fill="var(--surface)"
                  stroke="var(--primary)"
                  strokeWidth={isHovered ? 2.5 : 1.5}
                  className="transition-all cursor-pointer"
                />
                {/* Hit area */}
                <rect
                  x={p.x - 12}
                  y={0}
                  width={24}
                  height={height}
                  fill="transparent"
                  className="cursor-pointer"
                />
              </g>
            );
          })}

          {/* Active Guide Line */}
          {activePoint && (
            <line
              x1={activePoint.x}
              y1={paddingY}
              x2={activePoint.x}
              y2={height - paddingY}
              stroke="var(--primary)"
              strokeWidth="1"
              strokeDasharray="2 2"
              opacity="0.6"
            />
          )}
        </svg>
      </div>

      {/* Axis Footer */}
      <div className="flex items-center justify-between text-[11px] font-mono-financial text-[var(--text-muted)] mt-1 pt-1 border-t border-[var(--surface-border-subtle)]">
        <span>{data[0]?.label}</span>
        <span>{metricLabel}</span>
        <span>{data[data.length - 1]?.label}</span>
      </div>
    </div>
  );
};
