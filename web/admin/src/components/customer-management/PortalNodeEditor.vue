<script setup lang="ts">
import { reactive, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  trafficTypes,
  type Inventory,
  type NodeDefaults,
  type NodeGroup,
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
  mode: "group" | "defaults";
  group?: NodeGroup;
  defaults?: NodeDefaults;
  node?: string;
  inventory: Inventory;
  pending: boolean;
  error: string;
}>();
const open = defineModel<boolean>("open", { default: false });
const emit = defineEmits<{
  saveGroup: [input: NodeGroup];
  saveDefaults: [input: NodeDefaults];
}>();
const { t } = useI18n({ useScope: "global" });
const label = (key: string) => t("customerManagement." + key);
const group = reactive<NodeGroup>({ id: "", name: "", nodes: [] });
const defaults = reactive<NodeDefaults>({
  node: "",
  group: "defaults",
  traffic_type: "Free",
  traffic_size: 0,
  expiry_days: 0,
  certificate_group: "",
});
const selectClass = "h-9 w-full rounded-md border bg-background px-2 text-sm";
watch(open, (value) => {
  if (!value) return;
  Object.assign(group, {
    id: props.group?.id ?? "",
    name: props.group?.name ?? "",
    nodes: [...(props.group?.nodes ?? [])],
  });
  Object.assign(
    defaults,
    props.defaults ?? {
      node: props.node ?? "",
      group: "defaults",
      traffic_type: "Free",
      traffic_size: 0,
      expiry_days: 0,
      certificate_group: "",
    },
  );
});
function submit() {
  if (props.mode === "group")
    emit("saveGroup", {
      ...group,
      name: group.name.trim(),
      nodes: [...group.nodes],
    });
  else
    emit("saveDefaults", {
      ...defaults,
      group: defaults.group.trim(),
      traffic_size:
        defaults.traffic_type === "Free" ? 0 : defaults.traffic_size,
    });
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
      <SheetHeader>
        <SheetTitle>{{
          label(
            mode === "defaults"
              ? "defaults"
              : group.id
                ? "editGroup"
                : "newGroup",
          )
        }}</SheetTitle>
        <SheetDescription>{{
          label(mode === "defaults" ? "defaultsHelp" : "groupHelp")
        }}</SheetDescription>
      </SheetHeader>
      <form class="grid gap-4 px-4" novalidate @submit.prevent="submit">
        <Alert v-if="error" variant="error" role="alert"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
        <template v-if="mode === 'group'">
          <label class="grid gap-1 text-sm"
            >{{ label("name")
            }}<Input v-model="group.name" required maxlength="128"
          /></label>
          <fieldset class="grid gap-1 text-sm">
            <legend class="mb-1 font-medium">{{ label("nodes") }}</legend>
            <label
              v-for="node in inventory.nodes"
              :key="node.id"
              class="flex items-center gap-2"
            >
              <input v-model="group.nodes" type="checkbox" :value="node.id" />{{
                node.id === "master" ? label("master") : node.name
              }}
            </label>
          </fieldset>
        </template>
        <template v-else>
          <label class="grid gap-1 text-sm"
            >{{ label("ocservGroup")
            }}<Input v-model="defaults.group" required maxlength="16"
          /></label>
          <label class="grid gap-1 text-sm"
            >{{ label("certificateGroup") }}
            <select v-model="defaults.certificate_group" :class="selectClass">
              <option value="">{{ label("independent") }}</option>
              <option
                v-for="item in inventory.groups"
                :key="item.id"
                :value="item.id"
              >
                {{ item.name }}
              </option>
            </select>
          </label>
          <label class="grid gap-1 text-sm"
            >{{ label("trafficPolicy") }}
            <select v-model="defaults.traffic_type" :class="selectClass">
              <option v-for="kind in trafficTypes" :key="kind" :value="kind">
                {{ label(kind) }}
              </option>
            </select>
          </label>
          <label
            v-if="defaults.traffic_type !== 'Free'"
            class="grid gap-1 text-sm"
            >{{ label("trafficBytes") }}
            <input
              v-model.number="defaults.traffic_size"
              type="number"
              min="0"
              :class="selectClass"
              required
            />
          </label>
          <label class="grid gap-1 text-sm"
            >{{ label("expiryDays") }}
            <input
              v-model.number="defaults.expiry_days"
              type="number"
              min="0"
              max="36500"
              :class="selectClass"
              required
            />
          </label>
        </template>
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
