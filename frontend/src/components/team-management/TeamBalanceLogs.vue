<template>
  <div class="space-y-4">
    <div v-if="team" class="card p-5">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('team.balance.currentBalance') }}</p>
      <p class="mt-1 text-3xl font-bold text-emerald-600 dark:text-emerald-400">
        ${{ Number(team.balance).toFixed(4) }}
      </p>
    </div>

    <div class="card">
      <div class="card-header px-6 py-4">
        <h3 class="card-title">{{ t('team.balance.title') }}</h3>
      </div>
      <DataTable :columns="columns" :data="logs" :loading="loading">
        <template #cell-type="{ value }">{{ t(`team.balance.type.${value}`) }}</template>
        <template #cell-amount="{ value }">
          <span :class="Number(value) > 0 ? 'text-green-600' : 'text-red-500'" class="font-mono">
            {{ Number(value) > 0 ? '+' : '' }}{{ Number(value).toFixed(4) }}
          </span>
        </template>
        <template #cell-operator="{ row }">
          <span>{{ row.operator?.email ?? `#${row.operator_id}` }}</span>
        </template>
        <template #cell-target_user="{ row }">
          <span v-if="row.target_user">{{ row.target_user.email }}</span>
          <span v-else-if="row.target_user_id">#{{ row.target_user_id }}</span>
          <span v-else class="text-gray-400">—</span>
        </template>
        <template #cell-created_at="{ value }">{{ formatDateTime(value) }}</template>
      </DataTable>
      <Pagination :total="total" :page="page" :page-size="pageSize" @change="onPageChange" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { teamAPI, type Team, type TeamBalanceLog } from '@/api/team'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'

interface Props {
  teamId: number
  source: 'admin' | 'team_admin'
}
const props = defineProps<Props>()

const { t } = useI18n()
const team = ref<Team | null>(null)
const logs = ref<TeamBalanceLog[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const columns = computed<Column[]>(() => [
  { key: 'type', label: t('common.status'), sortable: false },
  { key: 'amount', label: t('team.balance.amount'), sortable: false },
  { key: 'operator', label: t('team.balance.operator'), sortable: false },
  { key: 'target_user', label: t('team.balance.targetUser'), sortable: false },
  { key: 'note', label: t('team.balance.note'), sortable: false },
  { key: 'created_at', label: t('team.balance.date'), sortable: false },
])

async function load() {
  loading.value = true
  try {
    const [t1, t2] = await Promise.all([
      teamAPI.getTeam(props.source, props.teamId),
      teamAPI.listBalanceLogs(props.source, props.teamId, page.value, pageSize.value),
    ])
    team.value = t1
    logs.value = (t2 as any).items ?? []
    total.value = (t2 as any).total ?? 0
  } finally { loading.value = false }
}

function onPageChange(p: number) { page.value = p; load() }

watch(() => props.teamId, () => { page.value = 1; load() }, { immediate: true })
</script>
