import { useMemo, useState } from 'react';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, cx, formatBytes } from './ui/primitives';
import { useCollections, useDatabases } from '../api/hooks';
import { useSelection } from '../stores/selection';

interface TargetSelectorProps {
  // onChange is called with the flat list of selected (db, collection) pairs
  // whenever the set changes. Callers typically also read from the shared
  // useSelection() store, which is already updated in-place here.
  onChange?: (targets: { database: string; collection: string }[]) => void;
  // Optional filter: only collections with these types are selectable.
  // Defaults to ['regular'] so system/capped/timeseries scopes cannot be
  // accidentally included in a loader/deleter run.
  allowTypes?: string[];
}

export function TargetSelector({ onChange, allowTypes = ['regular'] }: TargetSelectorProps) {
  const dbs = useDatabases();
  const sel = useSelection();
  const [expandedDB, setExpandedDB] = useState<string | null>(null);
  const [filter, setFilter] = useState('');

  const filteredDBs = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const list = dbs.data ?? [];
    return q ? list.filter((d) => d.name.toLowerCase().includes(q)) : list;
  }, [filter, dbs.data]);

  const emit = (next: Set<string>) => {
    sel.set([...next]);
    if (onChange) {
      onChange([...next].map((k) => {
        const i = k.indexOf('.');
        return { database: k.slice(0, i), collection: k.slice(i + 1) };
      }));
    }
  };

  const toggle = (key: string) => {
    const next = new Set(sel.selected);
    if (next.has(key)) next.delete(key); else next.add(key);
    emit(next);
  };

  return (
    <Card>
      <CardHeader className="flex items-center justify-between">
        <CardTitle className="normal-case text-sm tracking-normal">Target selection</CardTitle>
        <div className="flex items-center gap-2">
          <Input placeholder="filter" value={filter} onChange={(e) => setFilter(e.target.value)} className="w-48" />
          <Button variant="ghost" size="sm" onClick={() => sel.clear()}>Clear</Button>
          <Badge>{sel.selected.size} selected</Badge>
        </div>
      </CardHeader>
      <CardBody className="max-h-[480px] overflow-auto">
        {dbs.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
        {filteredDBs.length === 0 && !dbs.isLoading && (
          <div className="text-slate-500 text-sm">No databases.</div>
        )}
        <div className="space-y-2">
          {filteredDBs.map((db) => (
            <DBBranch
              key={db.name}
              name={db.name}
              collections={db.collections}
              expanded={expandedDB === db.name}
              onExpand={() => setExpandedDB(expandedDB === db.name ? null : db.name)}
              toggle={toggle}
              selected={sel.selected}
              allowTypes={allowTypes}
            />
          ))}
        </div>
      </CardBody>
    </Card>
  );
}

function DBBranch({
  name, collections, expanded, onExpand, toggle, selected, allowTypes,
}: {
  name: string;
  collections: number;
  expanded: boolean;
  onExpand: () => void;
  toggle: (key: string) => void;
  selected: Set<string>;
  allowTypes: string[];
}) {
  const colls = useCollections(expanded ? name : null);
  const nSelected = [...selected].filter((k) => k.startsWith(name + '.')).length;

  return (
    <div className="rounded border border-slate-200 dark:border-slate-800">
      <div className="flex items-center justify-between px-2 py-1.5">
        <div className="flex items-center gap-2">
          <button onClick={onExpand} className="text-xs">{expanded ? '▾' : '▸'}</button>
          <span className="font-mono text-sm">{name}</span>
          <Badge tone="neutral">{collections} colls</Badge>
          {nSelected > 0 && <Badge tone="info">{nSelected} sel</Badge>}
        </div>
      </div>
      {expanded && (
        <div className="border-t border-slate-200 dark:border-slate-800">
          {colls.isLoading && <div className="p-2 text-xs text-slate-500">Loading…</div>}
          {colls.data?.length === 0 && <div className="p-2 text-xs text-slate-500">No collections.</div>}
          {colls.data?.map((c) => {
            const key = `${name}.${c.name}`;
            const allowed = allowTypes.includes(c.type);
            return (
              <label
                key={key}
                className={cx(
                  'flex items-center justify-between border-t border-slate-100 px-2 py-1 text-sm dark:border-slate-900',
                  !allowed && 'opacity-50 cursor-not-allowed',
                  selected.has(key) && 'bg-brand-500/5',
                )}
              >
                <span className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={selected.has(key)}
                    onChange={() => allowed && toggle(key)}
                    disabled={!allowed}
                  />
                  <span className="font-mono text-xs">{c.name}</span>
                  {c.type !== 'regular' && <Badge tone="warn">{c.type}</Badge>}
                </span>
                <span className="text-xs text-slate-500">
                  {c.count.toLocaleString()} docs · {formatBytes(c.storage_size)} · {(c.fragmentation_ratio * 100).toFixed(1)}% frag
                </span>
              </label>
            );
          })}
        </div>
      )}
    </div>
  );
}
