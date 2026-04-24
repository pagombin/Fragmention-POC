import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, ProgressBar, Table, Td, Th, cx, formatBytes, formatNumber, fragTone } from '../components/ui/primitives';
import { Page } from './ClusterOverview';
import { useCollections, useDatabases } from '../api/hooks';
import { useSelection } from '../stores/selection';
import type { CollectionSummary, DatabaseSummary } from '../api/types';

// The Data Browser lists every database and collection with live stats. The
// Operations Console reads useSelection() to prefill target specs, so a
// reviewer can "select 3 collections, jump to Load" in two clicks.

export function DataBrowser() {
  const [filter, setFilter] = useState('');
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const dbs = useDatabases();
  const sel = useSelection();
  const nav = useNavigate();

  const filtered = useMemo<DatabaseSummary[]>(() => {
    const q = filter.trim().toLowerCase();
    if (!q || !dbs.data) return dbs.data ?? [];
    return dbs.data.filter((d) => d.name.toLowerCase().includes(q));
  }, [filter, dbs.data]);

  const hasSelection = sel.selected.size > 0;

  const jumpTo = (path: string) => {
    if (!hasSelection) {
      alert('Select at least one collection first.');
      return;
    }
    nav(path);
  };

  return (
    <Page>
      <div className="flex items-center justify-between gap-2 mb-3">
        <div className="flex items-center gap-2">
          <Input placeholder="filter databases…" value={filter} onChange={(e) => setFilter(e.target.value)} className="w-64" />
          <Button variant="ghost" size="sm" onClick={() => dbs.refetch()}>Refresh</Button>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500">{sel.selected.size} selected</span>
          <Button variant="ghost" size="sm" onClick={() => sel.clear()} disabled={!hasSelection}>Clear</Button>
          <Button variant="primary" size="sm" onClick={() => jumpTo('/ops?tab=load')} disabled={!hasSelection}>Load into selected</Button>
          <Button variant="primary" size="sm" onClick={() => jumpTo('/ops?tab=delete')} disabled={!hasSelection}>Delete from selected</Button>
          <Button variant="primary" size="sm" onClick={() => jumpTo('/ops?tab=compact')} disabled={!hasSelection}>Compact selected</Button>
        </div>
      </div>

      {dbs.isLoading && <Card><CardBody>Loading databases…</CardBody></Card>}
      {dbs.isError && <Card><CardBody className="text-red-500">Failed: {String(dbs.error)}</CardBody></Card>}

      <div className="space-y-3">
        {filtered.map((db) => (
          <DatabaseCard
            key={db.name}
            db={db}
            expanded={expanded.has(db.name)}
            onToggleExpand={() => {
              setExpanded((s) => {
                const n = new Set(s);
                if (n.has(db.name)) n.delete(db.name); else n.add(db.name);
                return n;
              });
            }}
          />
        ))}
        {filtered.length === 0 && !dbs.isLoading && (
          <Card><CardBody className="text-slate-500">No databases match filter.</CardBody></Card>
        )}
      </div>
    </Page>
  );
}

function DatabaseCard({ db, expanded, onToggleExpand }: { db: DatabaseSummary; expanded: boolean; onToggleExpand: () => void }) {
  const frag = db.storage_size > 0 ? (db.storage_size - db.data_size) / db.storage_size : 0;
  const colls = useCollections(expanded ? db.name : null);
  const sel = useSelection();

  return (
    <Card>
      <CardHeader className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={onToggleExpand}>{expanded ? '▾' : '▸'}</Button>
          <CardTitle className="normal-case text-base tracking-normal">{db.name}</CardTitle>
          <Badge tone="neutral">{db.collections} colls</Badge>
          <span className={cx('rounded px-1.5 py-0.5 text-xs', fragTone(frag))}>
            {(frag * 100).toFixed(1)}% frag
          </span>
        </div>
        <div className="text-xs text-slate-500">
          data {formatBytes(db.data_size)} · storage {formatBytes(db.storage_size)} · idx {formatBytes(db.index_size)}
        </div>
      </CardHeader>
      {expanded && (
        <CardBody>
          {colls.isLoading && <div className="text-slate-500 text-sm">Loading collections…</div>}
          {colls.isError && <div className="text-red-500 text-sm">Failed: {String(colls.error)}</div>}
          {colls.data && (
            <Table>
              <thead>
                <tr>
                  <Th className="w-8"></Th>
                  <Th>Collection</Th>
                  <Th>Type</Th>
                  <Th className="text-right">Docs</Th>
                  <Th className="text-right">Size</Th>
                  <Th className="text-right">Storage</Th>
                  <Th className="text-right">Free</Th>
                  <Th className="text-right">Frag</Th>
                  <Th className="text-right">Indexes</Th>
                  <Th>Codec</Th>
                </tr>
              </thead>
              <tbody>
                {colls.data.map((c) => (
                  <CollectionRow key={c.name} db={db.name} c={c} selected={sel.selected.has(`${db.name}.${c.name}`)} onToggle={() => sel.toggle(`${db.name}.${c.name}`)} />
                ))}
              </tbody>
            </Table>
          )}
        </CardBody>
      )}
    </Card>
  );
}

function CollectionRow({ db, c, selected, onToggle }: { db: string; c: CollectionSummary; selected: boolean; onToggle: () => void }) {
  return (
    <tr className={cx(selected && 'bg-brand-500/5')}>
      <Td>
        <input type="checkbox" checked={selected} onChange={onToggle} aria-label={`select ${db}.${c.name}`} disabled={c.type !== 'regular'} />
      </Td>
      <Td className="font-mono">{c.name}</Td>
      <Td><Badge tone={c.type === 'regular' ? 'neutral' : 'warn'}>{c.type}</Badge></Td>
      <Td className="text-right font-mono text-xs">{formatNumber(c.count)}</Td>
      <Td className="text-right font-mono text-xs">{formatBytes(c.size)}</Td>
      <Td className="text-right font-mono text-xs">{formatBytes(c.storage_size)}</Td>
      <Td className="text-right font-mono text-xs">{formatBytes(c.free_storage_size)}</Td>
      <Td className="text-right">
        <div className={cx('inline-block rounded px-1.5 py-0.5 text-xs', fragTone(c.fragmentation_ratio))}>
          {(c.fragmentation_ratio * 100).toFixed(1)}%
        </div>
        <ProgressBar className="mt-1 w-20 inline-block" value={c.fragmentation_ratio} />
      </Td>
      <Td className="text-right font-mono text-xs">{c.num_indexes} · {formatBytes(c.total_index_size)}</Td>
      <Td className="font-mono text-xs">{c.compressor ?? '—'}</Td>
    </tr>
  );
}
