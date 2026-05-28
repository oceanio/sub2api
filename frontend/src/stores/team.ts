import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { teamAPI, type Team, type TeamMember } from '@/api/team'

const ACTIVE_TEAM_KEY = 'team:activeTeamId'
const ADMIN_LAST_TEAM_KEY = 'team:adminLastTeamId'

export const useTeamStore = defineStore('team', () => {
  const managedTeams = ref<Team[]>([])
  const myMembership = ref<TeamMember | null>(null)
  const activeTeamId = ref<number | null>(null)
  // Sys admin's last-viewed team ID (sidebar fallback). Independent of team_admin's activeTeamId.
  const adminLastTeamId = ref<number | null>(readStoredAdminLastTeamId())
  // Lightweight per-team detail cache used by sys admin tab pages so that
  // switching tabs (overview → members → usage → ...) doesn't refetch the team.
  // Keyed by team id; cleared on reset() / manual refresh.
  const adminTeamCache = ref<Map<number, Team>>(new Map())
  const loaded = ref(false)

  const isTeamAdmin = computed(() => managedTeams.value.length > 0)
  const activeTeam = computed<Team | null>(() => {
    if (activeTeamId.value == null) return null
    return managedTeams.value.find(t => t.id === activeTeamId.value) ?? null
  })

  async function load() {
    // /team/me works for any authenticated user — cheapest way to learn membership.
    try {
      myMembership.value = await teamAPI.getMyMembership()
    } catch {
      myMembership.value = null
    }
    // Load my managed teams (works for any user; empty list for non-admins).
    try {
      managedTeams.value = await teamAPI.getMyTeams()
    } catch {
      managedTeams.value = []
    }
    // Restore active team from localStorage if it's still in the list, else first one.
    const stored = readStoredActiveTeamId()
    if (stored != null && managedTeams.value.some(t => t.id === stored)) {
      activeTeamId.value = stored
    } else if (managedTeams.value.length > 0) {
      activeTeamId.value = managedTeams.value[0].id
    } else {
      activeTeamId.value = null
    }
    loaded.value = true
  }

  function setActiveTeam(id: number) {
    if (managedTeams.value.some(t => t.id === id)) {
      activeTeamId.value = id
    }
  }

  // Track sys admin's last viewed team (no membership check; admin can see all).
  function setAdminLastTeam(id: number) {
    adminLastTeamId.value = id
  }

  function cacheAdminTeam(t: Team) {
    adminTeamCache.value.set(t.id, t)
  }

  function getCachedAdminTeam(id: number): Team | null {
    return adminTeamCache.value.get(id) ?? null
  }

  function invalidateAdminTeam(id: number) {
    adminTeamCache.value.delete(id)
  }

  function reset() {
    managedTeams.value = []
    myMembership.value = null
    activeTeamId.value = null
    adminTeamCache.value.clear()
    loaded.value = false
  }

  watch(activeTeamId, (id) => {
    if (id == null) {
      localStorage.removeItem(ACTIVE_TEAM_KEY)
    } else {
      localStorage.setItem(ACTIVE_TEAM_KEY, String(id))
    }
  })

  watch(adminLastTeamId, (id) => {
    if (id == null) {
      localStorage.removeItem(ADMIN_LAST_TEAM_KEY)
    } else {
      localStorage.setItem(ADMIN_LAST_TEAM_KEY, String(id))
    }
  })

  return {
    managedTeams,
    myMembership,
    activeTeamId,
    activeTeam,
    adminLastTeamId,
    isTeamAdmin,
    loaded,
    load,
    setActiveTeam,
    setAdminLastTeam,
    cacheAdminTeam,
    getCachedAdminTeam,
    invalidateAdminTeam,
    reset,
  }
})

function readStoredAdminLastTeamId(): number | null {
  const raw = localStorage.getItem(ADMIN_LAST_TEAM_KEY)
  if (!raw) return null
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : null
}

function readStoredActiveTeamId(): number | null {
  const raw = localStorage.getItem(ACTIVE_TEAM_KEY)
  if (!raw) return null
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : null
}
