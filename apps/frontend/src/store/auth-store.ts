import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import {
  authApi,
  getStoredToken,
  setStoredToken,
  clearStoredToken,
  registerAuthFailureHandler,
  ApiError,
} from "@/services/api";
import type {
  User,
  LoginCredentials,
  RegisterPayload,
  Role,
  AuthStatus,
} from "@/types/auth";

export interface AuthState {
  user: User | null;
  accessToken: string | null;
  isAuthenticated: boolean;
  status: AuthStatus;
  isLoading: boolean;
  isHydrated: boolean;
  sessionExpired: boolean;
  error: string | null;

  // Actions
  login: (credentials: LoginCredentials) => Promise<User>;
  register: (payload: RegisterPayload) => Promise<User>;
  logout: () => Promise<void>;
  refreshToken: () => Promise<string | null>;
  restoreSession: () => Promise<User | null>;
  setUser: (user: User | null) => void;
  setAccessToken: (token: string | null) => void;
  clearError: () => void;
  clearSessionExpired: () => void;
  hasRole: (allowedRoles: (Role | string)[]) => boolean;
  setHydrated: (hydrated: boolean) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      status: "idle",
      isLoading: false,
      isHydrated: false,
      sessionExpired: false,
      error: null,

      login: async (credentials: LoginCredentials) => {
        set({ isLoading: true, error: null, sessionExpired: false });
        try {
          const res = await authApi.login(credentials);
          set({
            user: res.user,
            accessToken: res.accessToken,
            isAuthenticated: true,
            status: "authenticated",
            isLoading: false,
            sessionExpired: false,
            error: null,
          });
          return res.user;
        } catch (err: unknown) {
          const message =
            err instanceof ApiError
              ? err.message
              : "Failed to log in. Please check your credentials.";
          set({
            isLoading: false,
            status: "unauthenticated",
            error: message,
          });
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
            status: "authenticated",
            isLoading: false,
            sessionExpired: false,
            error: null,
          });
          return loginRes.user;
        } catch (err: unknown) {
          const message =
            err instanceof ApiError ? err.message : "Registration failed. Please try again.";
          set({
            isLoading: false,
            error: message,
          });
          throw err;
        }
      },

      logout: async () => {
        try {
          await authApi.logout();
        } catch {
          // Ignore network errors on logout, client state must be cleared regardless
        } finally {
          clearStoredToken();
          set({
            user: null,
            accessToken: null,
            isAuthenticated: false,
            status: "unauthenticated",
            isLoading: false,
            error: null,
          });
        }
      },

      refreshToken: async () => {
        try {
          const res = await authApi.refresh();
          if (res?.accessToken) {
            set({ accessToken: res.accessToken, status: "authenticated" });
            return res.accessToken;
          }
          return null;
        } catch {
          // If refresh fails, clear auth state
          clearStoredToken();
          set({
            user: null,
            accessToken: null,
            isAuthenticated: false,
            status: "unauthenticated",
            sessionExpired: true,
          });
          return null;
        }
      },

      restoreSession: async () => {
        const storedToken = getStoredToken();
        const currentUser = get().user;

        // If no token exists at all and no user in state, mark unauthenticated
        if (!storedToken && !currentUser) {
          set({ status: "unauthenticated", isAuthenticated: false });
          return null;
        }

        set({ status: "restoring", isLoading: true });

        try {
          // Validate existing token or cookie session with backend
          const me = await authApi.getMe();
          set({
            user: me,
            isAuthenticated: true,
            status: "authenticated",
            isLoading: false,
            sessionExpired: false,
          });
          return me;
        } catch {
          // If access token was expired, try silent refresh
          try {
            const refreshRes = await authApi.refresh();
            if (refreshRes?.accessToken) {
              const me = await authApi.getMe();
              set({
                user: me,
                accessToken: refreshRes.accessToken,
                isAuthenticated: true,
                status: "authenticated",
                isLoading: false,
                sessionExpired: false,
              });
              return me;
            }
          } catch {
            // Refresh also failed - session is truly expired
          }

          clearStoredToken();
          set({
            user: null,
            accessToken: null,
            isAuthenticated: false,
            status: "unauthenticated",
            isLoading: false,
            sessionExpired: storedToken !== null, // Only set sessionExpired if a token previously existed
          });
          return null;
        }
      },

      setUser: (user) =>
        set({
          user,
          isAuthenticated: !!user,
          status: user ? "authenticated" : "unauthenticated",
        }),

      setAccessToken: (accessToken) => {
        setStoredToken(accessToken);
        set({ accessToken });
      },

      clearError: () => set({ error: null }),
      clearSessionExpired: () => set({ sessionExpired: false }),

      hasRole: (allowedRoles: (Role | string)[]) => {
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
        // Align in-memory token with persisted store on rehydration
        if (state?.accessToken) {
          setStoredToken(state.accessToken);
        }
      },
    }
  )
);

// Subscribe to global API auth failure events (e.g. background 401 refresh failures)
if (typeof window !== "undefined") {
  registerAuthFailureHandler(() => {
    useAuthStore.setState({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      status: "unauthenticated",
      sessionExpired: true,
    });
  });
}
