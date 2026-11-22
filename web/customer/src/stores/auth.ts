import { computed, shallowRef } from "vue";
import { defineStore } from "pinia";

import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "@/api/auth-token";
import type { Customer, LoginData } from "@/api/generated";
import { normalizeApiError } from "@/api/http";
import { getSummary, login, type PortalSummary } from "@/api/services/customers";

export const useAuthStore = defineStore("auth", () => {
  const user = shallowRef<Customer | null>(null);
  const loading = shallowRef(false);
  const error = shallowRef("");
  let restoringSummary: Promise<PortalSummary> | null = null;
  let restoredSummary: PortalSummary | null = null;
  const isAuthenticated = computed(() =>
    Boolean(user.value && getAccessToken()),
  );
  function clearSession(): void {
    clearAccessToken();
    user.value = null;
    restoringSummary = null;
    restoredSummary = null;
  }
  async function signIn(credentials: LoginData): Promise<boolean> {
    loading.value = true;
    error.value = "";
    try {
      const response = await login(credentials);
      setAccessToken(response.token, response.expires_at);
      user.value = response.user;
      restoredSummary = null;
      return true;
    } catch (cause) {
      clearSession();
      error.value = normalizeApiError(cause).message;
      return false;
    } finally {
      loading.value = false;
    }
  }
  async function restoreSession(reuseSummaryOnHome: boolean): Promise<boolean> {
    const token = getAccessToken();
    if (!token) return false;

    const pending = (restoringSummary ??= getSummary());
    try {
      const summary = await pending;
      if (getAccessToken() !== token) return false;
      user.value = summary.ocserv_user ?? null;
      restoredSummary = reuseSummaryOnHome && user.value ? summary : null;
      return Boolean(user.value);
    } catch {
      if (getAccessToken() === token) clearSession();
      return false;
    } finally {
      if (restoringSummary === pending) restoringSummary = null;
    }
  }
  function takeRestoredSummary(): PortalSummary | null {
    const summary = restoredSummary;
    restoredSummary = null;
    return summary;
  }
  function signOut(): void {
    clearSession();
  }
  return {
    clearSession,
    error,
    isAuthenticated,
    loading,
    restoreSession,
    signIn,
    signOut,
    takeRestoredSummary,
    user,
  };
});
