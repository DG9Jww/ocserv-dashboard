<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
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
  grantsOnly?: boolean;
  inventory: Inventory;
  pending: boolean;
  error: string;
}>();
const open = defineModel<boolean>("open", { default: false });
const emit = defineEmits<{ save: [input: PortalCustomerInput] }>();
const { t } = useI18n({ useScope: "global" });
const label = (key: string, params: Record<string, string> = {}) =>
  t("customerManagement." + key, params);
const form = reactive<Omit<PortalCustomerInput, "nodes" | "groups">>({
  id: "",
  username: "",
  password: "",
  default_node: "",
});
const selectClass = "h-9 w-full rounded-md border bg-background px-2 text-sm";
const selection = ref("");
const legacyGrant = computed(() => {
  const customer = props.customer;
  if (!customer) return false;
  return (customer.nodes?.length ?? 0) + (customer.groups?.length ?? 0) !== 1;
});
const selectedNode = computed(() =>
  selection.value.startsWith("node:")
    ? props.inventory.nodes.find((node) => "node:" + node.id === selection.value)
    : undefined,
);
const selectedGroup = computed(() =>
  selection.value.startsWith("group:")
    ? props.inventory.groups?.find(
        (group) => "group:" + group.id === selection.value,
      )
    : undefined,
);
const targets = computed(() =>
  new Set(
    selectedNode.value ? [selectedNode.value.id] : selectedGroup.value?.nodes,
  ),
);
watch(open, (value) => {
  if (!value) return;
  const customer = props.customer;
  const node = customer?.nodes?.[0];
  const group = customer?.groups?.[0];
  const defaultNode = customer?.default_node ?? "";
  selection.value = "";
  if (!legacyGrant.value) {
    if (node) selection.value = "node:" + node;
    else if (group) selection.value = "group:" + group;
  }
  Object.assign(form, {
    id: customer?.id ?? "",
    username: customer?.username ?? "",
    password: "",
    default_node: targets.value.has(defaultNode) ? defaultNode : "",
  });
});
watch(targets, (nodes) => {
  if (!nodes.has(form.default_node)) form.default_node = "";
});
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
  if (!selectedNode.value && !selectedGroup.value) return;
  emit("save", {
    ...form,
    username: form.username.trim(),
    nodes: selectedNode.value ? [selectedNode.value.id] : [],
    groups: selectedGroup.value ? [selectedGroup.value.id] : [],
    default_node: targets.value.has(form.default_node) ? form.default_node : "",
  });
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
      <SheetHeader>
        <SheetTitle>{{
          label(
            grantsOnly
              ? "manageGrants"
              : form.id
                ? "editCustomer"
                : "newCustomer",
          )
        }}</SheetTitle>
        <SheetDescription>
          {{
            grantsOnly
              ? label("changeGrantHelp", { username: form.username })
              : label("grantHelp")
          }}
        </SheetDescription>
      </SheetHeader>
      <form class="grid gap-4 px-4" novalidate @submit.prevent="submit">
        <Alert v-if="error" variant="error" role="alert"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
        <label v-if="!grantsOnly" class="grid gap-1 text-sm"
          >{{ label("username") }}
          <Input
            v-model="form.username"
            :disabled="Boolean(form.id)"
            required
            minlength="2"
            maxlength="32"
          />
        </label>
        <label v-if="!grantsOnly" class="grid gap-1 text-sm"
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
        <p v-if="legacyGrant && !selection" class="text-sm text-muted-foreground">
          {{ label("legacyGrantHelp") }}
        </p>
        <label class="grid gap-1 text-sm">
          {{ label("grantSelection") }}
          <select v-model="selection" :class="selectClass" required>
            <option value="" disabled>{{ label("grantRequired") }}</option>
            <optgroup :label="label('nodes')">
              <option
                v-for="node in inventory.nodes"
                :key="node.id"
                :value="'node:' + node.id"
              >
                {{ name(node) }}
              </option>
            </optgroup>
            <optgroup v-if="inventory.groups?.length" :label="label('groups')">
              <option
                v-for="group in inventory.groups"
                :key="group.id"
                :value="'group:' + group.id"
              >
                {{ group.name }}
              </option>
            </optgroup>
          </select>
        </label>
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
                (node) => targets.has(node.id),
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
          <Button :disabled="pending || (!selectedNode && !selectedGroup)"
            ><Spinner v-if="pending" data-icon="inline-start" />{{
              label("save")
            }}</Button
          >
        </SheetFooter>
      </form>
    </SheetContent>
  </Sheet>
</template>
