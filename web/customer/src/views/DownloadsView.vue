<script setup lang="ts">
import { onMounted, shallowRef } from "vue";
import { useI18n } from "vue-i18n";
import type { CiscoSetup } from "@/api/generated";
import { normalizeApiError } from "@/api/http";
import { getCiscoSetup } from "@/api/services/customers";
import StatusMessage from "@/components/StatusMessage.vue";

const { t } = useI18n({ useScope: "global" });
const setup = shallowRef<CiscoSetup | null>(null);
const loading = shallowRef(true);
const error = shallowRef("");

async function load(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    setup.value = await getCiscoSetup();
  } catch (cause) {
    const failure = normalizeApiError(cause);
    error.value = failure.message === "customer_default_unavailable"
      ? t("portal.defaultUnavailable")
      : failure.message;
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section class="flex flex-col gap-4">
    <h2 class="text-xl font-semibold">{{ t("downloads.ciscoSetup") }}</h2>
    <StatusMessage :error="error" />
    <p v-if="loading">{{ t("common.loading") }}</p>
    <article v-else-if="setup" class="card flex flex-col gap-3">
      <dl class="grid grid-cols-2 gap-2 text-sm">
        <dt>{{ t("downloads.connectionName") }}</dt>
        <dd>{{ setup.connection_name }}</dd>
        <dt>{{ t("downloads.server") }}</dt>
        <dd>{{ setup.server_address }}:{{ setup.server_port }}</dd>
      </dl>
      <div>
        <a v-if="setup.connection_create_uri" class="button-secondary" :href="setup.connection_create_uri">
          {{ t("downloads.connect") }}
        </a>
      </div>
    </article>
    <div v-else-if="error">
      <button class="button-secondary" type="button" @click="load">{{ t("portal.retry") }}</button>
    </div>
  </section>
</template>
