import { getUserBreakdown } from '@/api/admin/dashboard'
import { teamAPI } from '@/api/team'
import type { UserBreakdownItem } from '@/types'

/**
 * Shared drill-down fetcher used by the distribution charts
 * (Model / Group / Endpoint). Picks the team-scoped endpoint when the chart is
 * embedded in the team usage page, otherwise hits the admin endpoint.
 *
 * Returns [] on error so charts can render the "no breakdown" state without
 * surfacing the failure (team_admin users hitting the admin endpoint cleanly
 * fall through this path).
 */
export interface BreakdownScopeProps {
  breakdownScope?: 'admin' | 'team'
  breakdownSource?: 'admin' | 'team_admin'
  teamId?: number
}

export async function fetchUserBreakdown(
  scope: BreakdownScopeProps,
  params: Record<string, any>
): Promise<UserBreakdownItem[]> {
  try {
    if (scope.breakdownScope === 'team' && scope.teamId && scope.breakdownSource) {
      const res = await teamAPI.getUsageUserBreakdown(scope.breakdownSource, scope.teamId, params as any)
      return (res.users as UserBreakdownItem[]) || []
    }
    const res = await getUserBreakdown(params as any)
    return res.users || []
  } catch {
    return []
  }
}
