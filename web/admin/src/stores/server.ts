import { defineStore } from "pinia";
import { computed, shallowRef } from "vue";

import { apiBaseUrl, setApiBaseUrl } from "@/api/http";
import { getOcservAgents, type OcservAgent } from "@/api/services/system";

export const MASTER_SERVER = "master";
export type ServerError = "agentList" | "invalidAddress" | "unavailable";

export function agentServerId(agent: OcservAgent): string {
  return agent.id == null ? `address:${agent.address}` : String(agent.id);
}

export function agentApiBaseUrl(agent: OcservAgent): string | null {
  if (
    agent.id == null ||
    !Number.isSafeInteger(agent.id) ||
    agent.id < 1 ||
    !agent.address.trim()
  )
    return null;
  return `${apiBaseUrl}/ocserv/agents/${agent.id}/proxy`;
}

export const useServerStore = defineStore("server", () => {
  const agents = shallowRef<OcservAgent[]>([]);
  const selectedServer = shallowRef<string>(MASTER_SERVER);
  const error = shallowRef<ServerError | null>(null);
  const isLoading = shallowRef(false);
  const selectableAgents = computed(() =>
    agents.value.filter((agent) => agentApiBaseUrl(agent) !== null),
  );
  const hasAgents = computed(() => selectableAgents.value.length > 0);
  let refreshPromise: Promise<void> | null = null;

  function useMaster(nextError: ServerError | null = null): void {
    selectedServer.value = MASTER_SERVER;
    setApiBaseUrl(apiBaseUrl);
    error.value = nextError;
  }

  function selectServer(value: string): void {
    if (value === MASTER_SERVER) return useMaster();
    const agent = agents.value.find((item) => agentServerId(item) === value);
    if (!agent) return useMaster("unavailable");
    const baseUrl = agentApiBaseUrl(agent);
    if (!baseUrl) return useMaster("invalidAddress");
    if (selectedServer.value === value) return;
    selectedServer.value = value;
    setApiBaseUrl(baseUrl);
    error.value = null;
  }

  function setAgents(nextAgents: OcservAgent[]): void {
    agents.value = nextAgents;
    if (selectedServer.value === MASTER_SERVER) return;
    const selected = selectableAgents.value.find(
      (agent) => agentServerId(agent) === selectedServer.value,
    );
    if (!selected) return useMaster("unavailable");
  }

  function reset(): void {
    refreshPromise = null;
    agents.value = [];
    isLoading.value = false;
    useMaster();
  }

  function refreshAgents(): Promise<void> {
    if (refreshPromise) return refreshPromise;
    isLoading.value = true;
    error.value = null;
    const request = getOcservAgents()
      .then((nextAgents) => {
        if (refreshPromise === request) setAgents(nextAgents);
      })
      .catch(() => {
        if (refreshPromise !== request) return;
        error.value = "agentList";
        if (!agents.value.length) useMaster("agentList");
      })
      .finally(() => {
        if (refreshPromise !== request) return;
        isLoading.value = false;
        refreshPromise = null;
      });
    refreshPromise = request;
    return request;
  }

  function markUnavailable(): void {
    if (selectedServer.value !== MASTER_SERVER) error.value = "unavailable";
  }

  return {
    agents,
    error,
    hasAgents,
    isLoading,
    markUnavailable,
    refreshAgents,
    reset,
    selectableAgents,
    selectedServer,
    setAgents,
    selectServer,
  };
});
