import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import {
  authApi,
  type User,
  type LoginCredentials,
  type RegisterPayload,
  ApiError,
} from "@/services/api";
import { STORAGE_KEYS } from "@/lib/constants";

export interface AuthState {
  user: User | null;
  accessToken: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  isHydrated: boolean;
  error: string | null;

  // Actions
  login: (credentials: LoginCredentials) => Promise<User>;
  register: (payload: RegisterPayload) => Promise<User>;
  logout: () => void;
  refreshToken: () => Promise<string | null>;
  setUser: (user: User | null) => void;
  setAccessToken: (token: string | null) => void;
  clearError: () => void;
  hasRole: (allowedRoles: string[]) => boolean;
  setHydrated: (hydrated: boolean) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      isLoading: false,
      isHydrated: false,
      error: null,

      login: async (credentials: LoginCredentials) => {
        set({ isLoading: true, error: null });
        try {
          const res = await authApi.login(credentials);
          set({
            user: res.user,
            accessToken: res.accessToken,
            isAuthenticated: true,
            isLoading: false,
            error: null,
          });
          return res.user;
        } catch (err: unknown) {
          const message =
            err instanceof ApiError
              ? err.message
              : "Failed to log in. Please check your credentials.";
          set({ isLoading: false, error: message });
          throw err;
        }
      },

      register: async (payload: RegisterPayload) => {
        set({ isLoading: true, error: null });
        try {
          await authApi.register(payload);
          // Auto login after registration
          const loginRes = await authApi.login({
            email: payload.email,
            password: payload.password,
          });
          set({
            user: loginRes.user,
            accessToken: loginRes.accessToken,
            isAuthenticated: true,
            isLoading: false,
            error: null,
          });
          return loginRes.user;
        } catch (err: unknown) {
          const message =
            err instanceof ApiError ? err.message : "Registration failed. Please try again.";
          set({ isLoading: false, error: message });
          throw err;
        }
      },

      logout: () => {
        authApi.logout();
        set({
          user: null,
          accessToken: null,
          isAuthenticated: false,
          isLoading: false,
          error: null,
        });
      },

      refreshToken: async () => {
        try {
          const res = await authApi.refresh();
          if (res?.accessToken) {
            set({ accessToken: res.accessToken });
            return res.accessToken;
          }
          return null;
        } catch {
          // If refresh fails, clear auth state
          get().logout();
          return null;
        }
      },

      setUser: (user) => set({ user, isAuthenticated: !!user }),
      setAccessToken: (accessToken) => set({ accessToken }),
      clearError: () => set({ error: null }),

      hasRole: (allowedRoles: string[]) => {
        const { user } = get();
        if (!user) return false;
        return allowedRoles.includes(user.role);
      },

      setHydrated: (isHydrated) => set({ isHydrated }),
    }),
    {
      name: "ai-scribe-auth-storage",
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({
        user: state.user,
        accessToken: state.accessToken,
        isAuthenticated: state.isAuthenticated,
      }),
      onRehydrateStorage: () => (state) => {
        state?.setHydrated(true);
      },
    }
  )
);
