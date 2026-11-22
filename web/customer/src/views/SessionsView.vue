<script setup lang="ts">
import { onMounted, shallowRef } from "vue";
import { useI18n } from "vue-i18n";

import { normalizeApiError } from "@/api/http";
import {
  disconnectSessions,
  getSessions,
  terminateSessions,
  type PortalSession,
} from "@/api/services/customers";
import StatusMessage from "@/components/StatusMessage.vue";

const { t } = useI18n({ useScope: "global" });
const sessions = shallowRef<PortalSession[]>([]);
const loading = shallowRef(true);
const error = shallowRef("");
const partial = shallowRef(false);
const success = shallowRef("");
async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  partial.value = false;
  try {
    const result = await getSessions();
    sessions.value = result.data;
    partial.value = result.status.partial;
  } catch (cause) {
    sessions.value = [];
    const failure = normalizeApiError(cause);
    error.value = failure.status === 503 ? t("portal.unavailable")
      : failure.message === "invalid_customer_configuration" ? t("portal.invalid")
      : failure.message;
  } finally {
    loading.value = false;
  }
}
async function run(action: "disconnect" | "terminate"): Promise<void> {
  const prompt =
    action === "disconnect"
      ? "sessions.confirmDisconnect"
      : "sessions.confirmTerminate";
  if (!window.confirm(t(prompt))) return;
  loading.value = true;
  error.value = "";
  success.value = "";
  try {
    const actionPartial = await (action === "disconnect"
      ? disconnectSessions()
      : terminateSessions());
    success.value = t(actionPartial ? "portal.partialAction" : "common.success");
    await load();
  } catch (cause) {
    const failure = normalizeApiError(cause);
    error.value = failure.status === 503 ? t("portal.unavailable")
      : failure.message === "invalid_customer_configuration" ? t("portal.invalid")
      : failure.message;
    loading.value = false;
  }
}
onMounted(load);
</script>

<template>
  <section class="flex flex-col gap-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h1 class="text-2xl font-semibold">{{ t("sessions.title") }}</h1>
      <div class="flex gap-2">
        <button
          class="button-secondary"
          :disabled="loading || !sessions.length"
          @click="run('disconnect')"
        >
          {{ t("sessions.disconnect") }}
        </button>
        <button
          class="button"
          :disabled="loading || !sessions.length"
          @click="run('terminate')"
        >
          {{ t("sessions.terminate") }}
        </button>
      </div>
    </div>
    <StatusMessage :error="error" :success="success" />
    <p v-if="partial" role="status" class="text-sm">{{ t("portal.partial") }}</p>
    <p v-if="loading">{{ t("common.loading") }}</p>
    <p v-else-if="!sessions.length" class="card">{{ t("common.empty") }}</p>
    <div v-else class="card overflow-x-auto">
      <table class="table">
        <thead>
          <tr>
            <th>{{ t("sessions.device") }}</th>
            <th>{{ t("sessions.ip") }}</th>
            <th>{{ t("sessions.started") }}</th>
            <th>{{ t("sessions.receive") }}</th>
            <th>{{ t("sessions.transmit") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="session in sessions" :key="session.portal_key ?? session.ID">
            <td>{{ session.Device }}</td>
            <td>{{ session.IPv4 }}</td>
            <td>{{ session["Session started at"] }}</td>
            <td>{{ session["Average RX"] ?? "—" }}</td>
            <td>{{ session["Average TX"] ?? "—" }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
