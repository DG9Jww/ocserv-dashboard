<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { AgentHealth } from "@/api/services/system";

defineProps<{ status: AgentHealth; compact?: boolean }>();
const { t } = useI18n({ useScope: "global" });
const colors: Record<AgentHealth, string> = {
  online: "bg-success",
  offline: "bg-destructive",
  unavailable: "bg-warning",
  checking: "bg-muted-foreground",
  unknown: "bg-muted-foreground",
};
</script>

<template>
  <span
    class="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap text-xs"
    :title="compact ? t('customerManagement.agentHealth.' + status) : undefined"
  >
    <span
      aria-hidden="true"
      class="size-2 shrink-0 rounded-full"
      :class="colors[status]"
    />
    <span :class="{ 'sr-only': compact }">{{
      t("customerManagement.agentHealth." + status)
    }}</span>
  </span>
</template>
