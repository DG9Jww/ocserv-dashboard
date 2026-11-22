<script setup lang="ts">
import {
  MoreHorizontal,
  Network,
  Pencil,
  Settings2,
  Trash2,
} from "@lucide/vue";
import { useI18n } from "vue-i18n";

import type {
  NodeDefaults,
  NodeGroup,
} from "@/api/services/customer-management";
import type { AgentHealth, OcservAgent } from "@/api/services/system";
import AgentStatusIndicator from "@/components/AgentStatusIndicator.vue";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export type AgentNodeRow = {
  id: string;
  name: string;
  agent?: OcservAgent;
  health?: AgentHealth;
  defaults?: NodeDefaults;
  groups: NodeGroup[];
  certificateGroup?: string;
};

defineProps<{
  rows: AgentNodeRow[];
  loading: boolean;
  busy: boolean;
  hasInventory: boolean;
}>();
const emit = defineEmits<{
  edit: [agent: OcservAgent];
  remove: [agent: OcservAgent];
  defaults: [node: AgentNodeRow];
}>();
const { t } = useI18n({ useScope: "global" });
</script>

<template>
  <div class="rounded-md border">
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{{ t("systemSettings.agentName") }}</TableHead>
          <TableHead>{{ t("systemSettings.agentAddress") }}</TableHead>
          <TableHead>{{ t("customerManagement.apiPort") }}</TableHead>
          <TableHead>{{ t("customerManagement.status") }}</TableHead>
          <TableHead>{{ t("customerManagement.groups") }}</TableHead>
          <TableHead>{{ t("customerManagement.ocservGroup") }}</TableHead>
          <TableHead>{{ t("customerManagement.certificateGroup") }}</TableHead>
          <TableHead class="w-12"
            ><span class="sr-only">{{
              t("systemSettings.actions")
            }}</span></TableHead
          >
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-if="loading">
          <TableCell v-for="column in 8" :key="column"
            ><Skeleton class="h-8 w-full"
          /></TableCell>
        </TableRow>
        <TableRow v-for="row in loading ? [] : rows" :key="row.id">
          <TableCell class="font-medium">
            <div class="flex flex-wrap items-center gap-2">
              <Badge v-if="!row.agent" variant="secondary">{{
                row.name
              }}</Badge>
              <span v-else>{{ row.name }}</span>
            </div>
          </TableCell>
          <TableCell>
            <template v-if="row.agent">
              <bdi>{{ row.agent.address }}</bdi>
              <div class="text-xs text-muted-foreground">
                {{
                  t(
                    row.agent.address_type === "ip"
                      ? "systemSettings.ip"
                      : "systemSettings.domain",
                  )
                }}
              </div>
            </template>
            <span v-else class="text-muted-foreground">—</span>
          </TableCell>
          <TableCell class="tabular-nums">{{
            row.agent ? (row.agent.port ?? 8080) : "—"
          }}</TableCell>
          <TableCell>
            <AgentStatusIndicator v-if="row.health" :status="row.health" />
            <span v-else class="text-muted-foreground">—</span>
          </TableCell>
          <TableCell>
            <div v-if="row.groups.length" class="flex flex-wrap gap-1">
              <Badge
                v-for="group in row.groups"
                :key="group.id"
                variant="outline"
                >{{ group.name }}</Badge
              >
            </div>
            <span v-else class="text-muted-foreground">—</span>
          </TableCell>
          <TableCell>{{ row.defaults?.group ?? "—" }}</TableCell>
          <TableCell>{{
            row.defaults
              ? (row.certificateGroup ?? t("customerManagement.independent"))
              : "—"
          }}</TableCell>
          <TableCell>
            <DropdownMenu v-if="hasInventory || row.agent?.id != null">
              <DropdownMenuTrigger as-child>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  :aria-label="`${row.name} · ${t('systemSettings.actions')}`"
                  :disabled="busy"
                  ><MoreHorizontal
                /></Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuGroup>
                  <DropdownMenuItem
                    v-if="row.agent?.id != null"
                    @select="emit('edit', row.agent!)"
                  >
                    <Pencil />{{ t("systemSettings.edit") }}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    v-if="hasInventory"
                    @select="emit('defaults', row)"
                  >
                    <Settings2 />{{ t("customerManagement.editDefaults") }}
                  </DropdownMenuItem>
                </DropdownMenuGroup>
                <template v-if="row.agent?.id != null">
                  <DropdownMenuSeparator />
                  <DropdownMenuGroup>
                    <DropdownMenuItem
                      variant="destructive"
                      @select="emit('remove', row.agent!)"
                    >
                      <Trash2 />{{ t("systemSettings.delete") }}
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                </template>
              </DropdownMenuContent>
            </DropdownMenu>
          </TableCell>
        </TableRow>
        <TableRow v-if="!loading && !rows.length">
          <TableCell :colspan="8">
            <Empty
              ><EmptyHeader
                ><EmptyMedia variant="icon"><Network /></EmptyMedia
                ><EmptyTitle>{{
                  t("customerManagement.noMatchingNodes")
                }}</EmptyTitle></EmptyHeader
              ></Empty
            >
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>
