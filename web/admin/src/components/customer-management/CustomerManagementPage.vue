<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { normalizeApiError } from "@/api/http";
import {
  loadCustomerManagement, savePortalCustomer, savePortalDefaults, savePortalGroup,
  trafficTypes, type Assignment, type Inventory, type NodeDefaults, type NodeGroup, type PortalCustomer,
} from "@/api/services/customer-management";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const { t } = useI18n({ useScope: "global" });
const label = (name: string) => t("customerManagement." + name);
const data = ref<Inventory | null>(null);
const pending = ref(false);
const error = ref("");
const saved = ref(false);
const group = ref<NodeGroup>({ id: "", name: "", nodes: [] });
const customer = ref<PortalCustomer & { password: string }>({
  id: "", username: "", password: "", nodes: [], groups: [], default_node: "",
});
const defaults = ref<NodeDefaults>({
  node: "", group: "defaults", traffic_type: "Free", traffic_size: 0, expiry_days: 0, certificate_group: "",
});
const selectClass = "h-9 w-full rounded-md border bg-background px-2 text-sm";
const nodeNames = computed(() => new Map(data.value?.nodes.map((node) => [node.id, node.name])));
const nodeName = (id: string) => id === "master" ? label("master") : nodeNames.value.get(id) ?? id;
const selectedNodes = computed(() => new Set([
  ...customer.value.nodes,
  ...(data.value?.groups ?? []).filter((g) => customer.value.groups.includes(g.id)).flatMap((g) => g.nodes),
]));
const certificates = computed(() => data.value?.groups ?? []);
const assignments = computed(() => {
  const byCustomer = new Map<string, Assignment[]>();
  for (const item of data.value?.assignments ?? []) {
    if (!byCustomer.has(item.customer_id)) byCustomer.set(item.customer_id, []);
    byCustomer.get(item.customer_id)?.push(item);
  }
  return byCustomer;
});
function status(a: Assignment) {
  if (a.error) return label(a.error === "customer_account_conflict" ? "conflict" : "failed");
  if (a.version === a.applied) return label(a.desired ? "active" : "inactive");
  return label(a.desired ? "pending" : "pendingDisable");
}
function editGroup(id: string) {
  const item = data.value?.groups?.find((g) => g.id === id);
  group.value = item ? { ...item, nodes: [...item.nodes] } : { id: "", name: "", nodes: [] };
}
function editCustomer(id: string) {
  const item = data.value?.customers?.find((c) => c.id === id);
  customer.value = item ? { ...item, password: "", nodes: [...item.nodes], groups: [...item.groups] } :
    { id: "", username: "", password: "", nodes: [], groups: [], default_node: "" };
}
function editDefaults(node: string) {
  const item = data.value?.defaults?.find((n) => n.node === node);
  defaults.value = item ? { ...item } : { node, group: "defaults", traffic_type: "Free", traffic_size: 0, expiry_days: 0, certificate_group: "" };
}
async function execute(task?: () => Promise<void>) {
  pending.value = true;
  error.value = "";
  saved.value = false;
  try {
    await task?.();
    data.value = await loadCustomerManagement();
    saved.value = Boolean(task);
  } catch (cause) {
    const code = normalizeApiError(cause).status;
    error.value = code === 404 ? "disabled" : code === 409 ? "conflict" : code === 400 ? "invalid" : "failed";
  } finally {
    pending.value = false;
  }
}
onMounted(() => execute());
</script>

<template>
  <div class="grid gap-5 p-4">
    <header class="flex flex-wrap items-center justify-between gap-2">
      <h1 class="text-xl font-semibold">{{ label("title") }}</h1>
      <Button variant="outline" :disabled="pending" @click="execute()">{{ label("refresh") }}</Button>
    </header>
    <p v-if="error" role="alert" class="text-sm text-destructive">{{ label(error) }}</p>
    <p v-else-if="saved" role="status" class="text-sm">{{ label("saved") }}</p>
    <p v-else-if="pending" role="status" class="text-sm">{{ label("loading") }}</p>
    <template v-if="data">
      <div class="grid gap-4 lg:grid-cols-2">
        <form class="grid content-start gap-3 rounded-lg border p-4" @submit.prevent="execute(() => savePortalGroup(group))">
          <h2 class="font-semibold">{{ label("groups") }}</h2>
          <label class="grid gap-1 text-sm">
            {{ label("group") }}
            <select :class="selectClass" :value="group.id" @change="editGroup(($event.target as HTMLSelectElement).value)">
              <option value="">{{ label("new") }}</option>
              <option v-for="item in data.groups" :key="item.id" :value="item.id">{{ item.name }}</option>
            </select>
          </label>
          <label class="grid gap-1 text-sm">{{ label("name") }}<Input v-model="group.name" required maxlength="128" /></label>
          <fieldset class="grid gap-1 text-sm">
            <legend>{{ label("nodes") }}</legend>
            <label v-for="node in data.nodes" :key="node.id" class="flex items-center gap-2">
              <input v-model="group.nodes" type="checkbox" :value="node.id" />{{ nodeName(node.id) }}
            </label>
          </fieldset>
          <Button class="justify-self-start" :disabled="pending">{{ label("save") }}</Button>
        </form>

        <form class="grid content-start gap-3 rounded-lg border p-4" @submit.prevent="execute(() => savePortalDefaults(defaults))">
          <h2 class="font-semibold">{{ label("defaults") }}</h2>
          <label class="grid gap-1 text-sm">{{ label("node") }}
            <select :class="selectClass" :value="defaults.node" required @change="editDefaults(($event.target as HTMLSelectElement).value)">
              <option value="">{{ label("select") }}</option>
              <option v-for="node in data.nodes" :key="node.id" :value="node.id">{{ nodeName(node.id) }}</option>
            </select>
          </label>
          <label class="grid gap-1 text-sm">{{ label("ocservGroup") }}<Input v-model="defaults.group" required maxlength="16" /></label>
          <label class="grid gap-1 text-sm">{{ label("certificateGroup") }}
            <select v-model="defaults.certificate_group" :class="selectClass">
              <option value="">{{ label("independent") }}</option>
              <option v-for="item in certificates" :key="item.id" :value="item.id">{{ item.name }}</option>
            </select>
          </label>
          <label class="grid gap-1 text-sm">{{ label("trafficPolicy") }}
            <select v-model="defaults.traffic_type" :class="selectClass">
              <option v-for="kind in trafficTypes" :key="kind" :value="kind">{{ t("ocservUsers.trafficTypes." + kind) }}</option>
            </select>
          </label>
          <label class="grid gap-1 text-sm">{{ label("trafficBytes") }}
            <input v-model.number="defaults.traffic_size" type="number" min="0" :class="selectClass" :disabled="defaults.traffic_type === 'Free'" required />
          </label>
          <label class="grid gap-1 text-sm">{{ label("expiryDays") }}
            <input v-model.number="defaults.expiry_days" type="number" min="0" max="36500" :class="selectClass" required />
          </label>
          <Button class="justify-self-start" :disabled="pending">{{ label("save") }}</Button>
        </form>
      </div>

      <form class="grid gap-3 rounded-lg border p-4" @submit.prevent="execute(async () => { await savePortalCustomer(customer); customer.password = ''; })">
        <h2 class="font-semibold">{{ label("customers") }}</h2>
        <label class="grid gap-1 text-sm">{{ label("customer") }}
          <select :class="selectClass" :value="customer.id" @change="editCustomer(($event.target as HTMLSelectElement).value)">
            <option value="">{{ label("new") }}</option>
            <option v-for="item in data.customers" :key="item.id" :value="item.id">{{ item.username }}</option>
          </select>
        </label>
        <label class="grid gap-1 text-sm">{{ label("username") }}<Input v-model="customer.username" :disabled="Boolean(customer.id)" required minlength="2" maxlength="32" /></label>
        <label class="grid gap-1 text-sm">{{ label("password") }}<Input v-model="customer.password" type="password" autocomplete="new-password" :required="!customer.id" minlength="2" maxlength="32" /></label>
        <div class="grid gap-3 sm:grid-cols-2">
          <fieldset class="grid gap-1 text-sm">
            <legend>{{ label("direct") }}</legend>
            <label v-for="node in data.nodes" :key="node.id" class="flex items-center gap-2">
              <input v-model="customer.nodes" type="checkbox" :value="node.id" />{{ nodeName(node.id) }}
            </label>
          </fieldset>
          <fieldset class="grid gap-1 text-sm">
            <legend>{{ label("groups") }}</legend>
            <label v-for="item in data.groups" :key="item.id" class="flex items-center gap-2">
              <input v-model="customer.groups" type="checkbox" :value="item.id" />{{ item.name }}
            </label>
          </fieldset>
        </div>
        <label class="grid gap-1 text-sm">{{ label("defaultConnection") }}
          <select v-model="customer.default_node" :class="selectClass">
            <option value="">{{ label("unassigned") }}</option>
            <option v-for="node in data.nodes.filter((n) => selectedNodes.has(n.id) || n.id === customer.default_node)" :key="node.id" :value="node.id">{{ nodeName(node.id) }}</option>
          </select>
        </label>
        <ul v-if="assignments.get(customer.id)?.length" class="grid gap-1 text-sm">
          <li v-for="row in assignments.get(customer.id)" :key="row.node">{{ nodeName(row.node) }} · {{ status(row) }}</li>
        </ul>
        <Button class="justify-self-start" :disabled="pending">{{ label("save") }}</Button>
      </form>
    </template>
  </div>
</template>
