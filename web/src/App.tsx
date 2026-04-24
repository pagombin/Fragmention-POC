import { useEffect } from 'react';
import { Route, Routes, useNavigate } from 'react-router-dom';
import { ClusterOverview } from './views/ClusterOverview';
import { Placeholder } from './views/Placeholder';
import { TopBar } from './components/layout/TopBar';
import { Sidebar } from './components/layout/Sidebar';
import { LiveOpsPanel } from './components/layout/LiveOpsPanel';
import { Toasts } from './components/ui/Toasts';
import { useUI } from './stores/ui';

export default function App() {
  const theme = useUI((s) => s.theme);
  const nav = useNavigate();

  // Apply theme on boot - Zustand persistence restores the value but doesn't
  // touch the DOM root class.
  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark');
  }, [theme]);

  // "g X" shortcut navigation. Not triggered inside inputs/textareas.
  useEffect(() => {
    let gPending = false;
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      if (e.key === '?') {
        alert('g c Cluster Overview · g d Data Browser · g o Ops Console · g r Runs · g n Wizard · g m Compare · g i Initial Sync · g e Events · g s Settings');
        return;
      }
      if (e.key === 'g') { gPending = true; setTimeout(() => (gPending = false), 1000); return; }
      if (gPending) {
        const map: Record<string, string> = {
          c: '/', d: '/data', o: '/ops', r: '/runs', n: '/wizard',
          m: '/compare', i: '/initial-sync', e: '/events', s: '/settings',
        };
        const target = map[e.key];
        if (target) nav(target);
        gPending = false;
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [nav]);

  return (
    <div className="flex h-screen flex-col">
      <TopBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <main className="flex min-w-0 flex-1 flex-col">
          <Routes>
            <Route path="/" element={<ClusterOverview />} />
            <Route path="/data" element={<Placeholder title="Data Browser" body="Coming in Phase 10." />} />
            <Route path="/ops" element={<Placeholder title="Operations Console" body="Coming in Phases 11–13." />} />
            <Route path="/runs" element={<Placeholder title="Runs" body="Coming in Phase 14." />} />
            <Route path="/runs/:id" element={<Placeholder title="Run Detail" body="Coming in Phase 14." />} />
            <Route path="/wizard" element={<Placeholder title="New Run Wizard" body="Coming in Phase 14." />} />
            <Route path="/compare" element={<Placeholder title="Compare Runs" body="Coming in Phase 14." />} />
            <Route path="/initial-sync" element={<Placeholder title="Initial Sync Companion" body="Coming in Phase 14." />} />
            <Route path="/events" element={<Placeholder title="Event Log" body="Coming in Phase 14." />} />
            <Route path="/settings" element={<Placeholder title="Settings" body="Coming in Phase 14." />} />
          </Routes>
        </main>
        <LiveOpsPanel />
      </div>
      <Toasts />
    </div>
  );
}
