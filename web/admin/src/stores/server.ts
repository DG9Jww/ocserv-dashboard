import { defineStore } from "pinia";
import { computed, shallowRef } from "vue";

import { apiBaseUrl, setApiBaseUrl } from "@/api/http";
import {
  getAgentHealth,
  getOcservAgents,
  type AgentHealth,
  type OcservAgent,
} from "@/api/services/system";

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
  const statuses = shallowRef<Record<string, AgentHealth>>({});
  const statusRequestFailed = shallowRef(false);
  const selectableAgents = computed(() =>
    agents.value.filter((agent) => agentApiBaseUrl(agent) !== null),
  );
  const hasAgents = computed(() => selectableAgents.value.length > 0);
  let refreshPromise: Promise<void> | null = null;
  let statusPromise: Promise<void> | null = null;
  let statusTimer: ReturnType<typeof setTimeout> | undefined;

  function agentStatus(agent: OcservAgent): AgentHealth {
    if (agent.id == null) return "unknown";
    if (statusRequestFailed.value) return "unknown";
    return statuses.value[agent.id] ?? "checking";
  }

  function refreshStatus(): void {
    clearTimeout(statusTimer);
    statusTimer = undefined;
    if (statusPromise || !agents.value.length) return;
    let interval = 30_000;
    const request = getAgentHealth(agents.value)
      .then((snapshot) => {
        if (statusPromise !== request) return;
        statuses.value = snapshot.statuses;
        statusRequestFailed.value = false;
        if (snapshot.refreshing) interval = 3_000;
      })
      .catch(() => {
        if (statusPromise === request) statusRequestFailed.value = true;
      })
      .finally(() => {
        if (statusPromise !== request) return;
        statusPromise = null;
        statusTimer = setTimeout(refreshStatus, interval);
      });
    statusPromise = request;
  }

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

  function setAgents(nextAgents: OcservAgent[]): boolean {
    const previous = new Map(agents.value.map((agent) => [agent.id, agent]));
    const current = statuses.value;
    let changed = nextAgents.length !== agents.value.length;
    statuses.value = Object.fromEntries(
      nextAgents.flatMap((agent) => {
        const prior = previous.get(agent.id);
        if (
          !prior ||
          prior.updated_at !== agent.updated_at ||
          prior.address !== agent.address ||
          prior.port !== agent.port ||
          prior.address_type !== agent.address_type
        ) {
          changed = true;
          return [];
        }
        const status = agent.id == null ? undefined : current[agent.id];
        return status ? [[agent.id, status]] : [];
      }),
    );
    agents.value = nextAgents;
    if (
      selectedServer.value !== MASTER_SERVER &&
      !selectableAgents.value.some(
        (agent) => agentServerId(agent) === selectedServer.value,
      )
    )
      useMaster("unavailable");
    return changed;
  }

  function reset(): void {
    refreshPromise = null;
    statusPromise = null;
    clearTimeout(statusTimer);
    statusTimer = undefined;
    statuses.value = {};
    statusRequestFailed.value = false;
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
        if (refreshPromise !== request) return;
        const changed = setAgents(nextAgents);
        if (changed) statusPromise = null;
        if (changed || (!statusPromise && statusTimer === undefined))
          refreshStatus();
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
    agentStatus,
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
