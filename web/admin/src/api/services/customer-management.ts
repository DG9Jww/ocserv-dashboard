import { apiBaseUrl, httpClient } from "@/api/http";
import { isTestMode } from "@/api/environment";
import { mockCustomerManagement } from "@/mocks/customer-management";

export const trafficTypes = [
  "Free",
  "MonthlyTransmit",
  "MonthlyReceive",
  "MonthlyRxTx",
  "TotallyTransmit",
  "TotallyReceive",
  "TotallyRxTx",
] as const;

export interface PortalCustomer {
  id: string;
  locked?: boolean;
  deleted_at?: string;
  username: string;
  default_node: string;
  node: string;
  policy_group_id: string | null;
  traffic_type: (typeof trafficTypes)[number];
  traffic_size: number;
  expires_at: string | null;
}
export interface CustomerPolicyGroup {
  id: string;
  name: string;
  nodes: string[];
  primary_dns: string;
  secondary_dns: string;
  routes: string[];
  upload_kb_per_second: number;
  download_kb_per_second: number;
}
export interface Assignment {
  customer_id: string;
  node: string;
  desired: boolean;
  remove?: boolean;
  version: number;
  applied: number;
  error: string;
}
export interface Inventory {
  nodes: { id: string; name: string }[];
  policy_groups?: CustomerPolicyGroup[] | null;
  customers: PortalCustomer[] | null;
  assignments: Assignment[] | null;
}

const url = "/customer-management";
const master = { baseURL: apiBaseUrl };

export async function loadCustomerManagement(): Promise<Inventory> {
  if (isTestMode) return mockCustomerManagement.inventory();
  return (await httpClient.get<Inventory>(url, master)).data;
}

export type PortalCustomerInput = Pick<
  PortalCustomer,
  "id" | "username" | "node" | "default_node" | "policy_group_id"
> &
  Partial<
    Pick<PortalCustomer, "traffic_type" | "traffic_size" | "expires_at">
  > & {
    password: string;
  };

export async function savePortalCustomer(
  customer: PortalCustomerInput,
): Promise<void> {
  if (isTestMode) return mockCustomerManagement.saveCustomer(customer);
  if (customer.id)
    await httpClient.patch(url + "/accounts/" + customer.id, customer, master);
  else await httpClient.post(url + "/accounts", customer, master);
}

export async function saveCustomerPolicyGroup(
  group: CustomerPolicyGroup,
): Promise<void> {
  if (isTestMode) return mockCustomerManagement.savePolicyGroup(group);
  if (group.id)
    await httpClient.patch(url + "/policy-groups/" + group.id, group, master);
  else await httpClient.post(url + "/policy-groups", group, master);
}

export async function deleteCustomerPolicyGroup(id: string): Promise<void> {
  if (isTestMode) return mockCustomerManagement.deletePolicyGroup(id);
  await httpClient.delete(url + "/policy-groups/" + id, master);
}

export async function setPortalCustomerLocked(
  id: string,
  locked: boolean,
): Promise<void> {
  if (isTestMode) return mockCustomerManagement.setStatus(id, locked);
  await httpClient.patch(
    url + "/accounts/" + id + "/status",
    { locked },
    master,
  );
}

export async function deletePortalCustomer(id: string): Promise<void> {
  if (isTestMode) return mockCustomerManagement.setStatus(id, true, true);
  await httpClient.delete(url + "/accounts/" + id, master);
}
