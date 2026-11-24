<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { normalizeApiError } from "@/api/http";
import {
  loadCustomerLoginProtection,
  saveCustomerLoginProtection,
  type CustomerLoginProtection,
} from "@/api/services/customer-management";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

const { t } = useI18n({ useScope: "global" });
const label = (key: string) => t("customerManagement." + key);
const policy = ref<CustomerLoginProtection>();
const loading = ref(true);
const saving = ref(false);
const error = ref("");
const saved = ref(false);
const fields = [
  { key: "max_failures", label: "loginMaxFailures", min: 2, max: 100 },
  { key: "window_minutes", label: "loginWindowMinutes", min: 1, max: 1440 },
  { key: "lock_minutes", label: "loginLockMinutes", min: 1, max: 1440 },
] as const;

onMounted(async () => {
  try {
    policy.value = await loadCustomerLoginProtection();
  } catch (cause) {
    error.value = label(
      normalizeApiError(cause).status === 404 ? "disabled" : "failed",
    );
  } finally {
    loading.value = false;
  }
});

async function save() {
  if (!policy.value || saving.value) return;
  saving.value = true;
  saved.value = false;
  error.value = "";
  try {
    await saveCustomerLoginProtection(policy.value);
    saved.value = true;
  } catch {
    error.value = label("failed");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>{{ label("loginProtection") }}</CardTitle>
      <CardDescription>{{ label("loginProtectionHelp") }}</CardDescription>
    </CardHeader>
    <CardContent class="grid gap-4">
      <Spinner v-if="loading" :aria-label="label('loading')" />
      <Alert v-if="error" role="alert" variant="error">
        <AlertDescription>{{ error }}</AlertDescription>
      </Alert>
      <Alert v-if="saved" role="status">
        <AlertDescription>{{ label("saved") }}</AlertDescription>
      </Alert>
      <form
        v-if="policy"
        class="grid gap-4"
        @submit.prevent="save"
        @input="saved = false"
      >
        <div class="grid gap-4 sm:grid-cols-3">
          <label
            v-for="field in fields"
            :key="field.key"
            class="grid gap-1 text-sm"
          >
            {{ label(field.label) }}
            <Input
              v-model.number="policy[field.key]"
              type="number"
              dir="ltr"
              :min="field.min"
              :max="field.max"
              :disabled="saving"
              required
            />
          </label>
        </div>
        <Button type="submit" class="justify-self-start" :disabled="saving">
          <Spinner v-if="saving" data-icon="inline-start" />
          {{ label("save") }}
        </Button>
      </form>
    </CardContent>
  </Card>
</template>
