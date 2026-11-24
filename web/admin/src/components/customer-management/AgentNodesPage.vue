<script setup lang="ts">
import { computed, onMounted, shallowRef, watch } from "vue";
import { Plus, RefreshCw, Search, X } from "@lucide/vue";
import { storeToRefs } from "pinia";
import { useI18n } from "vue-i18n";

import { normalizeApiError } from "@/api/http";
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
import { agentServerId, MASTER_SERVER, useServerStore } from "@/stores/server";

const { t } = useI18n({ useScope: "global" });
const serverStore = useServerStore();
const {
  agents,
  isLoading: agentsLoading,
  error: serverError,
} = storeToRefs(serverStore);
const label = (key: string) => t("customerManagement." + key);
const search = shallowRef("");
const role = shallowRef("all");
const page = shallowRef(1);
const pageSize = 20;

const agentEditorOpen = shallowRef(false);
const editingAgent = shallowRef<OcservAgent>();
const removeOpen = shallowRef(false);
const removingAgent = shallowRef<OcservAgent>();
const pending = shallowRef(false);
const error = shallowRef("");
const success = shallowRef("");

const rows = computed<AgentNodeRow[]>(() => {
  const nodes: Pick<AgentNodeRow, "id" | "name" | "agent">[] = [
    { id: MASTER_SERVER, name: label("master") },
    ...agents.value.map((agent) => ({
      id: agent.id == null ? agentServerId(agent) : "agent-" + agent.id,
      name: agent.name,
      agent,
    })),
  ];

  return nodes.map((node) => ({
    ...node,
    health: node.agent ? serverStore.agentStatus(node.agent) : undefined,
  }));
});

const matching = computed(() => {
  const query = search.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    if (role.value !== "all" && role.value !== (row.agent ? "agent" : "master"))
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
watch([search, role], () => {
  page.value = 1;
});
watch(pages, (count) => {
  page.value = Math.min(page.value, count);
});

function clearFilters() {
  search.value = "";
  role.value = "all";
}
async function reload() {
  await serverStore.refreshAgents();
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
  );
}
async function mutate(
  task: () => Promise<unknown>,
  close: () => void,
  successKey: string,
) {
  if (pending.value) return;
  pending.value = true;
  error.value = "";
  try {
    await task();
    close();
    success.value = t(successKey);
    await reload();
  } catch (cause) {
    error.value = normalizeApiError(cause).message;
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
            :disabled="agentsLoading || pending"
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
          <Button
            type="button"
            variant="ghost"
            :disabled="!search && role === 'all'"
            @click="clearFilters"
          >
            <X data-icon="inline-start" />{{ t("ocservUsers.clearFilters") }}
          </Button>
        </div>
        <AgentNodesTable
          :rows="visible"
          :loading="agentsLoading"
          :busy="pending"
          @edit="openAgent"
          @remove="confirmRemoval"
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

    <AgentNodeEditor
      v-model:open="agentEditorOpen"
      :agent="editingAgent"
      :pending="pending"
      :error="error"
      @save="saveAgent"
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
