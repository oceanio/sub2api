import type { RouteRecordRaw } from 'vue-router'

/**
 * Team-feature route table. Kept in its own file so the team feature's
 * footprint inside `router/index.ts` stays a single `...teamRoutes` spread.
 *
 * URL split into two trees:
 *   - /admin/teams/*    sys admin
 *   - /team/:teamId/*   team_admin (the same user can admin multiple teams,
 *                       hence the team id is part of the URL)
 */
export const teamRoutes: RouteRecordRaw[] = [
  // ── sys admin ──────────────────────────────────────────────────────────────
  {
    path: '/admin/teams',
    name: 'AdminTeams',
    component: () => import('@/views/admin/TeamsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.adminTeams.title' },
  },
  {
    path: '/admin/teams/:id',
    name: 'AdminTeamOverview',
    component: () => import('@/views/admin/TeamManageOverview.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.nav.overview' },
  },
  {
    path: '/admin/teams/:id/members',
    name: 'AdminTeamMembers',
    component: () => import('@/views/admin/TeamManageMembers.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.nav.members' },
  },
  {
    path: '/admin/teams/:id/usage',
    name: 'AdminTeamUsage',
    component: () => import('@/views/admin/TeamManageUsage.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.nav.usage' },
  },
  {
    path: '/admin/teams/:id/subscriptions',
    name: 'AdminTeamSubscriptions',
    component: () => import('@/views/admin/TeamManageSubscriptions.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.nav.subscriptions' },
  },
  {
    path: '/admin/teams/:id/balance',
    name: 'AdminTeamBalance',
    component: () => import('@/views/admin/TeamManageBalance.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.nav.balance' },
  },
  {
    path: '/admin/teams/:id/tags',
    name: 'AdminTeamTags',
    component: () => import('@/views/admin/TeamManageTags.vue'),
    meta: { requiresAuth: true, requiresAdmin: true, titleKey: 'team.nav.tags' },
  },
  // ── team_admin (parametric URL: one user can admin multiple teams) ────────
  {
    path: '/team',
    name: 'TeamList',
    component: () => import('@/views/team/TeamListView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.adminTeams.teamList' },
  },
  {
    path: '/team/:teamId',
    name: 'TeamOverview',
    component: () => import('@/views/team/TeamOverviewView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.nav.overview' },
  },
  {
    path: '/team/:teamId/members',
    name: 'TeamMembers',
    component: () => import('@/views/team/TeamMembersView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.nav.members' },
  },
  {
    path: '/team/:teamId/usage',
    name: 'TeamUsage',
    component: () => import('@/views/team/TeamUsageView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.nav.usage' },
  },
  {
    path: '/team/:teamId/balance',
    name: 'TeamBalance',
    component: () => import('@/views/team/TeamBalanceView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.nav.balance' },
  },
  {
    path: '/team/:teamId/subscriptions',
    name: 'TeamSubscriptions',
    component: () => import('@/views/team/TeamSubscriptionsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.nav.subscriptions' },
  },
  {
    path: '/team/:teamId/tags',
    name: 'TeamTags',
    component: () => import('@/views/team/TeamTagsView.vue'),
    meta: { requiresAuth: true, requiresAdmin: false, titleKey: 'team.nav.tags' },
  },
]
