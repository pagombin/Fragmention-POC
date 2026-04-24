import { useAuth } from '../../stores/auth';
import { useUI } from '../../stores/ui';
import { useTopology } from '../../api/hooks';
import { Badge, Button, cx } from '../ui/primitives';

export function TopBar() {
  const { theme, toggleTheme } = useUI();
  const { token, setToken } = useAuth();
  const { data, isLoading, isError } = useTopology();

  let statusTone: 'success' | 'warn' | 'error' = 'error';
  let statusLabel = 'disconnected';
  if (isLoading) { statusTone = 'warn'; statusLabel = 'connecting'; }
  else if (isError) { statusTone = 'error'; statusLabel = 'disconnected'; }
  else if (data) { statusTone = 'success'; statusLabel = 'connected'; }

  return (
    <header className="flex h-12 items-center justify-between border-b border-slate-200 bg-white/90 px-4 backdrop-blur dark:border-slate-800 dark:bg-slate-950/90">
      <div className="flex items-center gap-3">
        <span className="font-semibold tracking-tight">mfpoc</span>
        <Badge tone={statusTone} title={data?.redacted_uri}>{statusLabel}</Badge>
        {data && (
          <span className="text-xs text-slate-500 dark:text-slate-400 hidden md:inline">
            {data.topology.kind}
            {data.topology.replica_set ? ` · ${data.topology.replica_set}` : ''}
            {data.server_info.Version ? ` · mongo ${data.server_info.Version}` : ''}
            {data.is_srv ? ' · SRV' : ''}
          </span>
        )}
      </div>
      <div className="flex items-center gap-2">
        <input
          className={cx(
            'mono-sm w-64 rounded border px-2 py-1 text-xs',
            'border-slate-300 bg-white dark:border-slate-700 dark:bg-slate-900',
          )}
          placeholder={token ? 'token set' : 'bearer token'}
          type="password"
          value={token ?? ''}
          onChange={(e) => setToken(e.target.value || null)}
        />
        <Button variant="ghost" size="sm" onClick={toggleTheme} title="Toggle theme">
          {theme === 'dark' ? '☾' : '☀︎'}
        </Button>
      </div>
    </header>
  );
}
