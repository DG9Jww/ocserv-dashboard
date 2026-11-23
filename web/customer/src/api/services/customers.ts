import type {
  ActivitiesQuery,
  ActivitiesResponse,
  Bandwidth,
  ChangePasswordInput,
  CiscoSetup,
  DailyTraffic,
  DateRangeQuery,
  LoginData,
  LoginResponse,
  OnlineUserSession,
  SummaryResponse,
} from "@/api/generated";
import { isTestMode } from "@/api/environment";
import { httpClient } from "@/api/http";
import type { AxiosResponse } from "axios";
import { mockCustomers } from "@/mocks/customers";

const path = "/customers";
export async function login(request: LoginData): Promise<LoginResponse> {
  return isTestMode
    ? mockCustomers.login(request)
    : (await httpClient.post<LoginResponse>(path + "/login", request)).data;
}
type PortalStatus = { partial: boolean; unavailable: boolean };
const noStatus: PortalStatus = { partial: false, unavailable: false };

function portalStatus(response: AxiosResponse): PortalStatus {
  return {
    partial: response.headers["x-ocserv-partial"] === "true",
    unavailable: response.headers["x-ocserv-unavailable"] === "true",
  };
}
function result<T>(response: AxiosResponse<T>) {
  return { data: response.data, status: portalStatus(response) };
}
export type PortalSummary = SummaryResponse & { portalStatus?: PortalStatus };
export type PortalSession = OnlineUserSession & { portal_key?: string };
export type PortalActivities = Omit<ActivitiesResponse, "result"> & {
  result?: (NonNullable<ActivitiesResponse["result"]>[number] & { portal_key?: string })[] | null;
};

export async function getSummary(): Promise<PortalSummary> {
  if (isTestMode) return mockCustomers.summary();
  const response = await httpClient.get<SummaryResponse>(path + "/summary");
  return { ...response.data, portalStatus: portalStatus(response) };
}
export async function getSessions() {
  if (isTestMode) return { data: await mockCustomers.sessions() as PortalSession[], status: noStatus };
  return result(await httpClient.get<PortalSession[]>(path + "/sessions"));
}
export async function disconnectSessions(): Promise<boolean> {
  if (isTestMode) { await mockCustomers.accept(); return false; }
  return portalStatus(await httpClient.post(path + "/disconnect_sessions")).partial;
}
export async function terminateSessions(): Promise<boolean> {
  if (isTestMode) { await mockCustomers.accept(); return false; }
  return portalStatus(await httpClient.post(path + "/terminate_sessions")).partial;
}
export async function changePassword(request: ChangePasswordInput): Promise<void> {
  if (isTestMode) return mockCustomers.changePassword(request);
  await httpClient.post(path + "/password", request);
}
export async function getActivities(params: ActivitiesQuery) {
  if (isTestMode) return { data: await mockCustomers.activities(params) as PortalActivities, status: noStatus };
  return result(await httpClient.get<PortalActivities>(path + "/activities", { params }));
}
export async function getStats(params: DateRangeQuery) {
  if (isTestMode) return { data: await mockCustomers.stats(params), status: noStatus };
  const response = result(await httpClient.get<DailyTraffic[] | null>(path + "/stats", { params }));
  return { ...response, data: response.data ?? [] };
}
export async function getBandwidth(params: DateRangeQuery) {
  if (isTestMode) return { data: await mockCustomers.bandwidth(params), status: noStatus };
  return result(await httpClient.get<Bandwidth>(path + "/bandwidth", { params }));
}
export async function getCiscoSetup(): Promise<CiscoSetup> {
  return isTestMode
    ? mockCustomers.ciscoSetup()
    : (await httpClient.get<CiscoSetup>(path + "/setup/cisco")).data;
}
