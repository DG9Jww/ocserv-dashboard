<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { CustomerPolicyGroup } from "@/api/services/customer-management";
import OcservUserActionDialog from "@/components/ocserv-users/OcservUserActionDialog.vue";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

const props = defineProps<{
  groups: CustomerPolicyGroup[];
  pending: boolean;
  error: string;
}>();
const open = defineModel<boolean>("open", { default: false });
const emit = defineEmits<{
  save: [group: CustomerPolicyGroup];
  remove: [id: string];
  select: [];
}>();
const { t } = useI18n({ useScope: "global" });
const label = (key: string) => t("customerManagement." + key);
const selected = ref("");
const confirmDelete = ref(false);
const routeText = ref("");
const form = reactive({
  name: "",
  primary_dns: "",
  secondary_dns: "",
  upload_kb_per_second: 0,
  download_kb_per_second: 0,
});
const current = computed(() =>
  props.groups.find((group) => group.id === selected.value),
);
const inputClass = "rounded-md border bg-background px-2 py-1.5 text-sm";

function reset() {
  const group = current.value;
  form.name = group?.name ?? "";
  form.primary_dns = group?.primary_dns ?? "";
  form.secondary_dns = group?.secondary_dns ?? "";
  form.upload_kb_per_second = group?.upload_kb_per_second ?? 0;
  form.download_kb_per_second = group?.download_kb_per_second ?? 0;
  routeText.value = group?.routes.join("\n") ?? "";
}

watch(open, () => {
  selected.value = "";
  confirmDelete.value = false;
  reset();
});
watch(selected, () => {
  reset();
  emit("select");
});

function submit() {
  emit("save", {
    ...form,
    id: selected.value,
    upload_kb_per_second: Number(form.upload_kb_per_second),
    download_kb_per_second: Number(form.download_kb_per_second),
    routes: routeText.value.split(/\s+/).filter(Boolean),
  });
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
      <SheetHeader>
        <SheetTitle>{{ label("policyGroups") }}</SheetTitle>
        <SheetDescription>{{ label("policyHelp") }}</SheetDescription>
      </SheetHeader>
      <form class="grid gap-4 px-4" @submit.prevent="submit">
        <Alert v-if="error && !confirmDelete" role="alert" variant="error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <label class="grid gap-1 text-sm">
          {{ label("policyGroup") }}
          <select v-model="selected" :class="inputClass" :disabled="pending">
            <option value="">{{ label("newPolicyGroup") }}</option>
            <option v-for="group in groups" :key="group.id" :value="group.id">
              {{ group.name }}
            </option>
          </select>
        </label>
        <label class="grid gap-1 text-sm">
          {{ label("name") }}
          <Input
            v-model="form.name"
            required
            maxlength="128"
            :disabled="pending"
          />
        </label>
        <label class="grid gap-1 text-sm">
          {{ label("primaryDNS") }}
          <Input v-model="form.primary_dns" dir="ltr" :disabled="pending" />
        </label>
        <label class="grid gap-1 text-sm">
          {{ label("secondaryDNS") }}
          <Input v-model="form.secondary_dns" dir="ltr" :disabled="pending" />
        </label>
        <label class="grid gap-1 text-sm">
          {{ label("policyRoutes") }}
          <textarea
            v-model="routeText"
            :class="inputClass"
            rows="3"
            dir="ltr"
            :disabled="pending"
          />
          <span class="text-xs text-muted-foreground">{{
            label("policyRoutesHint")
          }}</span>
        </label>
        <div class="grid grid-cols-2 gap-3">
          <label class="grid gap-1 text-sm">
            {{ label("uploadKBPerSecond") }}
            <input
              v-model.number="form.upload_kb_per_second"
              :class="inputClass"
              type="number"
              min="0"
              max="4294967295"
              :disabled="pending"
            />
          </label>
          <label class="grid gap-1 text-sm">
            {{ label("downloadKBPerSecond") }}
            <input
              v-model.number="form.download_kb_per_second"
              :class="inputClass"
              type="number"
              min="0"
              max="4294967295"
              :disabled="pending"
            />
          </label>
        </div>
        <SheetFooter class="px-0">
          <Button
            v-if="current"
            variant="destructive"
            type="button"
            :disabled="pending"
            @click="confirmDelete = true"
          >
            {{ t("ocservUsers.delete") }}
          </Button>
          <Button :disabled="pending">{{ label("save") }}</Button>
        </SheetFooter>
      </form>
    </SheetContent>
  </Sheet>
  <OcservUserActionDialog
    v-model:open="confirmDelete"
    :title="t('ocservUsers.delete')"
    :action="t('ocservUsers.delete')"
    :description="
      t('customerManagement.deletePolicyDescription', {
        name: current?.name ?? '',
      })
    "
    :error="error"
    :pending="pending"
    destructive
    @confirm="emit('remove', selected)"
  />
</template>
