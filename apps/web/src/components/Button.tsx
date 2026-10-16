import React from 'react';
import { Loader2 } from 'lucide-react';
import clsx from 'clsx';

export type ButtonVariant = 'primary' | 'secondary' | 'danger' | 'ghost';
export type ButtonSize = 'xs' | 'sm' | 'md' | 'lg';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  isLoading?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      variant = 'primary',
      size = 'md',
      isLoading = false,
      disabled = false,
      leftIcon,
      rightIcon,
      children,
      className,
      ...props
    },
    ref
  ) => {
    const isDisabled = disabled || isLoading;

    const baseStyles =
      'inline-flex items-center justify-center font-medium transition-colors select-none rounded focus-visible:outline-2 focus-visible:outline-offset-2 disabled:opacity-50 disabled:pointer-events-none cursor-pointer';

    const sizeStyles = {
      xs: 'text-[11px] px-2 py-0.5 gap-1 min-h-[24px]',
      sm: 'text-[12px] px-2.5 py-1 gap-1.5 min-h-[28px]',
      md: 'text-[13px] px-3.5 py-1.5 gap-2 min-h-[34px]',
      lg: 'text-[14px] px-4 py-2 gap-2.5 min-h-[40px]',
    };

    const variantStyles = {
      primary:
        'bg-[var(--primary)] text-white hover:bg-[var(--primary-hover)] active:bg-[var(--primary-active)] focus-visible:outline-[var(--focus-ring)] shadow-xs',
      secondary:
        'bg-[var(--surface)] text-[var(--text-primary)] border border-[var(--surface-border)] hover:bg-[var(--surface-raised)] hover:border-[var(--text-muted)] active:bg-[var(--canvas-subdued)] focus-visible:outline-[var(--focus-ring)] shadow-xs',
      danger:
        'bg-[#dc2626] text-white hover:bg-[#b91c1c] active:bg-[#991b1b] focus-visible:outline-[#dc2626] shadow-xs',
      ghost:
        'bg-transparent text-[var(--text-secondary)] hover:bg-[var(--canvas-subdued)] hover:text-[var(--text-primary)] active:bg-[var(--surface-border-subtle)] focus-visible:outline-[var(--focus-ring)]',
    };

    return (
      <button
        ref={ref}
        disabled={isDisabled}
        aria-busy={isLoading}
        className={clsx(
          baseStyles,
          sizeStyles[size],
          variantStyles[variant],
          className
        )}
        {...props}
      >
        {isLoading ? (
          <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
        ) : (
          leftIcon
        )}
        <span>{children}</span>
        {!isLoading && rightIcon}
      </button>
    );
  }
);

Button.displayName = 'Button';
