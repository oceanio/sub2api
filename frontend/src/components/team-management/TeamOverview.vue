<template>
  <div class="space-y-6">
    <div v-if="team" class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <div class="card p-5">
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('team.overview.balance') }}</p>
        <p class="mt-1 text-2xl font-bold text-primary-600">${{ Number(team.balance).toFixed(4) }}</p>
      </div>
      <div class="card p-5">
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('team.overview.memberCount') }}</p>
        <p class="mt-1 text-2xl font-bold">
          {{ team.member_count ?? 0 }}<span class="ml-1 text-base font-normal text-gray-400">/ {{ team.max_members > 0 ? team.max_members : '∞' }}</span>
        </p>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <h3 class="card-title">{{ t('team.overview.recentLogs') }}</h3>
      </div>
      <div class="divide-y divide-gray-100 dark:divide-dark-700">
        <div v-for="log in recentLogs" :key="log.id" class="flex items-center justify-between px-6 py-3 text-sm">
          <div>
            <span class="font-medium">{{ t(`team.balance.type.${log.type}`) }}</span>
            <span v-if="log.note" class="ml-2 text-gray-500">{{ log.note }}</span>
          </div>
          <div class="flex items-center gap-4">
            <span :class="log.amount > 0 ? 'text-green-600' : 'text-red-500'" class="font-mono font-medium">
              {{ log.amount > 0 ? '+' : '' }}{{ log.amount.toFixed(4) }} USD
            </span>
            <span class="text-gray-400">{{ formatDateTime(log.created_at) }}</span>
          </div>
        </div>
        <div v-if="recentLogs.length === 0" class="px-6 py-8 text-center text-sm text-gray-400">
          {{ t('team.balance.noLogs') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { teamAPI, type Team, type TeamBalanceLog } from '@/api/team'
import { formatDateTime } from '@/utils/format'

interface Props {
  teamId: number
  source: 'admin' | 'team_admin'
}

const props = defineProps<Props>()
const { t } = useI18n()

const team = ref<Team | null>(null)
const recentLogs = ref<TeamBalanceLog[]>([])

async function load() {
  try {
    team.value = await teamAPI.getTeam(props.source, props.teamId)
  } catch {
    team.value = null
  }
  try {
    const res = await teamAPI.listBalanceLogs(props.source, props.teamId, 1, 10)
    recentLogs.value = (res as any).items ?? []
  } catch {
    recentLogs.value = []
  }
}

watch(() => props.teamId, load, { immediate: true })
</script>
