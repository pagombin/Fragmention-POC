import { useUI } from '../../stores/ui';
import { cx } from './primitives';

export function Toasts() {
  const { toasts, dismissToast } = useUI();
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          onClick={() => dismissToast(t.id)}
          className={cx(
            'cursor-pointer rounded border px-3 py-2 shadow-lg text-sm max-w-sm',
            t.kind === 'error' && 'border-red-500 bg-red-50 text-red-900 dark:bg-red-900/40 dark:text-red-100',
            t.kind === 'warn' && 'border-yellow-500 bg-yellow-50 text-yellow-900 dark:bg-yellow-900/40 dark:text-yellow-100',
            t.kind === 'success' && 'border-green-500 bg-green-50 text-green-900 dark:bg-green-900/40 dark:text-green-100',
            t.kind === 'info' && 'border-brand-500 bg-brand-50 text-brand-900 dark:bg-brand-900/40 dark:text-brand-100',
          )}
        >
          <div className="font-semibold">{t.title}</div>
          {t.body && <div className="mt-0.5 text-xs opacity-90">{t.body}</div>}
        </div>
      ))}
    </div>
  );
}
