import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, ProgressBar, Table, Td, Th, cx, formatBytes, formatNumber, fragTone } from '../components/ui/primitives';
import { Modal } from '../components/ui/Modal';
import { Page } from './ClusterOverview';
import { useCollections, useDatabases, useDropCollection, useDropDatabase } from '../api/hooks';
import { useSelection } from '../stores/selection';
import { useUI } from '../stores/ui';
import { ApiError } from '../api/client';
import type { CollectionSummary, DatabaseSummary } from '../api/types';

const SYSTEM_DBS = new Set(['admin', 'config', 'local']);

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
      <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
        <div className="flex items-center gap-2">
          <Input placeholder="filter databases…" value={filter} onChange={(e) => setFilter(e.target.value)} className="w-64" title="Substring match against database names" />
          <Button variant="ghost" size="sm" onClick={() => dbs.refetch()} title="Re-fetch the database list">Refresh</Button>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500" title="Total collections currently checked across all expanded databases">{sel.selected.size} selected</span>
          <Button variant="ghost" size="sm" onClick={() => sel.clear()} disabled={!hasSelection} title="Uncheck every selected collection">Clear</Button>
          <Button variant="primary" size="sm" onClick={() => jumpTo('/ops?tab=load')} disabled={!hasSelection} title="Jump to Operations Console → Load with the selected collections preloaded">Load into selected</Button>
          <Button variant="primary" size="sm" onClick={() => jumpTo('/ops?tab=delete')} disabled={!hasSelection} title="Jump to Operations Console → Delete with the selected collections preloaded">Delete from selected</Button>
          <Button variant="primary" size="sm" onClick={() => jumpTo('/ops?tab=compact')} disabled={!hasSelection} title="Jump to Operations Console → Compact with the selected collections preloaded">Compact selected</Button>
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
  const dropDB = useDropDatabase();
  const pushToast = useUI((s) => s.pushToast);
  const [dropOpen, setDropOpen] = useState(false);
  const [confirmText, setConfirmText] = useState('');
  const isSystem = SYSTEM_DBS.has(db.name);

  const performDrop = async () => {
    try {
      await dropDB.mutateAsync(db.name);
      pushToast({ kind: 'success', title: 'Database dropped', body: db.name });
      setDropOpen(false);
      setConfirmText('');
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Drop failed', body: msg });
    }
  };

  return (
    <Card>
      <CardHeader className="flex items-center justify-between flex-wrap gap-2">
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={onToggleExpand} title={expanded ? 'Collapse' : 'Expand to see collections'}>{expanded ? '▾' : '▸'}</Button>
          <CardTitle className="normal-case text-base tracking-normal">{db.name}</CardTitle>
          {isSystem && <Badge tone="warn" title="System database; cannot be dropped or used as a target">system</Badge>}
          <Badge tone="neutral" title="Number of collections in this database">{db.collections} colls</Badge>
          <span className={cx('rounded px-1.5 py-0.5 text-xs', fragTone(frag))} title="Database-level fragmentation = (storageSize - dataSize) / storageSize">
            {(frag * 100).toFixed(1)}% frag
          </span>
        </div>
        <div className="flex items-center gap-2">
          <div className="text-xs text-slate-500">
            data {formatBytes(db.data_size)} · storage {formatBytes(db.storage_size)} · idx {formatBytes(db.index_size)}
          </div>
          {!isSystem && (
            <Button
              variant="danger"
              size="sm"
              onClick={() => setDropOpen(true)}
              title="Permanently drop this database. Cannot be undone."
            >
              Drop DB
            </Button>
          )}
        </div>
      </CardHeader>
      {expanded && (
        <CardBody>
          {colls.isLoading && <div className="text-slate-500 text-sm">Loading collections…</div>}
          {colls.isError && <div className="text-red-500 text-sm">Failed: {String(colls.error)}</div>}
          {colls.data && (
            <div className="overflow-x-auto">
              <Table>
                <thead>
                  <tr>
                    <Th className="w-8"></Th>
                    <Th>Collection</Th>
                    <Th>Type</Th>
                    <Th className="text-right" title="Documents in the collection">Docs</Th>
                    <Th className="text-right" title="Logical/uncompressed BSON size">Size</Th>
                    <Th className="text-right" title="On-disk WiredTiger storage">Storage</Th>
                    <Th className="text-right" title="freeStorageSize: bytes WiredTiger has marked reusable but not yet released to the filesystem">Free</Th>
                    <Th className="text-right" title="Fragmentation ratio = freeStorageSize / storageSize">Frag</Th>
                    <Th className="text-right" title="Number of indexes · total index size">Indexes</Th>
                    <Th title="WiredTiger block compressor (snappy/zlib/zstd/none)">Codec</Th>
                    {!isSystem && <Th className="text-right">Actions</Th>}
                  </tr>
                </thead>
                <tbody>
                  {colls.data.map((c) => (
                    <CollectionRow
                      key={c.name}
                      db={db.name}
                      c={c}
                      selected={sel.selected.has(`${db.name}.${c.name}`)}
                      onToggle={() => sel.toggle(`${db.name}.${c.name}`)}
                      droppable={!isSystem}
                    />
                  ))}
                </tbody>
              </Table>
            </div>
          )}
        </CardBody>
      )}

      <Modal
        open={dropOpen}
        title={`Drop database: ${db.name}`}
        onClose={() => { setDropOpen(false); setConfirmText(''); }}
        width="md"
        footer={
          <>
            <Button variant="ghost" onClick={() => { setDropOpen(false); setConfirmText(''); }}>Cancel</Button>
            <Button
              variant="danger"
              onClick={performDrop}
              disabled={confirmText !== db.name || dropDB.isPending}
              title={confirmText !== db.name ? 'Type the database name to enable' : ''}
            >
              {dropDB.isPending ? 'Dropping…' : 'Drop database'}
            </Button>
          </>
        }
      >
        <div className="space-y-3 text-sm">
          <div className="rounded border border-red-500 bg-red-50 p-2 text-red-900 dark:bg-red-900/40 dark:text-red-100">
            This permanently drops <code className="font-mono">{db.name}</code> and every collection inside it. Cannot be undone.
          </div>
          <div className="text-xs text-slate-500">
            data {formatBytes(db.data_size)} · storage {formatBytes(db.storage_size)} · {db.collections} collections
          </div>
          <div>
            <Label>Type the database name to confirm</Label>
            <Input value={confirmText} onChange={(e) => setConfirmText(e.target.value)} placeholder={db.name} />
          </div>
        </div>
      </Modal>
    </Card>
  );
}

function CollectionRow({ db, c, selected, onToggle, droppable }: { db: string; c: CollectionSummary; selected: boolean; onToggle: () => void; droppable: boolean }) {
  const dropColl = useDropCollection();
  const pushToast = useUI((s) => s.pushToast);
  const [open, setOpen] = useState(false);
  const [confirmText, setConfirmText] = useState('');

  const performDrop = async () => {
    try {
      await dropColl.mutateAsync({ database: db, collection: c.name });
      pushToast({ kind: 'success', title: 'Collection dropped', body: `${db}.${c.name}` });
      setOpen(false);
      setConfirmText('');
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Drop failed', body: msg });
    }
  };

  return (
    <tr className={cx(selected && 'bg-brand-500/5')}>
      <Td>
        <input
          type="checkbox"
          checked={selected}
          onChange={onToggle}
          aria-label={`select ${db}.${c.name}`}
          disabled={c.type !== 'regular'}
          title={c.type !== 'regular' ? 'Only regular collections can be loaded/deleted/compacted' : 'Select for cross-tab Operations Console actions'}
        />
      </Td>
      <Td className="font-mono">{c.name}</Td>
      <Td><Badge tone={c.type === 'regular' ? 'neutral' : 'warn'} title={collectionTypeHelp(c.type)}>{c.type}</Badge></Td>
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
      {droppable && (
        <Td className="text-right">
          <Button variant="danger" size="sm" onClick={() => setOpen(true)} title="Drop this collection">Drop</Button>
          <Modal
            open={open}
            title={`Drop ${db}.${c.name}`}
            onClose={() => { setOpen(false); setConfirmText(''); }}
            width="md"
            footer={
              <>
                <Button variant="ghost" onClick={() => { setOpen(false); setConfirmText(''); }}>Cancel</Button>
                <Button
                  variant="danger"
                  onClick={performDrop}
                  disabled={confirmText !== c.name || dropColl.isPending}
                >
                  {dropColl.isPending ? 'Dropping…' : 'Drop collection'}
                </Button>
              </>
            }
          >
            <div className="space-y-3 text-sm">
              <div className="rounded border border-red-500 bg-red-50 p-2 text-red-900 dark:bg-red-900/40 dark:text-red-100">
                Permanently drops <code className="font-mono">{db}.{c.name}</code>. Cannot be undone.
              </div>
              <div className="text-xs text-slate-500">
                {formatNumber(c.count)} docs · {formatBytes(c.storage_size)} on disk
              </div>
              <div>
                <Label>Type the collection name to confirm</Label>
                <Input value={confirmText} onChange={(e) => setConfirmText(e.target.value)} placeholder={c.name} />
              </div>
            </div>
          </Modal>
        </Td>
      )}
    </tr>
  );
}

function collectionTypeHelp(t: string): string {
  switch (t) {
    case 'regular': return 'Standard MongoDB collection. Eligible for load/delete/compact.';
    case 'capped': return 'Fixed-size, FIFO collection. compact is a no-op.';
    case 'timeseries': return 'Time-series collection. Different storage model; compact semantics differ.';
    case 'view': return 'A read-only view; not a real collection.';
    case 'clustered': return 'Clustered collection (5.3+). Storage layout differs from regular.';
    case 'system': return 'System collection (admin/config/local namespace).';
    default: return t;
  }
}
