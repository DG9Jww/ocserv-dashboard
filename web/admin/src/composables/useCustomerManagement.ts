import { computed, onMounted, onUnmounted, ref, shallowRef } from "vue";
import { useI18n } from "vue-i18n";
import {
  loadCustomerManagement,
  type Inventory,
} from "@/api/services/customer-management";
import { normalizeApiError } from "@/api/http";

const messageKeys: Record<string, string> = {
  customer_invalid_username: "invalidUsername",
  customer_invalid_password: "invalidPassword",
  customer_grant_required: "grantRequired",
  customer_node_defaults_missing: "missingDefaults",
  invalid_customer_configuration: "invalid",
  customer_account_conflict: "conflict",
  customer_node_setup_required: "nodeSetup",
  customer_ocserv_group_missing: "localGroupMissing",
  customer_node_auth_failed: "nodeAuth",
  customer_node_tls_failed: "nodeTLS",
  customer_nodes_unavailable: "nodeUnavailable",
};

export function useCustomerManagement(poll = false) {
  const { t } = useI18n({ useScope: "global" });
  const label = (key: string) => t("customerManagement." + key);
  const data = shallowRef<Inventory | null>(null);
  const loading = ref(false);
  const loadError = ref("");
  const nodeNames = computed(
    () => new Map(data.value?.nodes.map((node) => [node.id, node.name])),
  );
  const nodeName = (id: string) =>
    id === "master" ? label("master") : (nodeNames.value.get(id) ?? id);
  function describe(code: string) {
    return label(messageKeys[code] ?? "failed");
  }
  function errorMessage(cause: unknown, loadingInventory = false) {
    const error = normalizeApiError(cause);
    if (loadingInventory && error.status === 404) return label("disabled");
    if (error.status === 403) return label("permission");
    const response = error.data as { message?: unknown } | undefined;
    const message = response?.message;
    const code = Array.isArray(message) ? message[0] : message;
    return describe(typeof code === "string" ? code : error.message);
  }
  let inFlight: Promise<void> | undefined;
  async function refresh(silent = false) {
    if (inFlight) {
      if (silent) return;
      await inFlight;
    }
    inFlight = load(silent);
    await inFlight;
  }
  async function load(silent: boolean) {
    if (!silent) {
      loading.value = true;
      loadError.value = "";
    }
    try {
      data.value = await loadCustomerManagement();
      loadError.value = "";
    } catch (cause) {
      loadError.value = errorMessage(cause, true);
    } finally {
      loading.value = false;
      inFlight = undefined;
    }
  }
  let timer: ReturnType<typeof setInterval> | undefined;
  onMounted(() => {
    void refresh();
    if (poll)
      timer = setInterval(() => {
        if (data.value?.assignments?.some((row) => row.version !== row.applied))
          void refresh(true);
      }, 3000);
  });
  onUnmounted(() => clearInterval(timer));
  return {
    label,
    data,
    loading,
    loadError,
    nodeName,
    describe,
    errorMessage,
    refresh,
  };
}
