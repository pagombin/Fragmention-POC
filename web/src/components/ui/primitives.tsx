import React from 'react';

// Minimal shadcn-style primitives. Every component accepts className so
// consumers can extend styling without forking the primitive.

export function cx(...parts: (string | false | undefined | null)[]): string {
  return parts.filter(Boolean).join(' ');
}

export const Card: React.FC<React.HTMLAttributes<HTMLDivElement>> = ({ className, ...p }) => (
  <div
    className={cx(
      'rounded-lg border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900',
      className,
    )}
    {...p}
  />
);

export const CardHeader: React.FC<React.HTMLAttributes<HTMLDivElement>> = ({ className, ...p }) => (
  <div className={cx('p-4 border-b border-slate-200 dark:border-slate-800', className)} {...p} />
);

export const CardBody: React.FC<React.HTMLAttributes<HTMLDivElement>> = ({ className, ...p }) => (
  <div className={cx('p-4', className)} {...p} />
);

export const CardTitle: React.FC<React.HTMLAttributes<HTMLHeadingElement>> = ({ className, ...p }) => (
  <h3 className={cx('text-sm font-medium tracking-wide uppercase text-slate-500 dark:text-slate-400', className)} {...p} />
);

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  size?: 'sm' | 'md';
};

export const Button: React.FC<ButtonProps> = ({ className, variant = 'secondary', size = 'md', disabled, ...p }) => {
  const base = 'inline-flex items-center justify-center rounded font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus:ring-2 focus:ring-brand-400';
  const sizes = { sm: 'px-2 py-1 text-xs', md: 'px-3 py-1.5 text-sm' };
  const variants = {
    primary: 'bg-brand-600 text-white hover:bg-brand-500',
    secondary: 'bg-slate-200 text-slate-900 hover:bg-slate-300 dark:bg-slate-800 dark:text-slate-100 dark:hover:bg-slate-700',
    danger: 'bg-red-600 text-white hover:bg-red-500',
    ghost: 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800',
  };
  return <button className={cx(base, sizes[size], variants[variant], className)} disabled={disabled} {...p} />;
};

export const Input: React.FC<React.InputHTMLAttributes<HTMLInputElement>> = ({ className, ...p }) => (
  <input
    className={cx(
      'rounded border border-slate-300 bg-white px-2 py-1 text-sm text-slate-900 placeholder:text-slate-400',
      'focus:outline-none focus:ring-2 focus:ring-brand-400',
      'dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100',
      className,
    )}
    {...p}
  />
);

export const Select: React.FC<React.SelectHTMLAttributes<HTMLSelectElement>> = ({ className, ...p }) => (
  <select
    className={cx(
      'rounded border border-slate-300 bg-white px-2 py-1 text-sm text-slate-900',
      'focus:outline-none focus:ring-2 focus:ring-brand-400',
      'dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100',
      className,
    )}
    {...p}
  />
);

export const Label: React.FC<React.LabelHTMLAttributes<HTMLLabelElement>> = ({ className, ...p }) => (
  <label className={cx('block text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-1', className)} {...p} />
);

type BadgeProps = React.HTMLAttributes<HTMLSpanElement> & { tone?: 'info' | 'success' | 'warn' | 'error' | 'neutral' };

export const Badge: React.FC<BadgeProps> = ({ className, tone = 'neutral', ...p }) => {
  const tones = {
    neutral: 'bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-slate-200',
    info: 'bg-brand-500/15 text-brand-600 dark:text-brand-300',
    success: 'bg-green-500/15 text-green-600 dark:text-green-300',
    warn: 'bg-yellow-500/15 text-yellow-600 dark:text-yellow-300',
    error: 'bg-red-500/15 text-red-600 dark:text-red-300',
  };
  return <span className={cx('inline-block rounded px-1.5 py-0.5 text-xs font-medium', tones[tone], className)} {...p} />;
};

export const Table: React.FC<React.TableHTMLAttributes<HTMLTableElement>> = ({ className, ...p }) => (
  <table className={cx('w-full text-sm', className)} {...p} />
);

export const Th: React.FC<React.ThHTMLAttributes<HTMLTableCellElement>> = ({ className, ...p }) => (
  <th
    className={cx(
      'px-2 py-1 text-left font-medium text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800',
      className,
    )}
    {...p}
  />
);

export const Td: React.FC<React.TdHTMLAttributes<HTMLTableCellElement>> = ({ className, ...p }) => (
  <td className={cx('px-2 py-1 border-b border-slate-100 dark:border-slate-800/60', className)} {...p} />
);

export const ProgressBar: React.FC<{ value: number; max?: number; className?: string }> = ({ value, max = 1, className }) => {
  const pct = max > 0 ? Math.min(1, Math.max(0, value / max)) : 0;
  return (
    <div className={cx('h-1.5 w-full rounded bg-slate-200 dark:bg-slate-800 overflow-hidden', className)}>
      <div className="h-full bg-brand-500 transition-all" style={{ width: `${pct * 100}%` }} />
    </div>
  );
};

export function formatBytes(n: number): string {
  if (n === 0 || !Number.isFinite(n)) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.min(Math.floor(Math.log(Math.abs(n)) / Math.log(1024)), u.length - 1);
  return `${(n / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

export function formatNumber(n: number): string {
  if (!Number.isFinite(n)) return '0';
  return n.toLocaleString();
}

export function fragTone(ratio: number): 'frag-low' | 'frag-med' | 'frag-high' {
  if (ratio < 0.1) return 'frag-low';
  if (ratio < 0.3) return 'frag-med';
  return 'frag-high';
}
