<script setup lang="ts">
import { reactive, watch } from "vue";
import { useI18n } from "vue-i18n";

import { ModelsAgentAddressType } from "@/api/generated";
import type { OcservAgent, OcservAgentCreate } from "@/api/services/system";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
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

const props = defineProps<{
  agent?: OcservAgent;
  pending: boolean;
  error: string;
}>();
const emit = defineEmits<{ save: [request: OcservAgentCreate] }>();
const open = defineModel<boolean>("open", { default: false });
const { t } = useI18n({ useScope: "global" });
const form = reactive<OcservAgentCreate>({
  name: "",
  address: "",
  address_type: ModelsAgentAddressType.AgentAddressTypeDomain,
  port: 443,
  token: "",
});
watch(open, (value) => {
  if (!value) {
    form.token = "";
    return;
  }
  Object.assign(form, {
    name: props.agent?.name ?? "",
    address: props.agent?.address ?? "",
    address_type:
      props.agent?.address_type ??
      ModelsAgentAddressType.AgentAddressTypeDomain,
    port: props.agent ? (props.agent.port ?? 8080) : 443,
    token: "",
  });
});

function submit() {
  emit("save", {
    ...form,
    name: form.name.trim(),
    address: form.address.trim(),
    token: form.token.trim(),
  });
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
      <SheetHeader>
        <SheetTitle>{{
          t(agent ? "systemSettings.updateAgent" : "systemSettings.newAgent")
        }}</SheetTitle>
        <SheetDescription>{{
          t("systemSettings.agentsDescription")
        }}</SheetDescription>
      </SheetHeader>
      <form
        class="flex flex-col gap-4 px-4"
        novalidate
        @submit.prevent="submit"
      >
        <Alert v-if="error" role="alert" variant="error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
        <FieldGroup>
          <Field>
            <FieldLabel for="sheet-agent-name">{{
              t("systemSettings.agentName")
            }}</FieldLabel>
            <Input
              id="sheet-agent-name"
              v-model="form.name"
              required
              maxlength="128"
              :disabled="pending"
            />
          </Field>
          <Field>
            <FieldLabel for="sheet-agent-address">{{
              t("systemSettings.agentAddress")
            }}</FieldLabel>
            <Input
              id="sheet-agent-address"
              v-model="form.address"
              dir="ltr"
              required
              maxlength="255"
              :disabled="pending"
            />
          </Field>
          <Field>
            <FieldLabel for="sheet-agent-type">{{
              t("systemSettings.addressType")
            }}</FieldLabel>
            <Select v-model="form.address_type" :disabled="pending">
              <SelectTrigger id="sheet-agent-type"
                ><SelectValue
              /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem
                    :value="ModelsAgentAddressType.AgentAddressTypeDomain"
                    >{{ t("systemSettings.domain") }}</SelectItem
                  >
                  <SelectItem
                    :value="ModelsAgentAddressType.AgentAddressTypeIP"
                    >{{ t("systemSettings.ip") }}</SelectItem
                  >
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel for="sheet-agent-port">{{
              t("systemSettings.serverPort")
            }}</FieldLabel>
            <Input
              id="sheet-agent-port"
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
              required
              :disabled="pending"
            />
          </Field>
          <Field>
            <FieldLabel for="sheet-agent-token">{{
              t("systemSettings.agentToken")
            }}</FieldLabel>
            <Input
              id="sheet-agent-token"
              v-model="form.token"
              type="password"
              autocomplete="new-password"
              maxlength="512"
              :required="!agent"
              :disabled="pending"
            />
          </Field>
        </FieldGroup>
        <SheetFooter class="px-0">
          <SheetClose as-child
            ><Button type="button" variant="outline" :disabled="pending">{{
              t("systemSettings.cancel")
            }}</Button></SheetClose
          >
          <Button type="submit" :disabled="pending">
            <Spinner v-if="pending" data-icon="inline-start" />
            {{
              t(
                agent
                  ? "systemSettings.updateAgent"
                  : "systemSettings.createAgent",
              )
            }}
          </Button>
        </SheetFooter>
      </form>
    </SheetContent>
  </Sheet>
</template>
