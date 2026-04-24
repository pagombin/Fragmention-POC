import { create } from 'zustand';

// Shared multi-select state: a set of "database.collection" keys the user
// has highlighted in the Data Browser. Every Operations Console tab
// (Load / Delete / Compact / Workload / Snapshot) reads from this store so
// clicks flow naturally across views. Users who prefer to manage selection
// inside a single tab can override the set from the TargetSelector.
interface SelectionState {
  selected: Set<string>;
  toggle: (key: string) => void;
  set: (keys: string[]) => void;
  clear: () => void;
  hasAny: () => boolean;
  keys: () => string[];
  entries: () => { database: string; collection: string }[];
}

export const useSelection = create<SelectionState>((set, get) => ({
  selected: new Set(),
  toggle: (key) =>
    set((s) => {
      const next = new Set(s.selected);
      if (next.has(key)) next.delete(key); else next.add(key);
      return { selected: next };
    }),
  set: (keys) => set({ selected: new Set(keys) }),
  clear: () => set({ selected: new Set() }),
  hasAny: () => get().selected.size > 0,
  keys: () => [...get().selected],
  entries: () =>
    [...get().selected].map((k) => {
      const idx = k.indexOf('.');
      return { database: k.slice(0, idx), collection: k.slice(idx + 1) };
    }),
}));
