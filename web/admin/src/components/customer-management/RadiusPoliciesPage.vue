<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { Plus, RefreshCw } from "@lucide/vue";
import { useI18n } from "vue-i18n";

import {
  deleteCustomerPolicyGroup,
  saveCustomerPolicyGroup,
  type CustomerPolicyGroup,
} from "@/api/services/customer-management";
import RadiusPoliciesTable from "@/components/customer-management/RadiusPoliciesTable.vue";
import RadiusPolicySheet from "@/components/customer-management/RadiusPolicySheet.vue";
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
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { Separator } from "@/components/ui/separator";
import { Spinner } from "@/components/ui/spinner";
import { useCustomerManagement } from "@/composables/useCustomerManagement";

const { t, locale, n } = useI18n({ useScope: "global" });
const { label, data, loading, loadError, errorMessage, refresh } =
  useCustomerManagement();
const PAGE_SIZE = 20;
const search = shallowRef("");
const descending = shallowRef(false);
const page = shallowRef(1);
const currentPolicy = shallowRef<CustomerPolicyGroup>();
const readOnly = shallowRef(false);
const sheetOpen = shallowRef(false);
const deleteOpen = shallowRef(false);
const pending = shallowRef(false);
const mutationError = shallowRef("");
const success = shallowRef("");

const customerCounts = computed(() => {
  const counts = new Map<string, number>();
  for (const customer of data.value?.customers ?? []) {
    const id = customer.policy_group_id;
    if (id) counts.set(id, (counts.get(id) ?? 0) + 1);
  }
  return counts;
});
const filtered = computed(() => {
  const query = search.value.trim().toLocaleLowerCase(locale.value);
  const compare = new Intl.Collator(locale.value, {
    sensitivity: "base",
    numeric: true,
  }).compare;
  return (data.value?.policy_groups ?? [])
    .filter((policy) =>
      policy.name.toLocaleLowerCase(locale.value).includes(query),
    )
    .sort((a, b) =>
      descending.value ? compare(b.name, a.name) : compare(a.name, b.name),
    );
});
const pages = computed(() =>
  Math.max(1, Math.ceil(filtered.value.length / PAGE_SIZE)),
);
const visible = computed(() =>
  filtered.value.slice((page.value - 1) * PAGE_SIZE, page.value * PAGE_SIZE),
);

watch([search, descending], () => {
  page.value = 1;
});
watch(pages, (count) => {
  page.value = Math.min(count, page.value);
});

function openSheet(policy?: CustomerPolicyGroup, view = false) {
  currentPolicy.value = policy;
  readOnly.value = view;
  mutationError.value = "";
  sheetOpen.value = true;
}

function openDelete(policy: CustomerPolicyGroup) {
  currentPolicy.value = policy;
  mutationError.value = "";
  deleteOpen.value = true;
}

function selectAction(
  action: "view" | "edit" | "delete",
  policy: CustomerPolicyGroup,
) {
  if (action === "delete") openDelete(policy);
  else openSheet(policy, action === "view");
}

async function mutate(task: () => Promise<void>, message: string) {
  if (pending.value) return;
  pending.value = true;
  mutationError.value = "";
  success.value = "";
  try {
    await task();
    sheetOpen.value = false;
    deleteOpen.value = false;
    success.value = message;
    await refresh();
  } catch (cause) {
    mutationError.value = errorMessage(cause);
  } finally {
    pending.value = false;
  }
}

function save(policy: CustomerPolicyGroup) {
  return mutate(() => saveCustomerPolicyGroup(policy), label("policySaved"));
}

function remove() {
  const id = currentPolicy.value?.id;
  if (id)
    return mutate(() => deleteCustomerPolicyGroup(id), label("policyDeleted"));
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <Alert v-if="loadError" role="alert" variant="error">
      <AlertDescription>{{ loadError }}</AlertDescription>
    </Alert>
    <Alert v-if="success" role="status" variant="success">
      <AlertDescription>{{ success }}</AlertDescription>
    </Alert>
    <Card>
      <CardHeader
        class="has-data-[slot=card-action]:grid-cols-1 sm:has-data-[slot=card-action]:grid-cols-[1fr_auto]"
      >
        <CardTitle>{{ label("radiusPolicies") }}</CardTitle>
        <CardDescription>{{
          label("radiusPoliciesDescription")
        }}</CardDescription>
        <CardAction
          class="col-start-1 row-span-1 row-start-3 flex flex-wrap gap-2 justify-self-start sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:justify-self-end"
        >
          <Button
            type="button"
            variant="outline"
            :disabled="loading || pending"
            @click="refresh()"
          >
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCw v-else data-icon="inline-start" />
            {{ t("ocservGroups.refresh") }}
          </Button>
          <Button
            type="button"
            :disabled="!data || pending"
            @click="openSheet()"
          >
            <Plus data-icon="inline-start" />
            {{ label("newPolicy") }}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <RadiusPoliciesTable
          v-model:search="search"
          :policies="visible"
          :customer-counts="customerCounts"
          :loading="loading"
          :pending="pending"
          :descending="descending"
          @sort="descending = !descending"
          @action="selectAction"
        />
      </CardContent>
      <Separator />
      <CardFooter class="grid items-center gap-3 sm:grid-cols-[1fr_auto_1fr]">
        <span class="text-center text-sm text-muted-foreground sm:text-start">
          {{
            t("customerManagement.policyPageStatus", {
              page: n(page),
              pages: n(pages),
              total: n(filtered.length),
            })
          }}
        </span>
        <Pagination
          v-if="pages > 1"
          v-slot="{ page: current }"
          v-model:page="page"
          class="mx-auto w-auto sm:col-start-2"
          :disabled="loading || pending"
          :items-per-page="PAGE_SIZE"
          :sibling-count="1"
          :total="filtered.length"
          show-edges
        >
          <PaginationContent v-slot="{ items }">
            <PaginationPrevious :label="t('ocservGroups.previous')" />
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
                :label="label('policyMorePages')"
              />
            </template>
            <PaginationNext :label="t('ocservGroups.next')" />
          </PaginationContent>
        </Pagination>
      </CardFooter>
    </Card>

    <RadiusPolicySheet
      v-model:open="sheetOpen"
      :policy="currentPolicy"
      :read-only="readOnly"
      :pending="pending"
      :error="mutationError"
      @save="save"
    />
    <OcservUserActionDialog
      v-model:open="deleteOpen"
      :title="label('deletePolicy')"
      :action="t('ocservGroups.delete')"
      :description="
        t('customerManagement.deletePolicyDescription', {
          name: currentPolicy?.name ?? '',
        })
      "
      :pending="pending"
      :error="mutationError"
      destructive
      @confirm="remove"
    />
  </div>
</template>
