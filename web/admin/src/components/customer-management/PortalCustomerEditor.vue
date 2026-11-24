<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  trafficTypes,
  type Inventory,
  type PortalCustomer,
  type PortalCustomerInput,
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
const form = reactive({
  id: "",
  username: "",
  password: "",
  default_node: "",
  traffic_type: "Free" as NonNullable<PortalCustomerInput["traffic_type"]>,
  traffic_size: 0,
  expiresAt: "",
});
const selectClass = "h-9 w-full rounded-md border bg-background px-2 text-sm";
const selection = ref("");
const selectedNode = computed(() =>
  props.inventory.nodes.find((node) => "node:" + node.id === selection.value),
);
const selectedPolicy = computed(() =>
  props.inventory.policy_groups?.find(
    (policy) => "policy:" + policy.id === selection.value,
  ),
);
const targets = computed(() =>
  props.inventory.nodes.filter(
    (node) =>
      node.id === selectedNode.value?.id ||
      selectedPolicy.value?.nodes.includes(node.id),
  ),
);

watch(open, (value) => {
  if (!value) return;
  const customer = props.customer;
  selection.value = customer?.policy_group_id
    ? "policy:" + customer.policy_group_id
    : customer?.node
      ? "node:" + customer.node
      : "";
  const expiresAt = customer?.expires_at ? new Date(customer.expires_at) : null;
  Object.assign(form, {
    id: customer?.id ?? "",
    username: customer?.username ?? "",
    password: "",
    default_node: customer?.default_node ?? "",
    traffic_type: customer?.traffic_type ?? "Free",
    traffic_size: customer?.traffic_size ?? 0,
    expiresAt: expiresAt
      ? new Date(expiresAt.getTime() - expiresAt.getTimezoneOffset() * 60_000)
          .toISOString()
          .slice(0, 23)
      : "",
  });
});
watch(targets, (nodes) => {
  if (!nodes.some((node) => node.id === form.default_node))
    form.default_node = "";
});
function name(node: Inventory["nodes"][number]) {
  return node.id === "master" ? label("master") : node.name;
}
function submit() {
  if (!selectedNode.value && !selectedPolicy.value) return;
  emit("save", {
    id: form.id,
    username: form.username.trim(),
    password: form.password,
    node: selectedNode.value?.id ?? "",
    policy_group_id: selectedPolicy.value?.id ?? null,
    ...(props.grantsOnly
      ? {}
      : {
          traffic_type: form.traffic_type,
          traffic_size: form.traffic_type === "Free" ? 0 : form.traffic_size,
          expires_at: form.expiresAt
            ? new Date(form.expiresAt).toISOString()
            : null,
        }),
    default_node: targets.value.some((node) => node.id === form.default_node)
      ? form.default_node
      : "",
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
      <form class="grid gap-4 px-4" @submit.prevent="submit">
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
            <optgroup
              v-if="inventory.policy_groups?.length"
              :label="label('radiusPolicies')"
            >
              <option
                v-for="policy in inventory.policy_groups"
                :key="policy.id"
                :value="'policy:' + policy.id"
              >
                {{ policy.name }}
              </option>
            </optgroup>
          </select>
        </label>
        <template v-if="!grantsOnly">
          <label class="grid gap-1 text-sm">
            {{ label("trafficPolicy") }}
            <select v-model="form.traffic_type" :class="selectClass">
              <option v-for="kind in trafficTypes" :key="kind" :value="kind">
                {{ label(kind) }}
              </option>
            </select>
          </label>
          <label v-if="form.traffic_type !== 'Free'" class="grid gap-1 text-sm">
            {{ label("trafficBytes") }}
            <input
              v-model.number="form.traffic_size"
              type="number"
              dir="ltr"
              min="0"
              max="9007199254740991"
              :class="selectClass"
              required
            />
            <span class="text-xs text-muted-foreground">{{
              label("sharedQuotaHelp")
            }}</span>
          </label>
          <label class="grid gap-1 text-sm">
            {{ label("expiresAt") }}
            <input
              v-model="form.expiresAt"
              type="datetime-local"
              dir="ltr"
              step="0.001"
              :class="selectClass"
            />
            <span class="text-xs text-muted-foreground">{{
              label("expiresAtHelp")
            }}</span>
          </label>
        </template>
        <label class="grid gap-1 text-sm"
          >{{ label("defaultConnection") }}
          <select v-model="form.default_node" :class="selectClass">
            <option value="">{{ label("unassigned") }}</option>
            <option v-for="node in targets" :key="node.id" :value="node.id">
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
          <Button :disabled="pending || (!selectedNode && !selectedPolicy)"
            ><Spinner v-if="pending" data-icon="inline-start" />{{
              label("save")
            }}</Button
          >
        </SheetFooter>
      </form>
    </SheetContent>
  </Sheet>
</template>
