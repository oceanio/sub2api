<template>
  <AppLayout>
    <div class="space-y-4 p-6">
      <div class="mb-2">
        <h1 class="text-xl font-bold">{{ t('team.adminTeams.teamList') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('team.adminTeams.teamListHint') }}</p>
      </div>

      <div v-if="loading" class="card p-12 text-center text-gray-400">{{ t('common.loading') }}</div>
      <div v-else-if="teams.length === 0" class="card p-12 text-center text-gray-400">
        {{ t('team.adminTeams.noTeams') }}
      </div>
      <div v-else class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <div
          v-for="team in teams"
          :key="team.id"
          class="card cursor-pointer p-5 transition-shadow hover:shadow-lg"
          @click="open(team.id)"
        >
          <div class="flex items-start justify-between gap-2">
            <h3 class="text-lg font-semibold">{{ team.name }}</h3>
            <Icon name="chevronRight" size="sm" class="text-gray-400" />
          </div>
          <div class="mt-3 space-y-1 text-sm">
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-dark-400">{{ t('team.overview.balance') }}</span>
              <span class="font-mono text-emerald-600">${{ Number(team.balance).toFixed(2) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-dark-400">{{ t('team.overview.memberCount') }}</span>
              <span class="font-mono">{{ team.member_count ?? 0 }} / {{ team.max_members > 0 ? team.max_members : '∞' }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { teamAPI, type Team } from '@/api/team'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const teams = ref<Team[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    teams.value = await teamAPI.getMyTeams()
    // Single-team team_admin: the list is a no-op; jump straight to the team's
    // detail page so the user doesn't pay one click to look at one card.
    if (teams.value.length === 1) {
      router.replace(`/team/${teams.value[0].id}`)
    }
  } finally { loading.value = false }
}

function open(id: number) {
  router.push(`/team/${id}`)
}

onMounted(load)
</script>
