<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { useI18n } from "vue-i18n";
import type {
  Inventory,
  PortalCustomer,
  PortalCustomerInput,
} from "@/api/services/customer-management";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Spinner } from "@/components/ui/spinner";

const props = defineProps<{
  customer?: PortalCustomer;
  inventory: Inventory;
  pending: boolean;
  error: string;
}>();
const open = defineModel<boolean>("open", { default: false });
const emit = defineEmits<{ save: [input: PortalCustomerInput] }>();
const { t } = useI18n({ useScope: "global" });
const label = (key: string) => t("customerManagement." + key);
const form = reactive<PortalCustomerInput>({
  id: "",
  username: "",
  password: "",
  nodes: [],
  groups: [],
  default_node: "",
});
const selectClass = "h-9 w-full rounded-md border bg-background px-2 text-sm";
watch(open, (value) => {
  if (!value) return;
  const customer = props.customer;
  Object.assign(form, {
    id: customer?.id ?? "",
    username: customer?.username ?? "",
    password: "",
    nodes: [...(customer?.nodes ?? [])],
    groups: [...(customer?.groups ?? [])],
    default_node: customer?.default_node ?? "",
  });
});
const targets = computed(
  () =>
    new Set([
      ...form.nodes,
      ...(props.inventory.groups ?? [])
        .filter((group) => form.groups.includes(group.id))
        .flatMap((group) => group.nodes),
    ]),
);
const missing = computed(() =>
  props.inventory.nodes.filter(
    (node) =>
      targets.value.has(node.id) &&
      !props.inventory.defaults?.some((item) => item.node === node.id),
  ),
);
function name(node: Inventory["nodes"][number]) {
  return node.id === "master" ? label("master") : node.name;
}
function submit() {
  emit("save", {
    ...form,
    username: form.username.trim(),
    nodes: [...form.nodes],
    groups: [...form.groups],
  });
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
      <SheetHeader>
        <SheetTitle>{{
          label(form.id ? "editCustomer" : "newCustomer")
        }}</SheetTitle>
        <SheetDescription>{{ label("grantHelp") }}</SheetDescription>
      </SheetHeader>
      <form class="grid gap-4 px-4" novalidate @submit.prevent="submit">
        <Alert v-if="error" variant="error" role="alert"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
        <label class="grid gap-1 text-sm"
          >{{ label("username") }}
          <Input
            v-model="form.username"
            :disabled="Boolean(form.id)"
            required
            minlength="2"
            maxlength="32"
          />
        </label>
        <label class="grid gap-1 text-sm"
          >{{ label("password") }}
          <Input
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            :required="!form.id"
            minlength="2"
            maxlength="32"
          />
          <span v-if="form.id" class="text-xs text-muted-foreground">{{
            label("passwordHint")
          }}</span>
        </label>
        <div class="grid gap-3 sm:grid-cols-2">
          <fieldset class="grid content-start gap-1 text-sm">
            <legend class="mb-1 font-medium">{{ label("direct") }}</legend>
            <label
              v-for="node in inventory.nodes"
              :key="node.id"
              class="flex items-center gap-2"
            >
              <input v-model="form.nodes" type="checkbox" :value="node.id" />{{
                name(node)
              }}
            </label>
          </fieldset>
          <fieldset class="grid content-start gap-1 text-sm">
            <legend class="mb-1 font-medium">{{ label("groups") }}</legend>
            <label
              v-for="group in inventory.groups"
              :key="group.id"
              class="flex items-center gap-2"
            >
              <input
                v-model="form.groups"
                type="checkbox"
                :value="group.id"
              />{{ group.name }}
            </label>
          </fieldset>
        </div>
        <Alert v-if="missing.length" role="status">
          <AlertDescription
            >{{ label("missingDefaults") }}
            {{ missing.map(name).join(", ") }}</AlertDescription
          >
        </Alert>
        <label class="grid gap-1 text-sm"
          >{{ label("defaultConnection") }}
          <select v-model="form.default_node" :class="selectClass">
            <option value="">{{ label("unassigned") }}</option>
            <option
              v-for="node in inventory.nodes.filter(
                (node) => targets.has(node.id) || node.id === form.default_node,
              )"
              :key="node.id"
              :value="node.id"
            >
              {{ name(node) }}
            </option>
          </select>
        </label>
        <SheetFooter class="px-0">
          <SheetClose as-child
            ><Button type="button" variant="outline" :disabled="pending">{{
              t("ocservUsers.cancel")
            }}</Button></SheetClose
          >
          <Button :disabled="pending"
            ><Spinner v-if="pending" data-icon="inline-start" />{{
              label("save")
            }}</Button
          >
        </SheetFooter>
      </form>
    </SheetContent>
  </Sheet>
</template>
