<script setup lang="ts">
import { computed, onMounted, shallowRef, watch } from "vue";
import { Plus, RefreshCw, Search, X } from "@lucide/vue";
import { storeToRefs } from "pinia";
import { useI18n } from "vue-i18n";

import { normalizeApiError } from "@/api/http";
import {
  savePortalDefaults,
  savePortalGroup,
  type NodeGroup,
} from "@/api/services/customer-management";
import {
  createOcservAgent,
  deleteOcservAgent,
  updateOcservAgent,
  type OcservAgent,
  type OcservAgentCreate,
} from "@/api/services/system";
import AgentNodeEditor from "@/components/customer-management/AgentNodeEditor.vue";
import AgentNodesTable, {
  type AgentNodeRow,
} from "@/components/customer-management/AgentNodesTable.vue";
import PortalNodeEditor from "@/components/customer-management/PortalNodeEditor.vue";
import OcservUserActionDialog from "@/components/ocserv-users/OcservUserActionDialog.vue";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useCustomerManagement } from "@/composables/useCustomerManagement";
import { agentServerId, MASTER_SERVER, useServerStore } from "@/stores/server";

const { t } = useI18n({ useScope: "global" });
const serverStore = useServerStore();
const {
  agents,
  isLoading: agentsLoading,
  error: serverError,
} = storeToRefs(serverStore);
const { label, data, loading, loadError, nodeName, errorMessage, refresh } =
  useCustomerManagement();
const search = shallowRef("");
const role = shallowRef("all");
const groupFilter = shallowRef("all");
const page = shallowRef(1);
const pageSize = 20;

const agentEditorOpen = shallowRef(false);
const editingAgent = shallowRef<OcservAgent>();
const removeOpen = shallowRef(false);
const removingAgent = shallowRef<OcservAgent>();
const portalEditorOpen = shallowRef(false);
const portalMode = shallowRef<"group" | "defaults">("group");
const editingGroup = shallowRef<NodeGroup>();
const editingNode = shallowRef<AgentNodeRow>();
const pending = shallowRef(false);
const error = shallowRef("");
const success = shallowRef("");

const groups = computed(() => data.value?.groups ?? []);
const rows = computed<AgentNodeRow[]>(() => {
  const defaults = new Map(
    (data.value?.defaults ?? []).map((item) => [item.node, item]),
  );
  const groupsByNode = new Map<string, NodeGroup[]>();
  const groupNames = new Map(
    groups.value.map((group) => [group.id, group.name]),
  );
  for (const group of groups.value) {
    for (const id of group.nodes) {
      const memberships = groupsByNode.get(id) ?? [];
      memberships.push(group);
      groupsByNode.set(id, memberships);
    }
  }

  const nodes: Pick<AgentNodeRow, "id" | "name" | "agent">[] = [
    { id: MASTER_SERVER, name: nodeName(MASTER_SERVER) },
    ...agents.value.map((agent) => ({
      id: agent.id == null ? agentServerId(agent) : "agent-" + agent.id,
      name: agent.name,
      agent,
    })),
  ];
  return nodes.map((node) => {
    const settings = defaults.get(node.id);
    return {
      ...node,
      health: node.agent ? serverStore.agentStatus(node.agent) : undefined,
      defaults: settings,
      groups: groupsByNode.get(node.id) ?? [],
      certificateGroup: settings?.certificate_group
        ? groupNames.get(settings.certificate_group)
        : undefined,
    };
  });
});

const matching = computed(() => {
  const query = search.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    if (role.value !== "all" && role.value !== (row.agent ? "agent" : "master"))
      return false;
    if (groupFilter.value === "none" && row.groups.length) return false;
    if (
      groupFilter.value !== "all" &&
      groupFilter.value !== "none" &&
      !row.groups.some((group) => group.id === groupFilter.value)
    )
      return false;
    return (
      !query ||
      [
        row.name,
        row.agent?.address ?? "",
        row.agent ? String(row.agent.port ?? 8080) : "",
      ].some((value) => value.toLowerCase().includes(query))
    );
  });
});
const pages = computed(() =>
  Math.max(1, Math.ceil(matching.value.length / pageSize)),
);
const visible = computed(() =>
  matching.value.slice((page.value - 1) * pageSize, page.value * pageSize),
);
watch([search, role, groupFilter], () => {
  page.value = 1;
});
watch(pages, (count) => {
  page.value = Math.min(page.value, count);
});

function clearFilters() {
  search.value = "";
  role.value = "all";
  groupFilter.value = "all";
}
async function reload() {
  await Promise.all([serverStore.refreshAgents(), refresh()]);
}
function openAgent(agent?: OcservAgent) {
  editingAgent.value = agent;
  error.value = "";
  success.value = "";
  agentEditorOpen.value = true;
}
function confirmRemoval(agent: OcservAgent) {
  removingAgent.value = agent;
  error.value = "";
  success.value = "";
  removeOpen.value = true;
}
function openGroup(group?: NodeGroup) {
  editingGroup.value = group;
  portalMode.value = "group";
  error.value = "";
  success.value = "";
  portalEditorOpen.value = true;
}
function openDefaults(node: AgentNodeRow) {
  editingNode.value = node;
  portalMode.value = "defaults";
  error.value = "";
  success.value = "";
  portalEditorOpen.value = true;
}

async function saveAgent(input: OcservAgentCreate) {
  const id = editingAgent.value?.id;
  if (!input.name || !input.address || (id == null && !input.token)) {
    error.value = t("systemSettings.agentRequired");
    return;
  }
  if (
    input.port == null ||
    !Number.isInteger(input.port) ||
    input.port < 1 ||
    input.port > 65535
  ) {
    error.value = label("invalidAgentPort");
    return;
  }
  await mutate(
    async () => {
      if (id == null) await createOcservAgent(input);
      else await updateOcservAgent(id, input);
    },
    () => {
      agentEditorOpen.value = false;
    },
    "systemSettings.agentSaved",
    true,
  );
}
async function removeAgent() {
  const id = removingAgent.value?.id;
  if (id == null) return;
  await mutate(
    () => deleteOcservAgent(id),
    () => {
      removeOpen.value = false;
    },
    "systemSettings.agentDeleted",
    true,
  );
}
async function savePortal(task: () => Promise<void>) {
  await mutate(
    task,
    () => {
      portalEditorOpen.value = false;
    },
    "customerManagement.saved",
    false,
  );
}
async function mutate(
  task: () => Promise<unknown>,
  close: () => void,
  successKey: string,
  registry: boolean,
) {
  if (pending.value) return;
  pending.value = true;
  error.value = "";
  try {
    await task();
    close();
    success.value = t(successKey);
    if (registry) await reload();
    else await refresh();
  } catch (cause) {
    error.value = registry
      ? normalizeApiError(cause).message
      : errorMessage(cause);
  } finally {
    pending.value = false;
  }
}

onMounted(() => {
  void serverStore.refreshAgents();
});
</script>

<template>
  <div class="flex flex-col gap-6">
    <Alert v-if="serverError === 'agentList'" role="alert" variant="error">
      <AlertDescription>{{ label("agentListFailed") }}</AlertDescription>
    </Alert>
    <Alert v-if="loadError" role="status" :variant="data ? 'error' : 'info'">
      <AlertDescription>{{ loadError }}</AlertDescription>
    </Alert>
    <Alert v-if="success" role="status" variant="success">
      <AlertDescription>{{ success }}</AlertDescription>
    </Alert>

    <Card>
      <CardHeader>
        <CardTitle>{{ label("agentNodes") }}</CardTitle>
        <CardDescription>{{
          t("systemSettings.agentsDescription")
        }}</CardDescription>
        <CardAction class="flex flex-wrap gap-2">
          <Button
            variant="outline"
            :disabled="agentsLoading || loading || pending"
            @click="reload"
          >
            <RefreshCw data-icon="inline-start" />{{ label("refresh") }}
          </Button>
          <Button :disabled="agentsLoading || pending" @click="openAgent()">
            <Plus data-icon="inline-start" />{{ t("systemSettings.newAgent") }}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent class="grid gap-4">
        <div class="flex flex-wrap items-end gap-3">
          <Field class="w-full sm:max-w-80">
            <FieldLabel for="agent-node-search">{{
              label("searchNodes")
            }}</FieldLabel>
            <div class="relative">
              <Search
                class="pointer-events-none absolute start-3 top-2.5 size-4 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                id="agent-node-search"
                v-model="search"
                type="search"
                class="ps-9"
                :placeholder="label('searchNodesPlaceholder')"
              />
            </div>
          </Field>
          <Field class="w-40">
            <FieldLabel for="agent-node-role">{{
              label("nodeRole")
            }}</FieldLabel>
            <Select v-model="role">
              <SelectTrigger id="agent-node-role" class="w-full"
                ><SelectValue
              /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">{{ label("allNodes") }}</SelectItem>
                  <SelectItem value="agent">{{
                    label("agentNodes")
                  }}</SelectItem>
                  <SelectItem value="master">{{ label("master") }}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field v-if="data" class="w-48">
            <FieldLabel for="agent-node-group">{{
              label("groups")
            }}</FieldLabel>
            <Select v-model="groupFilter">
              <SelectTrigger id="agent-node-group" class="w-full"
                ><SelectValue
              /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">{{ label("allGroups") }}</SelectItem>
                  <SelectItem value="none">{{
                    label("ungroupedNodes")
                  }}</SelectItem>
                  <SelectItem
                    v-for="group in groups"
                    :key="group.id"
                    :value="group.id"
                    >{{ group.name }}</SelectItem
                  >
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Button
            type="button"
            variant="ghost"
            :disabled="!search && role === 'all' && groupFilter === 'all'"
            @click="clearFilters"
          >
            <X data-icon="inline-start" />{{ t("ocservUsers.clearFilters") }}
          </Button>
        </div>
        <AgentNodesTable
          :rows="visible"
          :loading="agentsLoading"
          :busy="pending"
          :has-inventory="Boolean(data)"
          @edit="openAgent"
          @remove="confirmRemoval"
          @defaults="openDefaults"
        />
      </CardContent>
      <CardFooter v-if="pages > 1" class="justify-end gap-2">
        <Button variant="outline" :disabled="page <= 1" @click="page--">{{
          label("previous")
        }}</Button>
        <span class="text-sm tabular-nums">{{ page }} / {{ pages }}</span>
        <Button variant="outline" :disabled="page >= pages" @click="page++">{{
          label("next")
        }}</Button>
      </CardFooter>
    </Card>

    <Card v-if="data">
      <CardHeader>
        <CardTitle>{{ label("groups") }}</CardTitle>
        <CardAction>
          <Button variant="outline" :disabled="pending" @click="openGroup()">
            <Plus data-icon="inline-start" />{{ label("newGroup") }}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div class="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ label("name") }}</TableHead>
                <TableHead>{{ label("nodes") }}</TableHead>
                <TableHead class="w-12"
                  ><span class="sr-only">{{
                    t("systemSettings.actions")
                  }}</span></TableHead
                >
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="group in groups" :key="group.id">
                <TableCell class="font-medium">{{ group.name }}</TableCell>
                <TableCell class="whitespace-normal">{{
                  group.nodes.map(nodeName).join(", ") || label("unassigned")
                }}</TableCell>
                <TableCell class="text-end">
                  <Button
                    variant="outline"
                    size="sm"
                    :disabled="pending"
                    @click="openGroup(group)"
                    >{{ t("ocservUsers.edit") }}</Button
                  >
                </TableCell>
              </TableRow>
              <TableRow v-if="!groups.length">
                <TableCell
                  colspan="3"
                  class="text-center text-muted-foreground"
                  >{{ label("noGroups") }}</TableCell
                >
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
    <AgentNodeEditor
      v-model:open="agentEditorOpen"
      :agent="editingAgent"
      :pending="pending"
      :error="error"
      @save="saveAgent"
    />
    <PortalNodeEditor
      v-if="data"
      v-model:open="portalEditorOpen"
      :mode="portalMode"
      :inventory="data"
      :group="editingGroup"
      :node="editingNode?.id"
      :defaults="editingNode?.defaults"
      :pending="pending"
      :error="error"
      @save-group="savePortal(() => savePortalGroup($event))"
      @save-defaults="savePortal(() => savePortalDefaults($event))"
    />
    <OcservUserActionDialog
      v-model:open="removeOpen"
      destructive
      :title="t('systemSettings.deleteAgentTitle')"
      :description="
        t('systemSettings.deleteAgentDescription', {
          name: removingAgent?.name,
        })
      "
      :action="t('systemSettings.delete')"
      :pending="pending"
      :error="error"
      @confirm="removeAgent"
    />
  </div>
</template>
