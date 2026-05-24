<template>
  <TablePageLayout>
    <template v-if="source === 'team_admin'" #actions>
      <div class="flex justify-end gap-3">
        <button class="btn btn-primary" @click="openPurchase">
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('team.subscriptions.purchase') }}
        </button>
      </div>
    </template>

    <template #table>
      <DataTable :columns="columns" :data="subscriptions" :loading="loading">
        <template #cell-user="{ row }">
          <span>{{ row.user?.email ?? row.user_id }}</span>
        </template>
        <template #cell-group="{ row }">
          <span>{{ row.group?.name ?? row.group_id }}</span>
          <span v-if="row.group?.platform" class="ml-1 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-700">{{ row.group.platform }}</span>
        </template>
        <template #cell-status="{ row }">
          <span :class="row.status === 'active' && !isExpired(row) ? 'text-green-600' : 'text-gray-400'">
            {{ row.status === 'active' && !isExpired(row) ? t('common.active') : t('common.disabled') }}
          </span>
        </template>
        <template #cell-expires_at="{ value }">{{ formatDateTime(value) }}</template>
        <template #cell-usage="{ row }">
          <span class="font-mono text-xs">
            <span>{{ t('team.subscriptions.usageDaily') }} ${{ Number(row.daily_usage_usd ?? 0).toFixed(2) }}</span>
            <span class="ml-2">{{ t('team.subscriptions.usageWeekly') }} ${{ Number(row.weekly_usage_usd ?? 0).toFixed(2) }}</span>
            <span class="ml-2">{{ t('team.subscriptions.usageMonthly') }} ${{ Number(row.monthly_usage_usd ?? 0).toFixed(2) }}</span>
          </span>
        </template>
      </DataTable>
      <Pagination :total="total" :page="page" :page-size="pageSize" @change="onPageChange" />
    </template>
  </TablePageLayout>

  <BaseDialog v-if="source === 'team_admin'" :show="showPurchaseDialog" :title="t('team.subscriptions.purchase')" width="normal" @close="showPurchaseDialog = false">
    <div class="space-y-4">
      <div>
        <label class="input-label">{{ t('team.subscriptions.selectMember') }}</label>
        <Select v-model="purchaseForm.userId" :options="memberOptions" :searchable="true" :placeholder="t('team.subscriptions.selectMember')" />
      </div>
      <div>
        <label class="input-label">{{ t('team.subscriptions.selectPlan') }}</label>
        <Select v-model="purchaseForm.planId" :options="planOptions" :searchable="true" :placeholder="t('team.subscriptions.selectPlan')" />
        <p v-if="plans.length === 0" class="mt-2 text-sm text-amber-600">{{ t('team.subscriptions.noPlans') }}</p>
      </div>
      <div v-if="selectedPlan && team" class="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-700 dark:bg-amber-900/20">
        <p class="text-amber-700 dark:text-amber-400">
          {{ t('team.subscriptions.willDeduct', { amount: selectedPlan.price.toFixed(2), balance: Number(team.balance).toFixed(2) }) }}
        </p>
        <p v-if="selectedPlan.price > Number(team.balance)" class="mt-1 font-medium text-red-600">
          {{ t('team.subscriptions.insufficientBalance') }}
        </p>
      </div>
    </div>
    <template #footer>
      <button class="btn btn-secondary" @click="showPurchaseDialog = false">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="!canPurchase || submitting" @click="handlePurchase">
        {{ submitting ? t('common.saving') : t('team.subscriptions.confirmPurchase') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { teamAPI, type TeamSubscription, type TeamMember, type Team, type SubscriptionPlan } from '@/api/team'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'

interface Props {
  teamId: number
  source: 'admin' | 'team_admin'
}
const props = defineProps<Props>()

const { t } = useI18n()
const appStore = useAppStore()

const subscriptions = ref<TeamSubscription[]>([])
const members = ref<TeamMember[]>([])
const plans = ref<SubscriptionPlan[]>([])
const team = ref<Team | null>(null)

const loading = ref(false)
const submitting = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const showPurchaseDialog = ref(false)
const purchaseForm = ref<{ userId: number | null; planId: number | null }>({ userId: null, planId: null })

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('team.members.email'), sortable: false },
  { key: 'group', label: t('team.members.subscriptionStatus'), sortable: false },
  { key: 'status', label: t('common.status'), sortable: false },
  { key: 'expires_at', label: t('team.subscriptions.expiresAt'), sortable: false },
  { key: 'usage', label: t('team.subscriptions.usage'), sortable: false },
])

const memberOptions = computed(() =>
  members.value.map(m => ({ value: m.user_id, label: m.user?.email ?? String(m.user_id) }))
)
const planOptions = computed(() =>
  plans.value.map(p => ({ value: p.id, label: `${p.name} — $${p.price.toFixed(2)} / ${p.validity_days}d` }))
)
const selectedPlan = computed(() => plans.value.find(p => p.id === purchaseForm.value.planId) ?? null)
const canPurchase = computed(() => {
  if (!purchaseForm.value.userId || !purchaseForm.value.planId) return false
  if (!selectedPlan.value || !team.value) return false
  return selectedPlan.value.price <= Number(team.value.balance)
})

function isExpired(row: TeamSubscription) {
  return new Date(row.expires_at).getTime() < Date.now()
}

async function load() {
  loading.value = true
  try {
    const res = await teamAPI.listSubscriptions(props.source, props.teamId, page.value, pageSize.value)
    subscriptions.value = (res as any).items ?? []
    total.value = (res as any).total ?? 0
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to load subscriptions')
  } finally { loading.value = false }
}

function onPageChange(p: number) { page.value = p; load() }

async function openPurchase() {
  purchaseForm.value = { userId: null, planId: null }
  try {
    const [membersRes, plansRes, teamRes] = await Promise.all([
      teamAPI.listMembers(props.source, props.teamId, 1, 500),
      teamAPI.listPlans(props.teamId),
      teamAPI.getTeam(props.source, props.teamId),
    ])
    members.value = (membersRes as any).items ?? []
    plans.value = plansRes ?? []
    team.value = teamRes
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to load purchase options')
    return
  }
  showPurchaseDialog.value = true
}

async function handlePurchase() {
  if (!canPurchase.value) return
  submitting.value = true
  try {
    await teamAPI.purchaseSubscription(props.teamId, purchaseForm.value.userId!, purchaseForm.value.planId!)
    appStore.showSuccess(t('team.subscriptions.purchased'))
    showPurchaseDialog.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to purchase')
  } finally { submitting.value = false }
}

watch(() => props.teamId, () => { page.value = 1; load() }, { immediate: true })
</script>
