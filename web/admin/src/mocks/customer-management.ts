import type {
  Assignment,
  CustomerPolicyGroup,
  Inventory,
  PortalCustomer,
  PortalCustomerInput,
} from "@/api/services/customer-management";
import { ApiError } from "@/api/http";
import { getMockAgents } from "@/mocks/system";
import { cloneMock } from "@/mocks/utils";

const policyGroups: CustomerPolicyGroup[] = [];
const customers: PortalCustomer[] = [];
let assignments: Assignment[] = [];

function reconcile() {
  for (const customer of customers) {
    const targets = customer.node
      ? [customer.node]
      : (policyGroups.find((policy) => policy.id === customer.policy_group_id)
          ?.nodes ?? []);
    if (!targets.includes(customer.default_node)) customer.default_node = "";
    const desired = new Set(customer.locked ? [] : targets);
    const history = assignments.filter(
      (item) => item.customer_id === customer.id,
    );
    for (const item of history) {
      item.desired = desired.delete(item.node);
    }
    assignments = [
      ...assignments,
      ...[...desired].map((node) => ({
        customer_id: customer.id,
        node,
        desired: true,
        version: 1,
        applied: 1,
        error: "",
      })),
    ];
  }
}
function saveByID<T extends { id: string }>(items: T[], value: T) {
  const index = items.findIndex((item) => item.id === value.id);
  if (index === -1) items.push(value);
  else items[index] = value;
}

export const mockCustomerManagement = {
  async inventory(): Promise<Inventory> {
    const agents = await getMockAgents();
    const nodes = [
      { id: "master", name: "" },
      ...agents
        .filter((agent) => agent.id != null)
        .map((agent) => ({ id: "agent-" + agent.id, name: agent.name })),
    ];
    return cloneMock({
      nodes,
      policy_groups: policyGroups,
      customers,
      assignments,
    });
  },
  saveCustomer(input: PortalCustomerInput) {
    const { password, policy_group_id, ...account } = input;
    const previous = customers.find((item) => item.id === account.id);
    if (Boolean(account.node) === Boolean(policy_group_id))
      throw new ApiError("customer_grant_required", { status: 400 });
    const policy = policyGroups.find((item) => item.id === policy_group_id);
    if (policy_group_id && !policy)
      throw new ApiError("invalid_customer_configuration", { status: 400 });
    const targets = new Set(policy?.nodes ?? [account.node]);
    if (account.default_node && !targets.has(account.default_node))
      throw new ApiError("invalid_customer_configuration", { status: 400 });
    if (
      !previous &&
      (!/^[A-Za-z0-9._-]{2,32}$/.test(account.username) ||
        account.username === "..")
    )
      throw new ApiError("customer_invalid_username", { status: 400 });
    if (
      (!previous || password) &&
      (password.length < 2 || password.length > 32)
    )
      throw new ApiError("customer_invalid_password", { status: 400 });
    if (
      customers.some(
        (item) => item.username === input.username && item.id !== input.id,
      )
    )
      throw new ApiError("customer_account_conflict", { status: 409 });
    saveByID(customers, {
      ...previous,
      ...cloneMock(account),
      policy_group_id,
      traffic_type: account.traffic_type ?? previous?.traffic_type ?? "Free",
      traffic_size: account.traffic_size ?? previous?.traffic_size ?? 0,
      expires_at:
        account.expires_at === undefined
          ? (previous?.expires_at ?? null)
          : account.expires_at,
      id: account.id || crypto.randomUUID(),
    });
    reconcile();
  },
  savePolicyGroup(group: CustomerPolicyGroup) {
    if (!group.name.trim())
      throw new ApiError("customer_policy_invalid", { status: 400 });
    if (
      policyGroups.some(
        (item) => item.name === group.name && item.id !== group.id,
      )
    )
      throw new ApiError("customer_account_conflict", { status: 409 });
    saveByID(policyGroups, {
      ...cloneMock(group),
      id: group.id || crypto.randomUUID(),
    });
    reconcile();
  },
  deletePolicyGroup(id: string) {
    if (customers.some((item) => item.policy_group_id === id))
      throw new ApiError("customer_policy_in_use", { status: 409 });
    const index = policyGroups.findIndex((item) => item.id === id);
    if (index >= 0) policyGroups.splice(index, 1);
  },
  setStatus(id: string, locked: boolean, remove = false) {
    const customer = customers.find((item) => item.id === id);
    if (!customer)
      throw new ApiError("customer_default_unavailable", { status: 404 });
    if (remove) {
      customers.splice(customers.indexOf(customer), 1);
      assignments = assignments.filter((item) => item.customer_id !== id);
      return;
    }
    customer.locked = locked;
    reconcile();
  },
};
