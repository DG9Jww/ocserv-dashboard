<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { useI18n } from "vue-i18n";

import type { CustomerPolicyGroup } from "@/api/services/customer-management";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
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
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";

const props = defineProps<{
  policy?: CustomerPolicyGroup;
  nodes: { id: string; name: string }[];
  readOnly: boolean;
  pending: boolean;
  error: string;
}>();
const open = defineModel<boolean>("open", { required: true });
const emit = defineEmits<{ save: [policy: CustomerPolicyGroup] }>();
const { t } = useI18n({ useScope: "global" });
const label = (key: string) => t("customerManagement." + key);
const title = computed(() =>
  label(
    props.readOnly ? "viewPolicy" : props.policy ? "editPolicy" : "newPolicy",
  ),
);

const fields = [
  { key: "primary_dns", label: "primaryDNS", hint: "primaryDNSHint" },
  { key: "secondary_dns", label: "secondaryDNS", hint: "secondaryDNSHint" },
  { key: "routes", label: "policyRoutes", hint: "policyRoutesHint" },
  {
    key: "upload_kb_per_second",
    label: "uploadKBPerSecond",
    hint: "uploadLimitHint",
  },
  {
    key: "download_kb_per_second",
    label: "downloadKBPerSecond",
    hint: "downloadLimitHint",
  },
  {
    key: "max_connections",
    label: "maxConnections",
    hint: "maxConnectionsHint",
  },
] as const;

const form = reactive({
  name: "",
  nodes: [] as string[],
  primary_dns: "",
  secondary_dns: "",
  routes: "",
  upload_kb_per_second: "" as string | number,
  download_kb_per_second: "" as string | number,
  max_connections: "" as string | number,
});

watch(
  [open, () => props.policy],
  ([isOpen, policy]) => {
    if (!isOpen) return;
    form.name = policy?.name ?? "";
    form.nodes = [...(policy?.nodes ?? [])];
    form.primary_dns = policy?.primary_dns ?? "";
    form.secondary_dns = policy?.secondary_dns ?? "";
    form.routes = policy?.routes?.join("\n") ?? "";
    form.upload_kb_per_second = policy?.upload_kb_per_second
      ? String(policy.upload_kb_per_second)
      : "";
    form.download_kb_per_second = policy?.download_kb_per_second
      ? String(policy.download_kb_per_second)
      : "";
    form.max_connections = policy?.max_connections
      ? String(policy.max_connections)
      : "";
  },
  { immediate: true },
);

function submit() {
  emit("save", {
    id: props.policy?.id ?? "",
    name: form.name.trim(),
    nodes: [...form.nodes],
    primary_dns: form.primary_dns.trim(),
    secondary_dns: form.secondary_dns.trim(),
    routes: form.routes.split(/\s+/).filter(Boolean),
    upload_kb_per_second: Number(form.upload_kb_per_second),
    download_kb_per_second: Number(form.download_kb_per_second),
    max_connections: Number(form.max_connections),
  });
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
      <SheetHeader class="pe-10">
        <SheetTitle>{{ title }}</SheetTitle>
        <SheetDescription>{{ label("policyHelp") }}</SheetDescription>
      </SheetHeader>

      <dl v-if="readOnly" class="grid gap-4 px-4 text-sm">
        <div>
          <dt class="text-muted-foreground">{{ label("name") }}</dt>
          <dd class="break-words font-medium">
            <bdi>{{ form.name }}</bdi>
          </dd>
        </div>
        <div>
          <dt class="text-muted-foreground">{{ label("nodes") }}</dt>
          <dd>
            {{
              form.nodes
                .map((id) =>
                  id === "master"
                    ? label("master")
                    : nodes.find((node) => node.id === id)?.name || id,
                )
                .join(", ") || label("unassigned")
            }}
          </dd>
        </div>
        <div v-for="field in fields" :key="field.key">
          <dt class="text-muted-foreground">{{ label(field.label) }}</dt>
          <dd v-if="form[field.key]" class="whitespace-pre-line">
            <bdi dir="ltr">{{ form[field.key] }}</bdi>
          </dd>
          <dd v-else-if="field.key === 'secondary_dns' && form.primary_dns">
            —
          </dd>
          <dd v-else>
            {{
              label(
                field.key === "max_connections"
                  ? "noConnectionLimit"
                  : "nodePolicyDefaults",
              )
            }}
          </dd>
        </div>
      </dl>

      <SheetFooter v-if="readOnly">
        <SheetClose as-child>
          <Button type="button" variant="outline">{{
            t("common.close")
          }}</Button>
        </SheetClose>
      </SheetFooter>

      <form v-else class="grid gap-4 px-4" @submit.prevent="submit">
        <Alert v-if="error" role="alert" variant="error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <FieldGroup class="gap-4">
          <Field>
            <FieldLabel for="radius-policy-name">{{
              label("name")
            }}</FieldLabel>
            <Input
              id="radius-policy-name"
              v-model="form.name"
              required
              maxlength="128"
              :disabled="pending"
            />
          </Field>
          <Field v-for="field in fields" :key="field.key">
            <FieldLabel :for="field.key">{{ label(field.label) }}</FieldLabel>
            <Textarea
              v-if="field.key === 'routes'"
              :id="field.key"
              v-model="form.routes"
              rows="3"
              dir="ltr"
              :aria-describedby="field.key + '-hint'"
              :disabled="pending"
            />
            <Input
              v-else-if="
                field.key === 'primary_dns' || field.key === 'secondary_dns'
              "
              :id="field.key"
              v-model="form[field.key]"
              dir="ltr"
              :aria-describedby="field.key + '-hint'"
              :disabled="pending"
            />
            <Input
              v-else
              :id="field.key"
              v-model="form[field.key]"
              type="number"
              min="0"
              max="4294967295"
              dir="ltr"
              :aria-describedby="field.key + '-hint'"
              :disabled="pending"
            />
            <FieldDescription :id="field.key + '-hint'">{{
              label(field.hint)
            }}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel for="radius-policy-nodes">{{
              label("assignNodes")
            }}</FieldLabel>
            <Select v-model="form.nodes" multiple :disabled="pending">
              <SelectTrigger
                id="radius-policy-nodes"
                class="w-full"
                aria-describedby="radius-policy-nodes-help"
              >
                <SelectValue :placeholder="label('unassigned')" />
              </SelectTrigger>
              <SelectContent align="start">
                <SelectGroup>
                  <SelectItem
                    v-for="node in nodes"
                    :key="node.id"
                    :value="node.id"
                  >
                    {{ node.id === "master" ? label("master") : node.name }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldDescription id="radius-policy-nodes-help">{{
              label("policyNodesHelp")
            }}</FieldDescription>
          </Field>
        </FieldGroup>
        <SheetFooter class="px-0">
          <SheetClose as-child>
            <Button type="button" variant="outline" :disabled="pending">{{
              t("ocservGroups.cancel")
            }}</Button>
          </SheetClose>
          <Button type="submit" :disabled="pending">
            <Spinner v-if="pending" data-icon="inline-start" />
            {{ label("save") }}
          </Button>
        </SheetFooter>
      </form>
    </SheetContent>
  </Sheet>
</template>
