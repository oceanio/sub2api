<template>
  <AppLayout>
    <div class="space-y-4 p-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
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
        </div>
        <div v-if="team && source === 'admin'" class="flex gap-2">
          <button class="btn btn-secondary btn-sm" @click="showEditDialog = true">{{ t('common.edit') }}</button>
        </div>
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

    <BaseDialog v-if="source === 'admin'" :show="showEditDialog" :title="t('common.edit')" width="narrow" @close="showEditDialog = false">
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('team.adminTeams.teamName') }}</label>
          <input v-model="editForm.name" type="text" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('team.adminTeams.maxMembers') }}</label>
          <input v-model.number="editForm.maxMembers" type="number" min="0" class="input" />
          <p class="mt-1 text-xs text-gray-500">{{ t('team.adminTeams.maxMembersHint') }}</p>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showEditDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="handleEdit">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useTeamStore } from '@/stores/team'
import { teamAPI, type Team } from '@/api/team'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TeamSelector from '@/components/team-management/TeamSelector.vue'

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
const showEditDialog = ref(false)
const editForm = ref({ name: '', maxMembers: 0 })
const saving = ref(false)

const tabs = computed(() => {
  const base = `${basePath.value}/${props.teamId}`
  return [
    { label: t('team.nav.overview'), path: base },
    { label: t('team.nav.members'), path: `${base}/members` },
    { label: t('team.nav.usage'), path: `${base}/usage` },
    { label: t('team.nav.subscriptions'), path: `${base}/subscriptions` },
    { label: t('team.nav.balance'), path: `${base}/balance` },
    { label: t('team.nav.tags'), path: `${base}/tags` },
  ]
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
      editForm.value = { name: cached.name, maxMembers: cached.max_members ?? 0 }
      return
    }
  }
  try {
    const fresh = await teamAPI.getTeam(props.source, props.teamId)
    team.value = fresh
    if (props.source === 'admin') {
      editForm.value = { name: fresh.name, maxMembers: fresh.max_members ?? 0 }
      teamStore.cacheAdminTeam(fresh)
    }
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

async function handleEdit() {
  if (props.source !== 'admin') return
  saving.value = true
  try {
    await teamAPI.adminUpdateTeam(props.teamId, { name: editForm.value.name, max_members: editForm.value.maxMembers })
    appStore.showSuccess(t('common.saved'))
    showEditDialog.value = false
    teamStore.invalidateAdminTeam(props.teamId)
    loadTeam(true)
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
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
