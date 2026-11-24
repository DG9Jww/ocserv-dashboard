<script setup lang="ts">
import {
  ArrowDown,
  ArrowUp,
  MoreHorizontal,
  Search,
  SlidersHorizontal,
} from "@lucide/vue";
import { useI18n } from "vue-i18n";

import type { CustomerPolicyGroup } from "@/api/services/customer-management";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

defineProps<{
  policies: CustomerPolicyGroup[];
  customerCounts: ReadonlyMap<string, number>;
  loading: boolean;
  pending: boolean;
  descending: boolean;
}>();
const search = defineModel<string>("search", { required: true });
const emit = defineEmits<{
  sort: [];
  action: [operation: "view" | "edit" | "delete", policy: CustomerPolicyGroup];
}>();
const { t, n } = useI18n({ useScope: "global" });

function configuredFields(policy: CustomerPolicyGroup) {
  return [
    policy.primary_dns,
    policy.secondary_dns,
    policy.routes?.length,
    policy.upload_kb_per_second,
    policy.download_kb_per_second,
    policy.max_connections,
  ].filter(Boolean).length;
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="relative w-full max-w-xs">
      <Search
        class="pointer-events-none absolute start-3 top-2.5 size-4 text-muted-foreground"
        aria-hidden="true"
      />
      <Input
        v-model="search"
        class="ps-9"
        type="search"
        :aria-label="t('customerManagement.searchPolicies')"
        :placeholder="t('customerManagement.searchPolicies')"
      />
    </div>
    <div class="rounded-md border">
      <Table :aria-busy="loading">
        <TableHeader>
          <TableRow>
            <TableHead
              class="text-center"
              :aria-sort="descending ? 'descending' : 'ascending'"
            >
              <Button type="button" variant="ghost" @click="emit('sort')">
                {{ t("customerManagement.name") }}
                <component
                  :is="descending ? ArrowDown : ArrowUp"
                  data-icon="inline-end"
                />
              </Button>
            </TableHead>
            <TableHead class="text-center">{{
              t("customerManagement.configuredPolicyFields")
            }}</TableHead>
            <TableHead class="text-center">{{
              t("customerManagement.assignedPolicyCustomers")
            }}</TableHead>
            <TableHead class="w-12"
              ><span class="sr-only">{{
                t("customerManagement.policyActions")
              }}</span></TableHead
            >
          </TableRow>
        </TableHeader>
        <TableBody>
          <template v-if="loading && !policies.length">
            <TableRow v-for="row in 3" :key="row">
              <TableCell v-for="column in 4" :key="column">
                <Skeleton class="mx-auto h-5 w-12 max-w-full" />
              </TableCell>
            </TableRow>
          </template>
          <TableRow v-else-if="!policies.length">
            <TableCell colspan="4">
              <Empty class="border-0 py-14">
                <EmptyHeader>
                  <EmptyMedia variant="icon"><SlidersHorizontal /></EmptyMedia>
                  <EmptyTitle>{{
                    t("customerManagement.noPolicies")
                  }}</EmptyTitle>
                </EmptyHeader>
              </Empty>
            </TableCell>
          </TableRow>
          <TableRow v-for="policy in policies" :key="policy.id">
            <TableCell class="text-center font-medium"
              ><bdi>{{ policy.name }}</bdi></TableCell
            >
            <TableCell class="text-center text-muted-foreground">{{
              n(configuredFields(policy))
            }}</TableCell>
            <TableCell class="text-center tabular-nums">{{
              n(customerCounts.get(policy.id) ?? 0)
            }}</TableCell>
            <TableCell class="text-center">
              <DropdownMenu :modal="false">
                <DropdownMenuTrigger as-child>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    :disabled="pending"
                    :aria-label="t('customerManagement.policyActions')"
                  >
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuGroup>
                    <DropdownMenuItem
                      @select="emit('action', 'view', policy)"
                      >{{ t("ocservGroups.view") }}</DropdownMenuItem
                    >
                    <DropdownMenuItem
                      @select="emit('action', 'edit', policy)"
                      >{{ t("ocservGroups.edit") }}</DropdownMenuItem
                    >
                    <DropdownMenuItem
                      variant="destructive"
                      @select="emit('action', 'delete', policy)"
                      >{{ t("ocservGroups.delete") }}</DropdownMenuItem
                    >
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
