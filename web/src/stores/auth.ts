import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
  token: string | null;
  setToken: (t: string | null) => void;
}

// Persisted to localStorage so the token survives reloads. Stored under a
// single key so /api/v1/admin/log-level can be hit without re-auth.
export const useAuth = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      setToken: (t) => set({ token: t }),
    }),
    { name: 'mfpoc-auth' },
  ),
);
