/**
 * Team management API (v2 model).
 *
 * URL pattern:
 *   /team/:teamId/*              — team_admin endpoints
 *   /admin/teams/:id/*           — sys admin endpoints (same shape, different auth)
 *
 * Both roles use parallel routes and exchange the same payloads. Pages select
 * the variant via a `source: 'admin' | 'team_admin'` prop.
 */

import { apiClient } from './client'
import type { PaginatedResponse, UsageRequestType } from '@/types'

// ── Domain types ──────────────────────────────────────────────────────────────

export interface Team {
  id: number
  name: string
  balance: number
  available_tags?: string[]
  max_members: number // 0 = unlimited
  created_at: string
  updated_at: string
  member_count?: number
}

export interface TeamMember {
  id: number
  team_id: number
  user_id: number
  sub_quota: number
  sub_quota_used: number
  tags: string[] | null
  created_at: string
  updated_at: string
  user?: {
    id: number
    email: string
    username: string
    status: string
  }
  active_subscriptions?: number
  last_active_at?: string | null
  is_admin?: boolean // computed: this user has a team_admins row for this team
}

export interface TeamAdmin {
  id: number
  team_id: number
  user_id: number
  created_at: string
  user?: { id: number; email: string; username?: string; status?: string }
}

export interface TeamBalanceLog {
  id: number
  team_id: number
  type: 'recharge' | 'subscription_purchase'
  amount: number
  operator_id: number
  target_user_id?: number | null
  note?: string | null
  created_at: string
  operator?: { id: number; email: string; username?: string } | null
  target_user?: { id: number; email: string; username?: string } | null
}

export interface TeamSubscription {
  id: number
  user_id: number
  group_id: number
  team_id: number | null
  starts_at: string
  expires_at: string
  status: string
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  user?: { id: number; email: string; username: string }
  group?: { id: number; name: string; platform: string }
}

export interface SubscriptionPlan {
  id: number
  group_id: number
  name: string
  description: string
  price: number
  validity_days: number
  product_name: string
  for_sale: boolean
}

// ── Path helpers ──────────────────────────────────────────────────────────────

type Source = 'admin' | 'team_admin'

function teamBasePath(source: Source, teamId: number): string {
  return source === 'admin' ? `/admin/teams/${teamId}` : `/team/${teamId}`
}

// ── User-scope endpoints (no team id needed) ──────────────────────────────────

/** GET /api/v1/team/me — my membership record */
export async function getMyMembership(): Promise<TeamMember | null> {
  const { data } = await apiClient.get<TeamMember | null>('/team/me')
  return data
}

/** GET /api/v1/team/teams — all teams I admin */
export async function getMyTeams(): Promise<Team[]> {
  const { data } = await apiClient.get<Team[]>('/team/teams')
  return data
}

// ── Team-scope endpoints (shared by both sources) ────────────────────────────

export async function getTeam(source: Source, teamId: number): Promise<Team> {
  const { data } = await apiClient.get<Team>(teamBasePath(source, teamId))
  return data
}

export async function listMembers(
  source: Source,
  teamId: number,
  page = 1,
  pageSize = 20,
  tags?: string[]
): Promise<PaginatedResponse<TeamMember>> {
  const params = new URLSearchParams()
  params.set('page', String(page))
  params.set('page_size', String(pageSize))
  if (tags && tags.length > 0) {
    for (const t of tags) params.append('tag', t)
  }
  const { data } = await apiClient.get<PaginatedResponse<TeamMember>>(
    `${teamBasePath(source, teamId)}/members?${params.toString()}`
  )
  return data
}

export async function getMember(source: Source, teamId: number, memberID: number): Promise<TeamMember> {
  const { data } = await apiClient.get<TeamMember>(`${teamBasePath(source, teamId)}/members/${memberID}`)
  return data
}

/** Create a NEW user and add to team (team_admin source only — sys admin uses adminAddMember). */
export async function createMember(
  teamId: number,
  payload: { email: string; password: string; username?: string }
): Promise<TeamMember> {
  const { data } = await apiClient.post<TeamMember>(`/team/${teamId}/members`, payload)
  return data
}

/** Add an existing user as paying member (admin source only). */
export async function adminAddMember(teamId: number, userID: number): Promise<TeamMember> {
  const { data } = await apiClient.post<TeamMember>(`/admin/teams/${teamId}/members`, {
    user_id: userID,
  })
  return data
}

export async function removeMember(source: Source, teamId: number, memberID: number): Promise<void> {
  await apiClient.delete(`${teamBasePath(source, teamId)}/members/${memberID}`)
}

export async function updateSubQuota(source: Source, teamId: number, memberID: number, subQuota: number): Promise<TeamMember> {
  const { data } = await apiClient.put<TeamMember>(
    `${teamBasePath(source, teamId)}/members/${memberID}/sub-quota`,
    { sub_quota: subQuota }
  )
  return data
}

/** Tags / password / status are team_admin-only ops. */
export async function updateMemberTags(teamId: number, memberID: number, tags: string[]): Promise<TeamMember> {
  const { data } = await apiClient.put<TeamMember>(`/team/${teamId}/members/${memberID}/tags`, { tags })
  return data
}

export async function resetMemberPassword(teamId: number, memberID: number, newPassword: string): Promise<void> {
  await apiClient.put(`/team/${teamId}/members/${memberID}/password`, { new_password: newPassword })
}

export async function setMemberStatus(teamId: number, memberID: number, status: 'active' | 'disabled'): Promise<void> {
  await apiClient.put(`/team/${teamId}/members/${memberID}/status`, { status })
}

export async function listMemberAPIKeys(teamId: number, memberID: number): Promise<unknown[]> {
  const { data } = await apiClient.get<unknown[]>(`/team/${teamId}/members/${memberID}/api-keys`)
  return data
}

export async function listMemberSubscriptions(teamId: number, memberID: number): Promise<TeamSubscription[]> {
  const { data } = await apiClient.get<TeamSubscription[]>(`/team/${teamId}/members/${memberID}/subscriptions`)
  return data
}

// ── Admin role management (team_admins table) ────────────────────────────────

export async function listAdmins(source: Source, teamId: number): Promise<TeamAdmin[]> {
  const { data } = await apiClient.get<TeamAdmin[]>(`${teamBasePath(source, teamId)}/admins`)
  return data
}

export async function addAdmin(source: Source, teamId: number, userID: number): Promise<TeamAdmin> {
  const { data } = await apiClient.post<TeamAdmin>(`${teamBasePath(source, teamId)}/admins`, { user_id: userID })
  return data
}

export async function removeAdmin(source: Source, teamId: number, userID: number): Promise<void> {
  await apiClient.delete(`${teamBasePath(source, teamId)}/admins/${userID}`)
}

// ── Usage / subscriptions / balance / tags ───────────────────────────────────

export interface TeamUsageFilters {
  user_id?: number
  model?: string
  api_key_id?: number
  api_key_name?: string
  group_id?: number
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
  billing_mode?: string
  start_date?: string
  end_date?: string
}

export interface ListUsageParams extends TeamUsageFilters {
  page: number
  page_size: number
}

export async function listUsage(source: Source, teamId: number, params: ListUsageParams): Promise<PaginatedResponse<any>> {
  const { data } = await apiClient.get<PaginatedResponse<any>>(`${teamBasePath(source, teamId)}/usage`, { params })
  return data
}

export interface TeamUsageStats {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_tokens: number
  total_cost: number
  total_actual_cost: number
  total_account_cost?: number
  average_duration_ms: number
}

export async function getUsageStats(
  source: Source,
  teamId: number,
  params: TeamUsageFilters
): Promise<TeamUsageStats> {
  const { data } = await apiClient.get<TeamUsageStats>(`${teamBasePath(source, teamId)}/usage/stats`, { params })
  return data
}

export interface TeamUsageTrendPoint {
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number
  actual_cost: number
}

export async function getUsageTrend(
  source: Source,
  teamId: number,
  params: TeamUsageFilters & { granularity?: 'day' | 'hour' }
): Promise<{ trend: TeamUsageTrendPoint[] }> {
  const { data } = await apiClient.get<{ trend: TeamUsageTrendPoint[] }>(
    `${teamBasePath(source, teamId)}/usage/trend`,
    { params }
  )
  return data
}

export interface TeamModelStat {
  model: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number
  actual_cost: number
  account_cost: number
}

export async function getUsageModelStats(
  source: Source,
  teamId: number,
  params: TeamUsageFilters & { model_source?: 'requested' | 'upstream' | 'mapping' }
): Promise<{ models: TeamModelStat[] }> {
  const { data } = await apiClient.get<{ models: TeamModelStat[] }>(
    `${teamBasePath(source, teamId)}/usage/model-stats`,
    { params }
  )
  return data
}

export interface TeamGroupStat {
  group_id: number
  group_name: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
  account_cost: number
}

export async function getUsageGroupStats(
  source: Source,
  teamId: number,
  params: TeamUsageFilters
): Promise<{ groups: TeamGroupStat[] }> {
  const { data } = await apiClient.get<{ groups: TeamGroupStat[] }>(
    `${teamBasePath(source, teamId)}/usage/group-stats`,
    { params }
  )
  return data
}

export interface TeamEndpointStat {
  endpoint: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
}

export async function getUsageEndpointStats(
  source: Source,
  teamId: number,
  params: TeamUsageFilters
): Promise<{ endpoints: TeamEndpointStat[]; endpoint_paths: TeamEndpointStat[] }> {
  const { data } = await apiClient.get<{ endpoints: TeamEndpointStat[]; endpoint_paths: TeamEndpointStat[] }>(
    `${teamBasePath(source, teamId)}/usage/endpoint-stats`,
    { params }
  )
  return data
}

export interface TeamUserBreakdownItem {
  user_id: number
  email: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
  account_cost: number
}

export interface TeamUserBreakdownParams extends TeamUsageFilters {
  group_id?: number
  model?: string
  model_source?: 'requested' | 'upstream' | 'mapping'
  endpoint?: string
  endpoint_type?: 'inbound' | 'upstream' | 'path'
  limit?: number
}

export async function getUsageUserBreakdown(
  source: Source,
  teamId: number,
  params: TeamUserBreakdownParams
): Promise<{ users: TeamUserBreakdownItem[] }> {
  const { data } = await apiClient.get<{ users: TeamUserBreakdownItem[] }>(
    `${teamBasePath(source, teamId)}/usage/user-breakdown`,
    { params }
  )
  return data
}

export async function listSubscriptions(source: Source, teamId: number, page = 1, pageSize = 20): Promise<PaginatedResponse<TeamSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<TeamSubscription>>(
    `${teamBasePath(source, teamId)}/subscriptions`,
    { params: { page, page_size: pageSize } }
  )
  return data
}

/** Purchase subscription (team_admin only). */
export async function purchaseSubscription(teamId: number, userID: number, planID: number): Promise<unknown> {
  const { data } = await apiClient.post(`/team/${teamId}/subscriptions`, { user_id: userID, plan_id: planID })
  return data
}

export async function listPlans(teamId: number): Promise<SubscriptionPlan[]> {
  const { data } = await apiClient.get<SubscriptionPlan[]>(`/team/${teamId}/plans`)
  return data
}

export async function listBalanceLogs(source: Source, teamId: number, page = 1, pageSize = 20): Promise<PaginatedResponse<TeamBalanceLog>> {
  const { data } = await apiClient.get<PaginatedResponse<TeamBalanceLog>>(
    `${teamBasePath(source, teamId)}/balance`,
    { params: { page, page_size: pageSize } }
  )
  return data
}

export async function updateAvailableTags(source: Source, teamId: number, tags: string[]): Promise<Team> {
  const { data } = await apiClient.put<Team>(`${teamBasePath(source, teamId)}/tags`, { tags })
  return data
}

// ── Sys admin only: teams collection + team-level ops ────────────────────────

export async function adminListTeams(page = 1, pageSize = 20): Promise<PaginatedResponse<Team>> {
  const { data } = await apiClient.get<PaginatedResponse<Team>>('/admin/teams', { params: { page, page_size: pageSize } })
  return data
}

export async function adminCreateTeam(payload: {
  name: string
  initial_admin_user_id: number
  initial_balance?: number
  max_members?: number
  also_add_as_member?: boolean
}): Promise<Team> {
  const { data } = await apiClient.post<Team>('/admin/teams', payload)
  return data
}

export async function adminUpdateTeam(id: number, payload: { name: string; max_members?: number }): Promise<Team> {
  const { data } = await apiClient.put<Team>(`/admin/teams/${id}`, payload)
  return data
}

export async function adminDeleteTeam(id: number): Promise<void> {
  await apiClient.delete(`/admin/teams/${id}`)
}

export async function adminRechargeTeam(id: number, amount: number, note?: string): Promise<Team> {
  const { data } = await apiClient.post<Team>(`/admin/teams/${id}/recharge`, { amount, note })
  return data
}

// ── Aggregate export (some legacy callers use teamAPI.*) ─────────────────────

export const teamAPI = {
  getMyMembership,
  getMyTeams,
  getTeam,
  listMembers,
  getMember,
  createMember,
  adminAddMember,
  removeMember,
  updateSubQuota,
  updateMemberTags,
  resetMemberPassword,
  setMemberStatus,
  listMemberAPIKeys,
  listMemberSubscriptions,
  listAdmins,
  addAdmin,
  removeAdmin,
  listUsage,
  getUsageStats,
  getUsageTrend,
  getUsageModelStats,
  getUsageGroupStats,
  getUsageEndpointStats,
  getUsageUserBreakdown,
  listSubscriptions,
  purchaseSubscription,
  listPlans,
  listBalanceLogs,
  updateAvailableTags,
  adminListTeams,
  adminCreateTeam,
  adminUpdateTeam,
  adminDeleteTeam,
  adminRechargeTeam,
}
