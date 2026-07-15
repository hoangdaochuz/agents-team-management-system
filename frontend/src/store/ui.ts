import { create } from "zustand";

interface UIState {
  sidebarOpen: boolean;
  toggleSidebar: () => void;
  closeSidebar: () => void;
  search: string;
  setSearch: (v: string) => void;
}

/** Client-only UI state (mobile sidebar drawer, global search field). */
export const useUI = create<UIState>((set) => ({
  sidebarOpen: false,
  toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
  closeSidebar: () => set({ sidebarOpen: false }),
  search: "",
  setSearch: (search) => set({ search }),
}));
