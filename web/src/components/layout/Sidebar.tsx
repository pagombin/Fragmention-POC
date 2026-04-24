import { NavLink } from 'react-router-dom';
import { cx } from '../ui/primitives';

// Navigation items mirror the spec § 5.7 view list.
const NAV = [
  { to: '/', label: 'Cluster Overview', key: 'c' },
  { to: '/data', label: 'Data Browser', key: 'd' },
  { to: '/ops', label: 'Operations Console', key: 'o' },
  { to: '/runs', label: 'Runs', key: 'r' },
  { to: '/wizard', label: 'New Run Wizard', key: 'n' },
  { to: '/compare', label: 'Compare Runs', key: 'm' },
  { to: '/initial-sync', label: 'Initial Sync Companion', key: 'i' },
  { to: '/events', label: 'Event Log', key: 'e' },
  { to: '/settings', label: 'Settings', key: 's' },
];

export function Sidebar() {
  return (
    <nav className="flex w-56 shrink-0 flex-col border-r border-slate-200 bg-white p-2 dark:border-slate-800 dark:bg-slate-950">
      <ul className="flex flex-col gap-0.5 text-sm">
        {NAV.map((item) => (
          <li key={item.to}>
            <NavLink
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                cx(
                  'block rounded px-2 py-1.5',
                  isActive
                    ? 'bg-brand-600/90 text-white'
                    : 'text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800',
                )
              }
            >
              {item.label}
            </NavLink>
          </li>
        ))}
      </ul>
      <div className="mt-auto text-[10px] text-slate-400 dark:text-slate-600 p-2">
        <kbd className="font-mono">g d</kbd>/<kbd className="font-mono">g o</kbd>/<kbd className="font-mono">g r</kbd>/<kbd className="font-mono">?</kbd>
      </div>
    </nav>
  );
}
