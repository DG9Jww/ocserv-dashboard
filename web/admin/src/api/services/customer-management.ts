import { apiBaseUrl, httpClient } from "@/api/http";
import { isTestMode } from "@/api/environment";

export const trafficTypes = [
  "Free", "MonthlyTransmit", "MonthlyReceive", "MonthlyRxTx",
  "TotallyTransmit", "TotallyReceive", "TotallyRxTx",
] as const;

export interface NodeGroup {
  id: string;
  name: string;
  nodes: string[];
}
export interface PortalCustomer {
  id: string;
  username: string;
  default_node: string;
  nodes: string[];
  groups: string[];
}
export interface NodeDefaults {
  node: string;
  group: string;
  traffic_type: (typeof trafficTypes)[number];
  traffic_size: number;
  expiry_days: number;
  certificate_group: string;
}
export interface Assignment {
  customer_id: string;
  node: string;
  desired: boolean;
  version: number;
  applied: number;
  error: string;
}
export interface Inventory {
  nodes: { id: string; name: string }[];
  groups: NodeGroup[] | null;
  customers: PortalCustomer[] | null;
  defaults: NodeDefaults[] | null;
  assignments: Assignment[] | null;
}

const url = "/customer-management";
const master = { baseURL: apiBaseUrl };

export async function loadCustomerManagement(): Promise<Inventory> {
  if (isTestMode) return { nodes: [{ id: "master", name: "" }], groups: [], customers: [], defaults: [], assignments: [] };
  return (await httpClient.get<Inventory>(url, master)).data;
}

export async function savePortalCustomer(customer: PortalCustomer & { password: string }): Promise<void> {
  if (customer.id) await httpClient.patch(url + "/accounts/" + customer.id, customer, master);
  else await httpClient.post(url + "/accounts", customer, master);
}

export async function savePortalGroup(group: NodeGroup): Promise<void> {
  if (group.id) await httpClient.patch(url + "/groups/" + group.id, group, master);
  else await httpClient.post(url + "/groups", group, master);
}

export async function savePortalDefaults(defaults: NodeDefaults): Promise<void> {
  await httpClient.patch(url + "/nodes/" + defaults.node, defaults, master);
}
