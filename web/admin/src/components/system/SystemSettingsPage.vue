<script setup lang="ts">
import { RefreshCw, Save } from "@lucide/vue";
import { computed, onMounted, reactive, shallowRef } from "vue";
import { useI18n } from "vue-i18n";
import { normalizeApiError } from "@/api/http";
import {
  getOcservConfig,
  getSystemConfig,
  getSystemRelease,
  updateOcservConfig,
  updateSystemConfig,
  type OcservConfig,
  type SystemConfig,
  type SystemUpdateData,
} from "@/api/services/system";
import CustomerLoginProtectionCard from "@/components/system/CustomerLoginProtectionCard.vue";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
const { t } = useI18n({ useScope: "global" });
const loading = shallowRef(true),
  saving = shallowRef<"system" | "ocserv" | null>(null),
  error = shallowRef(""),
  success = shallowRef("");
const release = shallowRef<{ current: string; latest: string } | null>(null),
  ocservOriginal = shallowRef<OcservConfig>({}),
  allowOcservUpdate = shallowRef(false),
  ocservError = shallowRef(""),
  activeTab = shallowRef<"release" | "panel" | "ocserv" | "loginProtection">(
    "release",
  );
const panel = reactive<SystemUpdateData>({
  auto_delete_inactive_users: false,
  client_profile_connection_name: "",
  client_profile_server_address: "",
  client_profile_server_port: 0,
  google_captcha_secret_key: "",
  google_captcha_site_key: "",
  keep_inactive_user_days: 0,
});
const ocserv = reactive<OcservConfig>({});
const certificatePathFields = ["server_cert", "server_key"] as const;
const numberFields: (keyof OcservConfig)[] = [
  "auth_timeout",
  "ban_reset_time",
  "cookie_timeout",
  "dpd",
  "keepalive",
  "log_level",
  "max_ban_score",
  "max_clients",
  "max_same_clients",
  "ban_time",
  "mobile_dpd",
  "mtu",
  "rate_limit_ms",
  "rekey_time",
  "switch_to_tcp_timeout",
  "tcp_port",
  "udp_port",
];
const booleanFields: (keyof OcservConfig)[] = [
  "cisco_client_compat",
  "deny_roaming",
  "dtls_legacy",
  "ping_leases",
  "predictable_ips",
  "try_mtu_discovery",
  "tunnel_all_dns",
];
const textFields: (keyof OcservConfig)[] = [
  "banner",
  "ipv4_network",
  "pre_login_banner",
];
const updateAvailable = computed(() =>
  Boolean(release.value && release.value.current !== release.value.latest),
);
function setPanel(value: SystemConfig): boolean {
  const keys = Object.keys(panel) as (keyof SystemUpdateData)[];
  if (keys.some((key) => typeof value[key] !== typeof panel[key])) return false;
  Object.assign(panel, value);
  return true;
}
function textValue(key: keyof OcservConfig): string {
  return (ocserv[key] as string | undefined) ?? "";
}
function numberValue(key: keyof OcservConfig): number | undefined {
  return ocserv[key] as number | undefined;
}
function booleanValue(key: keyof OcservConfig): boolean | undefined {
  return ocserv[key] as boolean | undefined;
}
function updateText(key: keyof OcservConfig, value: string): void {
  (ocserv as Record<string, unknown>)[key] = value || undefined;
}
function updateBoolean(key: keyof OcservConfig, value: boolean): void {
  (ocserv as Record<string, unknown>)[key] = value;
}
function setOcserv(value: OcservConfig): void {
  Object.keys(ocserv).forEach(
    (key) => delete ocserv[key as keyof OcservConfig],
  );
  Object.assign(ocserv, value);
}
function updateNumber(key: keyof OcservConfig, value: string): void {
  (ocserv as Record<string, unknown>)[key] =
    value === "" ? undefined : Number(value);
}
async function refresh(): Promise<void> {
  loading.value = true;
  error.value = "";
  try {
    const [system, config, nextRelease] = await Promise.all([
      getSystemConfig(),
      getOcservConfig(),
      getSystemRelease(),
    ]);
    if (!setPanel(system))
      throw new Error(t("systemSettings.invalidSystemConfig"));
    allowOcservUpdate.value = config.allow_update;
    const { allow_update: _allowUpdate, ...ocservConfig } = config;
    ocservOriginal.value = ocservConfig;
    setOcserv(ocservConfig);
    release.value = nextRelease;
  } catch (cause) {
    error.value = normalizeApiError(cause).message;
  } finally {
    loading.value = false;
  }
}
async function savePanel(): Promise<void> {
  saving.value = "system";
  error.value = "";
  try {
    setPanel(await updateSystemConfig({ ...panel }));
    success.value = t("systemSettings.panelSaved");
  } catch (cause) {
    error.value = normalizeApiError(cause).message;
  } finally {
    saving.value = null;
  }
}
async function saveOcserv(): Promise<void> {
  if (!allowOcservUpdate.value) {
    error.value = t("systemSettings.updatesUnavailable");
    return;
  }
  const changed = Object.fromEntries(
    Object.entries(ocserv).filter(
      ([key, value]) =>
        JSON.stringify(value) !==
        JSON.stringify(ocservOriginal.value[key as keyof OcservConfig]),
    ),
  ) as OcservConfig;
  saving.value = "ocserv";
  error.value = "";
  ocservError.value = "";
  try {
    await updateOcservConfig(changed);
    await refresh();
    success.value = t("systemSettings.ocservSaved");
  } catch (cause) {
    error.value = normalizeApiError(cause).message;
  } finally {
    saving.value = null;
  }
}
onMounted(refresh);
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">
          {{ t("navigation.settings") }}
        </h1>
        <p class="text-sm text-muted-foreground">
          {{ t("systemSettings.description") }}
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        :disabled="loading || saving !== null"
        @click="refresh"
      >
        <Spinner v-if="loading" data-icon="inline-start" />
        <RefreshCw v-else data-icon="inline-start" />
        {{ t("systemSettings.refresh") }}
      </Button>
    </div>
    <Alert v-if="error" variant="error"
      ><AlertTitle>{{ t("systemSettings.requestFailed") }}</AlertTitle
      ><AlertDescription>{{ error }}</AlertDescription></Alert
    ><Alert v-if="success" variant="success"
      ><AlertTitle>{{ t("systemSettings.success") }}</AlertTitle
      ><AlertDescription>{{ success }}</AlertDescription></Alert
    >
    <div class="flex flex-wrap items-center gap-2">
      <div class="flex flex-wrap gap-2" role="tablist">
        <Button
          v-for="tab in [
            'release',
            'panel',
            'ocserv',
            'loginProtection',
          ] as const"
          :key="tab"
          type="button"
          :variant="activeTab === tab ? 'default' : 'outline'"
          role="tab"
          :aria-selected="activeTab === tab"
          @click="activeTab = tab"
          >{{
            t(
              tab === "loginProtection"
                ? "customerManagement.loginProtection"
                : `systemSettings.tabs.${tab}`,
            )
          }}</Button
        >
      </div>
      <Button as-child variant="outline">
        <RouterLink :to="{ name: 'agent-nodes' }">{{
          t("customerManagement.agentNodes")
        }}</RouterLink>
      </Button>
    </div>
    <CustomerLoginProtectionCard v-if="activeTab === 'loginProtection'" />
    <Card v-if="activeTab === 'release'"
      ><CardHeader
        ><CardTitle>{{ t("systemSettings.release") }}</CardTitle
        ><CardDescription>{{
          t("systemSettings.releaseDescription")
        }}</CardDescription></CardHeader
      ><CardContent v-if="release" class="grid gap-3 sm:grid-cols-2"
        ><div>
          <p class="text-sm text-muted-foreground">
            {{ t("systemSettings.currentVersion") }}
          </p>
          <p>{{ release.current }}</p>
        </div>
        <div>
          <p class="text-sm text-muted-foreground">
            {{ t("systemSettings.latestVersion") }}
          </p>
          <p>{{ release.latest }}</p>
        </div>
        <Alert
          class="sm:col-span-2"
          :variant="updateAvailable ? 'warning' : 'success'"
        >
          <AlertDescription>{{
            t(
              updateAvailable
                ? "systemSettings.updateAvailable"
                : "systemSettings.upToDate",
            )
          }}</AlertDescription>
        </Alert></CardContent
      ></Card
    >
    <Card v-if="activeTab === 'panel'"
      ><CardHeader
        ><CardTitle>{{ t("systemSettings.panel") }}</CardTitle
        ><CardDescription>{{
          t("systemSettings.panelDescription")
        }}</CardDescription></CardHeader
      ><CardContent
        ><FieldGroup
          ><Field
            ><FieldLabel for="connection-name">{{
              t("systemSettings.connectionName")
            }}</FieldLabel
            ><Input
              id="connection-name"
              v-model="panel.client_profile_connection_name"
              :disabled="saving !== null" /></Field
          ><Field
            ><FieldLabel for="server-address">{{
              t("systemSettings.serverAddress")
            }}</FieldLabel
            ><Input
              id="server-address"
              v-model="panel.client_profile_server_address"
              :disabled="saving !== null" /></Field
          ><Field
            ><FieldLabel for="server-port">{{
              t("systemSettings.serverPort")
            }}</FieldLabel
            ><Input
              id="server-port"
              v-model.number="panel.client_profile_server_port"
              min="1"
              type="number"
              :disabled="saving !== null" /></Field
          ><Field
            ><FieldLabel for="inactive-days">{{
              t("systemSettings.inactiveDays")
            }}</FieldLabel
            ><Input
              id="inactive-days"
              v-model.number="panel.keep_inactive_user_days"
              min="0"
              type="number"
              :disabled="saving !== null" /></Field
          ><Field
            ><div class="flex items-center gap-2">
              <Checkbox
                id="auto-delete"
                v-model="panel.auto_delete_inactive_users"
                :disabled="saving !== null"
              /><FieldLabel for="auto-delete">{{
                t("systemSettings.autoDelete")
              }}</FieldLabel>
            </div></Field
          ><Field
            ><FieldLabel for="captcha-site">{{
              t("systemSettings.captchaSite")
            }}</FieldLabel
            ><Input
              id="captcha-site"
              v-model="panel.google_captcha_site_key"
              :disabled="saving !== null" /></Field
          ><Field
            ><FieldLabel for="captcha-secret">{{
              t("systemSettings.captchaSecret")
            }}</FieldLabel
            ><Input
              id="captcha-secret"
              v-model="panel.google_captcha_secret_key"
              type="password"
              :disabled="saving !== null" /></Field></FieldGroup></CardContent
      ><CardFooter
        ><Button type="button" :disabled="saving !== null" @click="savePanel"
          ><Spinner v-if="saving === 'system'" data-icon="inline-start" /><Save
            v-else
            data-icon="inline-start"
          />{{ t("systemSettings.save") }}</Button
        ></CardFooter
      ></Card
    >
    <Card v-if="activeTab === 'ocserv'"
      ><CardHeader
        ><CardTitle>{{ t("systemSettings.ocserv") }}</CardTitle
        ><CardDescription>{{
          t("systemSettings.ocservDescription")
        }}</CardDescription></CardHeader
      ><CardContent>
        <FieldGroup>
          <Field v-for="key in certificatePathFields" :key="key">
            <FieldLabel :for="`ocserv-${key}`">
              {{ t(`systemSettings.tlsPaths.${key}`) }}
            </FieldLabel>
            <Input
              :id="`ocserv-${key}`"
              :model-value="ocserv[key] ?? ''"
              :disabled="loading || saving !== null || !allowOcservUpdate"
              @update:model-value="ocserv[key] = String($event).trim()"
            />
            <FieldDescription v-if="key === 'server_key'">
              {{ t("systemSettings.tlsPaths.help") }}
            </FieldDescription>
          </Field>
          <Field v-for="key in textFields" :key="key"
            ><FieldLabel :for="`ocserv-${key}`">{{
              t("systemSettings.configField", { field: key })
            }}</FieldLabel
            ><Input
              :id="`ocserv-${key}`"
              :model-value="textValue(key)"
              :disabled="saving !== null || !allowOcservUpdate"
              @update:model-value="updateText(key, String($event))" /></Field
          ><Field v-for="key in numberFields" :key="key"
            ><FieldLabel :for="`ocserv-${key}`">{{
              t("systemSettings.configField", { field: key })
            }}</FieldLabel
            ><Input
              :id="`ocserv-${key}`"
              :model-value="numberValue(key)"
              type="number"
              :disabled="saving !== null || !allowOcservUpdate"
              @update:model-value="updateNumber(key, String($event))" /></Field
          ><Field v-for="key in booleanFields" :key="key"
            ><div class="flex items-center gap-2">
              <Checkbox
                :id="`ocserv-${key}`"
                :model-value="booleanValue(key)"
                :disabled="saving !== null || !allowOcservUpdate"
                @update:model-value="updateBoolean(key, Boolean($event))"
              /><FieldLabel :for="`ocserv-${key}`">{{
                t("systemSettings.configField", { field: key })
              }}</FieldLabel>
            </div></Field
          ><Field
            ><FieldLabel for="ocserv-dns">{{
              t("systemSettings.configField", { field: "dns" })
            }}</FieldLabel
            ><Input
              id="ocserv-dns"
              :model-value="ocserv.dns?.join(', ') ?? ''"
              :disabled="saving !== null || !allowOcservUpdate"
              @update:model-value="
                ocserv.dns = String($event)
                  .split(',')
                  .map((item) => item.trim())
                  .filter(Boolean)
              " /></Field
          ><Field
            ><FieldLabel for="ocserv-rekey">{{
              t("systemSettings.configField", { field: "rekey_method" })
            }}</FieldLabel
            ><select
              id="ocserv-rekey"
              v-model="ocserv.rekey_method"
              class="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              :disabled="saving !== null || !allowOcservUpdate"
            >
              <option value="ssl">ssl</option>
              <option value="new-tunnel">new-tunnel</option>
            </select></Field
          ></FieldGroup
        ><FieldDescription v-if="allowOcservUpdate">{{
          t("systemSettings.ocservHelp")
        }}</FieldDescription
        ><FieldDescription v-else>{{
          t("systemSettings.readOnly")
        }}</FieldDescription
        ><FieldError v-if="ocservError">{{
          ocservError
        }}</FieldError> </CardContent
      ><CardFooter
        ><Button
          type="button"
          :disabled="saving !== null || !allowOcservUpdate"
          @click="saveOcserv"
          ><Spinner v-if="saving === 'ocserv'" data-icon="inline-start" /><Save
            v-else
            data-icon="inline-start"
          />{{ t("systemSettings.save") }}</Button
        ></CardFooter
      ></Card
    >
  </div>
</template>
