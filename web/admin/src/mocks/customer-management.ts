import type {
  Assignment,
  Inventory,
  NodeDefaults,
  NodeGroup,
  PortalCustomer,
  PortalCustomerInput,
} from "@/api/services/customer-management";
import { ApiError } from "@/api/http";
import { getMockAgents } from "@/mocks/system";
import { cloneMock } from "@/mocks/utils";

const groups: NodeGroup[] = [];
const customers: PortalCustomer[] = [];
const defaults: NodeDefaults[] = [];
let assignments: Assignment[] = [];

function reconcile() {
  for (const customer of customers) {
    const desired = new Set(
      customer.locked || customer.deleted_at
        ? []
        : [
            ...customer.nodes,
            ...groups
              .filter((group) => customer.groups.includes(group.id))
              .flatMap((group) => group.nodes),
          ],
    );
    const history = assignments.filter(
      (item) => item.customer_id === customer.id,
    );
    for (const item of history) {
      item.desired = desired.delete(item.node);
      item.remove = Boolean(customer.deleted_at);
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
    return cloneMock({ nodes, groups, customers, defaults, assignments });
  },
  saveCustomer(input: PortalCustomerInput) {
    const { password, ...account } = input;
    const previous = customers.find((item) => item.id === account.id);
    const targets = new Set([
      ...account.nodes,
      ...groups
        .filter((group) => account.groups.includes(group.id))
        .flatMap((group) => group.nodes),
    ]);
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
    if (!previous && !targets.size)
      throw new ApiError("customer_grant_required", { status: 400 });
    if (
      [...targets].some((node) => !defaults.some((item) => item.node === node))
    )
      throw new ApiError("customer_node_defaults_missing", { status: 400 });
    if (
      customers.some(
        (item) => item.username === input.username && item.id !== input.id,
      )
    )
      throw new ApiError("customer_account_conflict", { status: 409 });
    saveByID(customers, {
      ...previous,
      ...cloneMock(account),
      id: account.id || crypto.randomUUID(),
    });
    reconcile();
  },
  saveGroup(group: NodeGroup) {
    if (!group.name)
      throw new ApiError("invalid_customer_configuration", { status: 400 });
    saveByID(groups, {
      ...cloneMock(group),
      id: group.id || crypto.randomUUID(),
    });
    reconcile();
  },
  saveDefaults(value: NodeDefaults) {
    if (!value.group)
      throw new ApiError("invalid_customer_configuration", { status: 400 });
    const index = defaults.findIndex((item) => item.node === value.node);
    if (index < 0) defaults.push({ ...value });
    else defaults[index] = { ...value };
  },
  setStatus(id: string, locked: boolean, remove = false) {
    const customer = customers.find((item) => item.id === id);
    if (!customer)
      throw new ApiError("customer_default_unavailable", { status: 404 });
    customer.locked = locked;
    if (remove) {
      customer.deleted_at = new Date().toISOString();
      customer.nodes = [];
      customer.groups = [];
      customer.default_node = "";
    }
    reconcile();
  },
};
