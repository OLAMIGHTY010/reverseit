import React from 'react';
import clsx from 'clsx';
import { AlertCircle } from 'lucide-react';

export interface TextFieldProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label: string;
  error?: string;
  helperText?: string;
}

export const TextField = React.forwardRef<HTMLInputElement, TextFieldProps>(
  ({ label, error, helperText, id, className, ...props }, ref) => {
    const inputId = id || `field-${label.toLowerCase().replace(/\s+/g, '-')}`;
    const errorId = `${inputId}-error`;
    const helperId = `${inputId}-helper`;

    return (
      <div className={clsx('space-y-1 text-[13px]', className)}>
        <label htmlFor={inputId} className="block font-medium text-[var(--text-secondary)]">
          {label}
        </label>
        <input
          ref={ref}
          id={inputId}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? errorId : helperText ? helperId : undefined}
          className={clsx(
            'w-full px-3 py-1.5 bg-[var(--surface)] text-[var(--text-primary)] border rounded transition-colors focus:outline-2',
            error
              ? 'border-rose-500 focus:outline-rose-500 bg-rose-50/20'
              : 'border-[var(--surface-border)] focus:outline-[var(--focus-ring)]'
          )}
          {...props}
        />
        {error && (
          <div id={errorId} className="flex items-center gap-1 text-[12px] text-rose-600 font-medium">
            <AlertCircle className="w-3.5 h-3.5" aria-hidden="true" />
            <span>{error}</span>
          </div>
        )}
        {!error && helperText && (
          <p id={helperId} className="text-[11px] text-[var(--text-muted)]">
            {helperText}
          </p>
        )}
      </div>
    );
  }
);

TextField.displayName = 'TextField';
