import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type Theme = 'dark' | 'light';

interface UIState {
  theme: Theme;
  liveOpsCollapsed: boolean;
  setTheme: (t: Theme) => void;
  toggleTheme: () => void;
  setLiveOpsCollapsed: (c: boolean) => void;
  toasts: Toast[];
  pushToast: (t: Omit<Toast, 'id'>) => void;
  dismissToast: (id: number) => void;
}

export interface Toast {
  id: number;
  kind: 'info' | 'success' | 'warn' | 'error';
  title: string;
  body?: string;
}

let toastSeq = 1;

export const useUI = create<UIState>()(
  persist(
    (set) => ({
      theme: 'dark',
      liveOpsCollapsed: false,
      toasts: [],
      setTheme: (theme) => {
        set({ theme });
        document.documentElement.classList.toggle('dark', theme === 'dark');
      },
      toggleTheme: () =>
        set((s) => {
          const next = s.theme === 'dark' ? 'light' : 'dark';
          document.documentElement.classList.toggle('dark', next === 'dark');
          return { theme: next };
        }),
      setLiveOpsCollapsed: (c) => set({ liveOpsCollapsed: c }),
      pushToast: (t) =>
        set((s) => ({ toasts: [...s.toasts, { id: toastSeq++, ...t }] })),
      dismissToast: (id) =>
        set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) })),
    }),
    { name: 'mfpoc-ui', partialize: (s) => ({ theme: s.theme, liveOpsCollapsed: s.liveOpsCollapsed }) },
  ),
);
