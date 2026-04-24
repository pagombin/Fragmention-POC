import { useEffect } from 'react';
import { Route, Routes, useNavigate } from 'react-router-dom';
import { ClusterOverview } from './views/ClusterOverview';
import { DataBrowser } from './views/DataBrowser';
import { OperationsConsole } from './views/OperationsConsole';
import { RunsList } from './views/RunsList';
import { RunDetail } from './views/RunDetail';
import { NewRunWizard } from './views/NewRunWizard';
import { CompareRuns } from './views/CompareRuns';
import { InitialSyncCompanion } from './views/InitialSyncCompanion';
import { EventLog } from './views/EventLog';
import { Settings } from './views/Settings';
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
            <Route path="/data" element={<DataBrowser />} />
            <Route path="/ops" element={<OperationsConsole />} />
            <Route path="/runs" element={<RunsList />} />
            <Route path="/runs/:id" element={<RunDetail />} />
            <Route path="/wizard" element={<NewRunWizard />} />
            <Route path="/compare" element={<CompareRuns />} />
            <Route path="/initial-sync" element={<InitialSyncCompanion />} />
            <Route path="/events" element={<EventLog />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
        <LiveOpsPanel />
      </div>
      <Toasts />
    </div>
  );
}
