<template>
  <TablePageLayout>
    <template #actions>
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

  <BaseDialog :show="showPurchaseDialog" :title="t('team.subscriptions.purchase')" width="normal" @close="showPurchaseDialog = false">
    <div class="space-y-4">
      <div>
        <label class="flex items-center gap-2 rounded-md border border-primary-200 bg-primary-50 px-3 py-2 dark:border-primary-700 dark:bg-primary-900/20">
          <input
            type="checkbox"
            v-model="purchaseForm.allMembers"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
          <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
            {{ t('team.subscriptions.forAllMembers', { count: members.length }) }}
          </span>
        </label>
      </div>

      <div v-if="!purchaseForm.allMembers">
        <div class="mb-2 flex items-center justify-between">
          <label class="input-label !mb-0">{{ t('team.subscriptions.selectMember') }}</label>
          <div class="flex items-center gap-3 text-xs">
            <span class="text-gray-500 dark:text-gray-400">{{ t('team.subscriptions.selectedCount', { count: purchaseForm.userIds.length }) }}</span>
            <button type="button" class="text-gray-500 hover:underline" @click="clearMembers">{{ t('team.subscriptions.clearAll') }}</button>
          </div>
        </div>
        <p v-if="members.length === 0" class="rounded-md border border-amber-200 bg-amber-50 p-2 text-sm text-amber-700 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
          {{ t('team.subscriptions.noMembers') }}
        </p>
        <div v-else class="max-h-64 overflow-y-auto rounded-md border border-gray-200 dark:border-dark-700">
          <label
            v-for="m in members"
            :key="m.user_id"
            class="flex items-center gap-2 border-b border-gray-100 px-3 py-2 last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800"
          >
            <input
              type="checkbox"
              :checked="purchaseForm.userIds.includes(m.user_id)"
              @change="toggleMember(m.user_id)"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ m.user?.email ?? m.user_id }}</span>
          </label>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('team.subscriptions.selectPlan') }}</label>
        <Select v-model="purchaseForm.planId" :options="planOptions" :searchable="true" :placeholder="t('team.subscriptions.selectPlan')" />
        <p v-if="plans.length === 0" class="mt-2 text-sm text-amber-600">{{ t('team.subscriptions.noPlans') }}</p>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('team.subscriptions.purchaseHint') }}</p>
      <div v-if="selectedPlan && team && targetCount > 0" class="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-700 dark:bg-amber-900/20">
        <p class="text-amber-700 dark:text-amber-400">
          {{ t('team.subscriptions.willDeduct', { amount: totalCost.toFixed(2), price: selectedPlan.price.toFixed(2), count: targetCount, balance: Number(team.balance).toFixed(2) }) }}
        </p>
        <p v-if="totalCost > Number(team.balance)" class="mt-1 font-medium text-red-600">
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
const purchaseForm = ref<{ allMembers: boolean; userIds: number[]; planId: number | null }>({
  allMembers: false,
  userIds: [],
  planId: null,
})

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('team.members.email'), sortable: false },
  { key: 'group', label: t('team.members.subscriptionStatus'), sortable: false },
  { key: 'status', label: t('common.status'), sortable: false },
  { key: 'expires_at', label: t('team.subscriptions.expiresAt'), sortable: false },
  { key: 'usage', label: t('team.subscriptions.usage'), sortable: false },
])

const planOptions = computed(() =>
  plans.value.map(p => ({ value: p.id, label: `${p.name} — $${p.price.toFixed(2)} / ${p.validity_days}d` }))
)
const selectedPlan = computed(() => plans.value.find(p => p.id === purchaseForm.value.planId) ?? null)
const targetCount = computed(() => (purchaseForm.value.allMembers ? members.value.length : purchaseForm.value.userIds.length))
const totalCost = computed(() => (selectedPlan.value ? selectedPlan.value.price * targetCount.value : 0))
const canPurchase = computed(() => {
  if (targetCount.value === 0 || !purchaseForm.value.planId) return false
  if (!selectedPlan.value || !team.value) return false
  return totalCost.value <= Number(team.value.balance)
})

function toggleMember(userId: number) {
  const idx = purchaseForm.value.userIds.indexOf(userId)
  if (idx === -1) purchaseForm.value.userIds.push(userId)
  else purchaseForm.value.userIds.splice(idx, 1)
}
function clearMembers() {
  purchaseForm.value.userIds = []
}

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
  purchaseForm.value = { allMembers: false, userIds: [], planId: null }
  try {
    const [membersRes, plansRes, teamRes] = await Promise.all([
      teamAPI.listMembers(props.source, props.teamId, 1, 500),
      teamAPI.listPlans(props.source, props.teamId),
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
    const target = purchaseForm.value.allMembers
      ? ({ allMembers: true } as const)
      : ({ userIDs: purchaseForm.value.userIds } as const)
    const result = await teamAPI.purchaseSubscription(props.source, props.teamId, target, purchaseForm.value.planId!)
    if (result.stopped_reason === 'insufficient_balance') {
      appStore.showError(t('team.subscriptions.partialInsufficient', { succeeded: result.succeeded, total: result.total }))
    } else {
      appStore.showSuccess(t('team.subscriptions.purchasedCount', { count: result.succeeded }))
    }
    showPurchaseDialog.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to purchase')
  } finally { submitting.value = false }
}

watch(() => props.teamId, () => { page.value = 1; load() }, { immediate: true })
</script>
