<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { MoreHorizontal, Plus, RefreshCw, Search, Users } from "@lucide/vue";
import { useTimestamp } from "@vueuse/core";
import { useI18n } from "vue-i18n";
import {
  deletePortalCustomer,
  savePortalCustomer,
  setPortalCustomerLocked,
  unlockPortalCustomerLogin,
  type Assignment,
  type PortalCustomer,
  type PortalCustomerInput,
} from "@/api/services/customer-management";
import PortalCustomerEditor from "@/components/customer-management/PortalCustomerEditor.vue";
import OcservUserActionDialog from "@/components/ocserv-users/OcservUserActionDialog.vue";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useCustomerManagement } from "@/composables/useCustomerManagement";

const { t, n, locale } = useI18n({ useScope: "global" });
const {
  label,
  data,
  loading,
  loadError,
  nodeName,
  describe,
  errorMessage,
  refresh,
} = useCustomerManagement(true);
const search = ref("");
const filter = ref("all");
const page = ref(1);
const pageSizes = [10, 20, 50];
const pageSize = ref(20);
const expiryFormat = computed(
  () =>
    new Intl.DateTimeFormat(locale.value, {
      dateStyle: "medium",
      timeStyle: "short",
    }),
);
const editorOpen = ref(false);
const policyGroups = computed(() => data.value?.policy_groups ?? []);
const policyNames = computed(
  () => new Map(policyGroups.value.map((group) => [group.id, group.name])),
);
const grantsOnly = ref(false);
const editorCustomer = ref<PortalCustomer>();
const syncOpen = ref(false);
const syncCustomer = ref<PortalCustomer>();
const actionOpen = ref(false);
const action = ref<"lock" | "unlock" | "unlockLogin" | "delete">("lock");
const actionCustomer = ref<PortalCustomer>();
const pending = ref(false);
const mutationError = ref("");
const success = ref("");

const now = useTimestamp({ interval: 1000 });
function loginIsLocked(customer: PortalCustomer) {
  return Boolean(
    customer.login_locked_until &&
    Date.parse(customer.login_locked_until) > now.value,
  );
}
const filtered = computed(() => {
  const query = search.value.trim().toLowerCase();
  return (data.value?.customers ?? []).filter((customer) => {
    if (
      !customer.username.toLowerCase().includes(query) &&
      !customer.email?.toLowerCase().includes(query)
    )
      return false;
    if (filter.value === "all") return true;
    return (
      !customer.deleted_at &&
      (Boolean(customer.locked) || loginIsLocked(customer)) ===
        (filter.value === "locked")
    );
  });
});
const pages = computed(() =>
  Math.max(1, Math.ceil(filtered.value.length / pageSize.value)),
);
const visible = computed(() =>
  filtered.value.slice(
    (page.value - 1) * pageSize.value,
    page.value * pageSize.value,
  ),
);
watch([search, filter, pageSize], () => {
  page.value = 1;
});
watch(pages, (count) => {
  page.value = Math.min(count, page.value);
});
const assignments = computed(() => {
  const result = new Map<string, Assignment[]>();
  for (const row of data.value?.assignments ?? []) {
    const current = result.get(row.customer_id) ?? [];
    current.push(row);
    result.set(row.customer_id, current);
  }
  return result;
});
const selectedAssignments = computed(() =>
  syncCustomer.value
    ? (assignments.value.get(syncCustomer.value.id) ?? [])
    : [],
);

function keyAssignment(rows: Assignment[]) {
  return (
    rows.find((row) => row.error) ??
    rows.find((row) => row.version !== row.applied) ??
    rows[0]
  );
}
const tableRows = computed(() =>
  visible.value.map((customer) => {
    const rows = assignments.value.get(customer.id) ?? [];
    return { customer, summary: keyAssignment(rows), nodeCount: rows.length };
  }),
);
function showSync(customer: PortalCustomer) {
  syncCustomer.value = customer;
  syncOpen.value = true;
}

function openEditor(customer?: PortalCustomer, editGrants = false) {
  grantsOnly.value = editGrants;
  editorCustomer.value = customer;
  mutationError.value = "";
  success.value = "";
  editorOpen.value = true;
}
function confirm(customer: PortalCustomer, operation: typeof action.value) {
  actionCustomer.value = customer;
  action.value = operation;
  mutationError.value = "";
  success.value = "";
  actionOpen.value = true;
}
async function mutate(task: () => Promise<void>, close: () => void) {
  pending.value = true;
  mutationError.value = "";
  try {
    await task();
    close();
    success.value = label("saved");
    await refresh();
  } catch (cause) {
    mutationError.value = errorMessage(cause);
  } finally {
    pending.value = false;
  }
}
async function save(input: PortalCustomerInput) {
  await mutate(
    () => savePortalCustomer(input),
    () => {
      editorOpen.value = false;
    },
  );
}
async function submitAction() {
  const customer = actionCustomer.value;
  if (!customer) return;
  await mutate(
    () =>
      action.value === "delete"
        ? deletePortalCustomer(customer.id)
        : action.value === "unlockLogin"
          ? unlockPortalCustomerLogin(customer.id)
          : setPortalCustomerLocked(customer.id, action.value === "lock"),
    () => {
      actionOpen.value = false;
      if (action.value === "delete") filter.value = "all";
    },
  );
}
function status(row: Assignment) {
  if (row.remove)
    return label(row.version === row.applied ? "removed" : "pendingRemove");
  if (row.version === row.applied)
    return label(row.desired ? "active" : "inactive");
  return label(row.desired ? "pending" : "pendingDisable");
}
const description = computed(() =>
  t("customerManagement." + action.value + "Description", {
    username: actionCustomer.value?.username ?? "",
  }),
);
</script>

<template>
  <div class="grid gap-4 p-4">
    <Alert v-if="loadError" role="alert" variant="error"
      ><AlertDescription>{{ loadError }}</AlertDescription></Alert
    >
    <Alert v-if="success" role="status"
      ><AlertDescription>{{ success }}</AlertDescription></Alert
    >
    <Card>
      <CardHeader>
        <CardTitle>{{ label("title") }}</CardTitle>
        <CardAction class="flex flex-wrap gap-2">
          <Button
            variant="outline"
            :disabled="loading || pending"
            @click="refresh()"
          >
            <RefreshCw data-icon="inline-start" />{{ label("refresh") }}
          </Button>
          <Button :disabled="!data || pending" @click="openEditor()">
            <Plus data-icon="inline-start" />{{ label("newCustomer") }}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent class="grid gap-4">
        <div class="flex flex-wrap items-center gap-2">
          <div class="relative w-full max-w-xs">
            <Search
              class="pointer-events-none absolute start-3 top-2.5 size-4 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              v-model="search"
              class="ps-9"
              :placeholder="t('ocservUsers.search')"
              :aria-label="t('ocservUsers.search')"
            />
          </div>
          <select
            v-model="filter"
            class="h-9 rounded-md border bg-background px-2 text-sm"
            :aria-label="label('status')"
          >
            <option value="all">{{ label("all") }}</option>
            <option value="enabled">{{ label("enabled") }}</option>
            <option value="locked">{{ label("locked") }}</option>
          </select>
        </div>
        <div class="rounded-md border">
          <Table>
            <TableHeader
              ><TableRow>
                <TableHead>{{ label("username") }}</TableHead>
                <TableHead>{{ label("email") }}</TableHead>
                <TableHead>{{ label("status") }}</TableHead>
                <TableHead>{{ label("expiresAt") }}</TableHead>
                <TableHead>{{ label("defaultConnection") }}</TableHead>
                <TableHead>{{ label("policyGroup") }}</TableHead>
                <TableHead>{{ label("nodeAccess") }}</TableHead>
                <TableHead class="w-12"
                  ><span class="sr-only">{{
                    label("actions")
                  }}</span></TableHead
                >
              </TableRow></TableHeader
            >
            <TableBody>
              <TableRow
                v-for="{ customer, summary, nodeCount } in tableRows"
                :key="customer.id"
              >
                <TableCell class="font-medium">{{
                  customer.username
                }}</TableCell>
                <TableCell
                  ><bdi>{{ customer.email || "—" }}</bdi></TableCell
                >
                <TableCell>
                  <Badge :variant="customer.locked ? 'secondary' : 'outline'">
                    {{
                      customer.deleted_at
                        ? label("pendingRemove")
                        : customer.locked
                          ? label("locked")
                          : loginIsLocked(customer)
                            ? label("loginTemporarilyLocked")
                            : label("enabled")
                    }}
                  </Badge>
                  <div
                    v-if="
                      loginIsLocked(customer) && customer.login_locked_until
                    "
                    class="mt-1 text-xs text-muted-foreground"
                  >
                    {{
                      t("customerManagement.loginLockedUntil", {
                        time: expiryFormat.format(
                          new Date(customer.login_locked_until),
                        ),
                      })
                    }}
                  </div>
                  <div
                    v-else-if="customer.last_login_lock_at"
                    class="mt-1 text-xs text-muted-foreground"
                  >
                    {{
                      t("customerManagement.lastLoginLock", {
                        time: expiryFormat.format(
                          new Date(customer.last_login_lock_at),
                        ),
                      })
                    }}
                  </div>
                </TableCell>
                <TableCell class="whitespace-nowrap">
                  <time
                    v-if="customer.expires_at"
                    :datetime="customer.expires_at"
                  >
                    {{ expiryFormat.format(new Date(customer.expires_at)) }}
                  </time>
                  <span v-else>{{ label("expiryNever") }}</span>
                </TableCell>
                <TableCell>{{
                  data?.nodes.some((node) => node.id === customer.default_node)
                    ? nodeName(customer.default_node)
                    : label("unassigned")
                }}</TableCell>
                <TableCell>{{
                  policyNames.get(customer.policy_group_id ?? "") ??
                  label("nodePolicyDefaults")
                }}</TableCell>
                <TableCell>
                  <template v-if="summary">
                    <div>
                      <span>{{ nodeName(summary.node) }}</span>
                      <Badge
                        class="ms-2"
                        :variant="summary.error ? 'destructive' : 'secondary'"
                        >{{ status(summary) }}</Badge
                      >
                      <p
                        v-if="summary.error"
                        class="mt-1 text-xs text-destructive"
                      >
                        {{ describe(summary.error) }}
                      </p>
                    </div>
                    <Button
                      v-if="nodeCount > 1"
                      class="h-auto px-0"
                      size="sm"
                      variant="link"
                      @click="showSync(customer)"
                      >{{ label("nodeAccess") }} ({{ nodeCount }})</Button
                    >
                  </template>
                  <span v-else class="text-muted-foreground">{{
                    label("unassigned")
                  }}</span>
                </TableCell>
                <TableCell class="text-end">
                  <Button
                    v-if="!customer.deleted_at"
                    variant="outline"
                    size="sm"
                    :disabled="pending"
                    @click="openEditor(customer, true)"
                  >
                    {{ label("manageGrants") }}
                  </Button>
                  <DropdownMenu v-if="!customer.deleted_at">
                    <DropdownMenuTrigger as-child>
                      <Button
                        variant="ghost"
                        size="icon"
                        :aria-label="label('actions')"
                        :disabled="pending"
                        ><MoreHorizontal
                      /></Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem @select="openEditor(customer)">{{
                        t("ocservUsers.edit")
                      }}</DropdownMenuItem>
                      <DropdownMenuItem
                        @select="
                          confirm(customer, customer.locked ? 'unlock' : 'lock')
                        "
                      >
                        {{ label(customer.locked ? "unlock" : "lock") }}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        v-if="loginIsLocked(customer)"
                        @select="confirm(customer, 'unlockLogin')"
                        >{{ label("unlockLogin") }}</DropdownMenuItem
                      >
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        class="text-destructive"
                        @select="confirm(customer, 'delete')"
                        >{{ t("ocservUsers.delete") }}</DropdownMenuItem
                      >
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
              <TableRow v-if="!visible.length">
                <TableCell
                  colspan="8"
                  class="h-28 text-center text-muted-foreground"
                >
                  <Users class="mx-auto mb-2 size-5" aria-hidden="true" />
                  {{ loading ? label("loading") : label("noCustomers") }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </CardContent>
      <Separator />
      <CardFooter
        class="grid items-center justify-items-center gap-3 lg:grid-cols-[1fr_auto_1fr]"
      >
        <span
          class="text-sm text-muted-foreground lg:justify-self-start"
          aria-live="polite"
        >
          {{
            t("reports.pageStatus", {
              page: n(page),
              pages: n(pages),
              total: n(filtered.length),
            })
          }}
        </span>
        <Pagination
          v-if="filtered.length"
          v-slot="{ page: current }"
          v-model:page="page"
          class="w-auto lg:col-start-2"
          :disabled="loading || pending"
          :items-per-page="pageSize"
          :sibling-count="1"
          :total="filtered.length"
          show-edges
        >
          <PaginationContent v-slot="{ items }">
            <PaginationPrevious :label="t('reports.previous')" />
            <template v-for="(item, index) in items" :key="index">
              <PaginationItem
                v-if="item.type === 'page'"
                :value="item.value"
                :is-active="item.value === current"
              >
                {{ n(item.value) }}
              </PaginationItem>
              <PaginationEllipsis
                v-else
                :index="index"
                :label="t('reports.morePages')"
              />
            </template>
            <PaginationNext :label="t('reports.next')" />
          </PaginationContent>
        </Pagination>
        <label
          class="flex items-center gap-2 text-sm lg:col-start-3 lg:justify-self-end"
        >
          {{ t("reports.pageSize") }}
          <select
            v-model.number="pageSize"
            class="h-9 rounded-md border bg-background px-2"
            :disabled="loading || pending"
          >
            <option v-for="size in pageSizes" :key="size" :value="size">
              {{ n(size) }}
            </option>
          </select>
        </label>
      </CardFooter>
    </Card>
    <Sheet v-model:open="syncOpen">
      <SheetContent class="w-full overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{{ label("nodeAccess") }}</SheetTitle>
          <SheetDescription>{{ syncCustomer?.username }}</SheetDescription>
        </SheetHeader>
        <ul class="divide-y px-4">
          <li v-for="row in selectedAssignments" :key="row.node" class="py-3">
            <span>{{ nodeName(row.node) }}</span>
            <Badge
              class="ms-2"
              :variant="row.error ? 'destructive' : 'secondary'"
              >{{ status(row) }}</Badge
            >
            <p v-if="row.error" class="mt-1 text-sm text-destructive">
              {{ describe(row.error) }}
            </p>
          </li>
        </ul>
      </SheetContent>
    </Sheet>
    <PortalCustomerEditor
      v-if="data"
      v-model:open="editorOpen"
      :customer="editorCustomer"
      :grants-only="grantsOnly"
      :inventory="data"
      :pending="pending"
      :error="mutationError"
      @save="save"
    />
    <OcservUserActionDialog
      v-model:open="actionOpen"
      :title="action === 'delete' ? t('ocservUsers.delete') : label(action)"
      :action="action === 'delete' ? t('ocservUsers.delete') : label(action)"
      :description="description"
      :destructive="action === 'delete'"
      :pending="pending"
      :error="mutationError"
      @confirm="submitAction"
    />
  </div>
</template>
