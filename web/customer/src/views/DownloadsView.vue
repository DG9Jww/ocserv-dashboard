<script setup lang="ts">
import { onMounted, shallowRef, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { CiscoSetup } from "@/api/generated";
import { normalizeApiError } from "@/api/http";
import {
  downloadCiscoCertificate,
  getCiscoSetup,
} from "@/api/services/customers";
import StatusMessage from "@/components/StatusMessage.vue";

const props = defineProps<{ syncPending: boolean }>();

const { t } = useI18n({ useScope: "global" });
const setup = shallowRef<CiscoSetup | null>(null);
const loading = shallowRef(true);
const downloading = shallowRef(false);
const error = shallowRef("");
const unavailable = shallowRef(false);
async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  unavailable.value = false;
  setup.value = null;
  try {
    setup.value = await getCiscoSetup();
  } catch (cause) {
    const failure = normalizeApiError(cause);
    unavailable.value = failure.message === "customer_default_unavailable" || failure.message === "customer_sync_pending";
    error.value = unavailable.value
      ? t(failure.message === "customer_sync_pending" ? "portal.pending" : "portal.defaultUnavailable")
      : failure.message;
  } finally {
    loading.value = false;
  }
}
async function saveCertificate(): Promise<void> {
  downloading.value = true;
  error.value = "";
  try {
    const url = URL.createObjectURL(await downloadCiscoCertificate());
    const link = document.createElement("a");
    link.href = url;
    link.download = "cisco-setup-certificate.p12";
    link.click();
    URL.revokeObjectURL(url);
  } catch (cause) {
    const failure = normalizeApiError(cause);
    error.value = failure.status === 409
      ? t(props.syncPending ? "portal.pending" : "portal.defaultUnavailable")
      : failure.message;
  } finally {
    downloading.value = false;
  }
}
onMounted(load);
watch(() => props.syncPending, (pending) => { if (!pending) void load(); });
</script>

<template>
  <section class="flex flex-col gap-4">
    <h2 class="text-xl font-semibold">{{ t("downloads.title") }}</h2>
    <StatusMessage :error="error" />
    <p v-if="loading">{{ t("common.loading") }}</p>
    <article v-else class="card flex flex-col gap-3">
      <h2 class="font-semibold">{{ t("downloads.ciscoCertificate") }}</h2>
      <dl v-if="setup" class="grid grid-cols-2 gap-2 text-sm">
        <dt>{{ t("downloads.connectionName") }}</dt>
        <dd>{{ setup.connection_name }}</dd>
        <dt>{{ t("downloads.server") }}</dt>
        <dd>{{ setup.server_address }}:{{ setup.server_port }}</dd>
        <dt>{{ t("downloads.certificatePassword") }}</dt>
        <dd>{{ setup.certificate_password }}</dd>
      </dl>
      <div class="flex flex-wrap gap-2">
        <button class="button" @click="saveCertificate" :disabled="unavailable || downloading">
          {{ t(downloading ? "common.loading" : "common.download") }}
        </button>
        <a
          v-if="!unavailable && setup?.certificate_import_uri"
          class="button-secondary"
          :href="setup.certificate_import_uri"
          >{{ t("downloads.import") }}</a
        >
        <a
          v-if="!unavailable && setup?.connection_create_uri"
          class="button-secondary"
          :href="setup.connection_create_uri"
          >{{ t("downloads.connect") }}</a
        >
      </div>
      <p v-if="unavailable && !syncPending"><button class="button-secondary" type="button" @click="load">{{ t("portal.retry") }}</button></p>
    </article>
  </section>
</template>
