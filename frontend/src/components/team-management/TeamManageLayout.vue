<template>
  <AppLayout>
    <div class="space-y-4 p-6">
      <div class="flex flex-wrap items-center gap-3">
        <button
          v-if="source === 'admin'"
          class="btn btn-secondary btn-sm"
          @click="$router.push('/admin/teams')"
        >
          ← {{ t('common.back') }}
        </button>
        <TeamSelector :model-value="teamId" :source="source" @update:modelValue="onTeamChange" />
        <span v-if="team" class="text-sm text-gray-500">
          {{ t('team.overview.balance') }}：
          <span class="font-mono font-medium text-emerald-600">${{ Number(team.balance).toFixed(2) }}</span>
        </span>
        <button
          v-if="team"
          type="button"
          class="text-gray-400 hover:text-emerald-600 disabled:opacity-50"
          :disabled="refreshing"
          :title="t('common.refresh')"
          @click="onRefreshBalance"
        >
          <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
        </button>
      </div>

      <div class="flex flex-wrap gap-2 border-b border-gray-200 dark:border-dark-700">
        <router-link
          v-for="tab in tabs"
          :key="tab.path"
          :to="tab.path"
          class="px-4 py-2 text-sm border-b-2"
          :class="route.path === tab.path
            ? 'border-primary-600 text-primary-600 font-medium'
            : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-dark-200'"
        >{{ tab.label }}</router-link>
      </div>

      <slot />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, provide, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useTeamStore } from '@/stores/team'
import { teamAPI, type Team } from '@/api/team'
import AppLayout from '@/components/layout/AppLayout.vue'
import TeamSelector from '@/components/team-management/TeamSelector.vue'
import Icon from '@/components/icons/Icon.vue'

type Source = 'admin' | 'team_admin'

interface Props {
  teamId: number
  source: Source
}
const props = defineProps<Props>()

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const teamStore = useTeamStore()

// Source-aware URL base (kept in one place so tabs / selector / cache logic
// all agree).
const basePath = computed(() => (props.source === 'admin' ? '/admin/teams' : '/team'))

function onTeamChange(newID: number) {
  const re = props.source === 'admin' ? /^\/admin\/teams\/\d+/ : /^\/team\/\d+/
  const sub = route.path.replace(re, '') // '', '/members', '/usage', …
  router.push(`${basePath.value}/${newID}${sub}`)
}

const team = ref<Team | null>(null)
const refreshing = ref(false)

const tabs = computed(() => {
  const base = `${basePath.value}/${props.teamId}`
  // Hide the Subscriptions tab for team_admin when the team has the
  // subscriptions feature switched off by sys admin. Strict `=== false` so
  // that older API responses missing the field (or pre-load) keep the tab
  // visible — matches the backend default of TRUE.
  const subsHidden = props.source === 'team_admin' && team.value?.subscriptions_enabled === false
  const list = [
    { label: t('team.nav.members'), path: `${base}/members` },
    { label: t('team.nav.usage'), path: `${base}/usage` },
  ]
  if (!subsHidden) list.push({ label: t('team.nav.subscriptions'), path: `${base}/subscriptions` })
  list.push({ label: t('team.nav.balance'), path: `${base}/balance` })
  // 分组管理 tab 仅 sys admin 可见。team_admin 路径既无路由、也无后端 endpoint。
  if (props.source === 'admin') {
    list.push({ label: t('team.nav.groupRates'), path: `${base}/group-rates` })
  }
  return list
})

async function loadTeam(force = false) {
  // Admin caches team detail across tab switches via teamStore.adminTeamCache.
  // team_admin reads team detail from teamStore.managedTeams which TeamSelector
  // already keeps fresh, so we just look it up there to avoid a duplicate fetch.
  if (props.source === 'team_admin') {
    const cached = teamStore.managedTeams.find((t) => t.id === props.teamId)
    if (cached) {
      team.value = cached
      return
    }
  }
  if (!force && props.source === 'admin') {
    const cached = teamStore.getCachedAdminTeam(props.teamId)
    if (cached) {
      team.value = cached
      return
    }
  }
  try {
    const fresh = await teamAPI.getTeam(props.source, props.teamId)
    team.value = fresh
    if (props.source === 'admin') {
      teamStore.cacheAdminTeam(fresh)
    }
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

// Children (e.g. TeamSubscriptionsList after purchase/refund) inject this to
// force the header balance to refresh.
provide('refreshTeam', () => {
  if (props.source === 'admin') {
    teamStore.invalidateAdminTeam(props.teamId)
  }
  loadTeam(true)
})

async function onRefreshBalance() {
  if (refreshing.value) return
  refreshing.value = true
  try {
    if (props.source === 'admin') {
      teamStore.invalidateAdminTeam(props.teamId)
    }
    await loadTeam(true)
  } finally {
    refreshing.value = false
  }
}

watch(() => props.teamId, (id) => {
  if (!id) return
  if (props.source === 'admin') {
    teamStore.setAdminLastTeam(id)
  } else {
    teamStore.setActiveTeam(id)
  }
  loadTeam()
}, { immediate: true })
</script>
