<script setup lang="ts">
import { computed, ref } from "vue";
import { Plus, RefreshCw } from "@lucide/vue";
import { useI18n } from "vue-i18n";
import {
  savePortalDefaults,
  savePortalGroup,
  type NodeGroup,
} from "@/api/services/customer-management";
import PortalNodeEditor from "@/components/customer-management/PortalNodeEditor.vue";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useCustomerManagement } from "@/composables/useCustomerManagement";

const { t } = useI18n({ useScope: "global" });
const { label, data, loading, loadError, nodeName, errorMessage, refresh } =
  useCustomerManagement();
const editing = ref(false);
const mode = ref<"group" | "defaults">("group");
const group = ref<NodeGroup>();
const node = ref("");
const selectedDefaults = computed(() =>
  data.value?.defaults?.find((item) => item.node === node.value),
);
const defaultsByNode = computed(
  () => new Map((data.value?.defaults ?? []).map((item) => [item.node, item])),
);
const pending = ref(false);
const error = ref("");
const saved = ref(false);
function editGroup(item?: NodeGroup) {
  mode.value = "group";
  group.value = item;
  error.value = "";
  saved.value = false;
  editing.value = true;
}
function editDefaults(id: string) {
  mode.value = "defaults";
  node.value = id;
  error.value = "";
  saved.value = false;
  editing.value = true;
}
async function save(task: () => Promise<void>) {
  pending.value = true;
  error.value = "";
  try {
    await task();
    editing.value = false;
    saved.value = true;
    await refresh();
  } catch (cause) {
    error.value = errorMessage(cause);
  } finally {
    pending.value = false;
  }
}
</script>

<template>
  <div class="grid gap-4 p-4">
    <Alert v-if="loadError" role="alert" variant="error"
      ><AlertDescription>{{ loadError }}</AlertDescription></Alert
    >
    <Alert v-if="saved" role="status"
      ><AlertDescription>{{ label("saved") }}</AlertDescription></Alert
    >
    <Card>
      <CardHeader>
        <CardTitle>{{ label("groups") }}</CardTitle>
        <CardAction class="flex flex-wrap gap-2">
          <Button variant="outline" :disabled="loading" @click="refresh()"
            ><RefreshCw data-icon="inline-start" />{{
              label("refresh")
            }}</Button
          >
          <Button :disabled="!data" @click="editGroup()"
            ><Plus data-icon="inline-start" />{{ label("newGroup") }}</Button
          >
        </CardAction>
      </CardHeader>
      <CardContent>
        <div class="rounded-md border">
          <Table>
            <TableHeader
              ><TableRow
                ><TableHead>{{ label("name") }}</TableHead
                ><TableHead>{{ label("nodes") }}</TableHead
                ><TableHead /></TableRow
            ></TableHeader>
            <TableBody>
              <TableRow v-for="item in data?.groups" :key="item.id">
                <TableCell class="font-medium">{{ item.name }}</TableCell>
                <TableCell>{{
                  item.nodes.map(nodeName).join(", ") || label("unassigned")
                }}</TableCell>
                <TableCell class="text-end"
                  ><Button variant="outline" @click="editGroup(item)">{{
                    t("ocservUsers.edit")
                  }}</Button></TableCell
                >
              </TableRow>
              <TableRow v-if="!data?.groups?.length"
                ><TableCell
                  colspan="3"
                  class="text-center text-muted-foreground"
                  >{{
                    loading ? label("loading") : label("noGroups")
                  }}</TableCell
                ></TableRow
              >
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
    <Card v-if="data">
      <CardHeader
        ><CardTitle>{{ label("defaults") }}</CardTitle></CardHeader
      >
      <CardContent>
        <div class="rounded-md border">
          <Table>
            <TableHeader
              ><TableRow
                ><TableHead>{{ label("node") }}</TableHead
                ><TableHead>{{ label("ocservGroup") }}</TableHead
                ><TableHead>{{ label("certificateGroup") }}</TableHead
                ><TableHead /></TableRow
            ></TableHeader>
            <TableBody>
              <TableRow v-for="item in data.nodes" :key="item.id">
                <TableCell class="font-medium">{{
                  nodeName(item.id)
                }}</TableCell>
                <TableCell
                  ><Badge variant="secondary">{{
                    defaultsByNode.get(item.id)?.group ?? label("unassigned")
                  }}</Badge></TableCell
                >
                <TableCell>{{
                  data.groups?.find(
                    (group) =>
                      group.id ===
                      defaultsByNode.get(item.id)?.certificate_group,
                  )?.name ?? label("independent")
                }}</TableCell>
                <TableCell class="text-end"
                  ><Button variant="outline" @click="editDefaults(item.id)">{{
                    t("ocservUsers.edit")
                  }}</Button></TableCell
                >
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
    <PortalNodeEditor
      v-if="data"
      v-model:open="editing"
      :mode="mode"
      :inventory="data"
      :group="group"
      :node="node"
      :defaults="selectedDefaults"
      :pending="pending"
      :error="error"
      @save-group="save(() => savePortalGroup($event))"
      @save-defaults="save(() => savePortalDefaults($event))"
    />
  </div>
</template>
