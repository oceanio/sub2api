<template>
  <TablePageLayout>
    <template #filters>
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-64">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('team.subscriptions.searchPlaceholder')"
              class="input pl-10"
              @input="onSearchInput"
            />
          </div>
          <div class="w-full sm:w-32">
            <Select
              v-model="filterStatus"
              :options="statusOptions"
              :placeholder="t('admin.subscriptions.allStatus')"
              @change="onFilterChange"
            />
          </div>
          <div class="w-full sm:w-44">
            <Select
              v-model="filterGroupId"
              :options="groupOptions"
              :placeholder="t('admin.subscriptions.allGroups')"
              searchable
              @change="onFilterChange"
            />
          </div>
        </div>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <button class="btn btn-secondary px-2 md:px-3" :disabled="loading" :title="t('common.refresh')" @click="load">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <div class="relative" ref="columnDropdownRef">
            <button
              class="btn btn-secondary px-2 md:px-3"
              :title="t('admin.users.columnSettings')"
              @click="showColumnDropdown = !showColumnDropdown"
            >
              <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
              </svg>
              <span class="hidden md:inline">{{ t('admin.users.columnSettings') }}</span>
            </button>
            <div
              v-if="showColumnDropdown"
              class="absolute right-0 top-full z-50 mt-1 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
            >
              <button
                v-for="col in toggleableColumns"
                :key="col.key"
                class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
                @click="toggleColumn(col.key)"
              >
                <span>{{ col.label }}</span>
                <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" :stroke-width="2" />
              </button>
            </div>
          </div>
          <button class="btn btn-primary" @click="openPurchase">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('team.subscriptions.purchase') }}
          </button>
        </div>
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
        <template #cell-status="{ value }">
          <span
            :class="[
              'badge',
              value === 'active' ? 'badge-success' : value === 'expired' ? 'badge-warning' : 'badge-danger',
            ]"
          >
            {{ t(`admin.subscriptions.status.${value}`) }}
          </span>
        </template>
        <template #cell-expires_at="{ value }">
          <div v-if="value">
            <span class="text-sm" :class="isExpiringSoon(value) ? 'text-orange-600 dark:text-orange-400' : 'text-gray-700 dark:text-gray-300'">
              {{ formatDateOnly(value) }}
            </span>
            <div v-if="getDaysRemaining(value) !== null" class="text-xs text-gray-500">
              {{ getDaysRemaining(value) }} {{ t('admin.subscriptions.daysRemaining') }}
            </div>
          </div>
          <span v-else class="text-sm text-gray-500">{{ t('admin.subscriptions.noExpiration') }}</span>
        </template>
        <template #cell-usage="{ row }">
          <div class="min-w-[260px] space-y-2">
            <div v-if="row.group?.daily_limit_usd" class="space-y-0.5">
              <div class="flex items-center gap-2 text-xs">
                <span class="w-8 shrink-0 text-gray-500">{{ t('admin.subscriptions.daily') }}</span>
                <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                  <div class="h-1.5 rounded-full transition-all" :class="getProgressClass(row.daily_usage_usd, row.group?.daily_limit_usd)" :style="{ width: getProgressWidth(row.daily_usage_usd, row.group?.daily_limit_usd) }"></div>
                </div>
                <span class="font-mono whitespace-nowrap">{{ getProgressPercentage(row.daily_usage_usd, row.group?.daily_limit_usd) }}</span>
              </div>
              <div class="pl-10 text-[10px] text-gray-400 dark:text-dark-500">{{ formatDailyUsageWindow(row) }}</div>
            </div>
            <div v-if="row.group?.weekly_limit_usd" class="space-y-0.5">
              <div class="flex items-center gap-2 text-xs">
                <span class="w-8 shrink-0 text-gray-500">{{ t('admin.subscriptions.weekly') }}</span>
                <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                  <div class="h-1.5 rounded-full transition-all" :class="getProgressClass(row.weekly_usage_usd, row.group?.weekly_limit_usd)" :style="{ width: getProgressWidth(row.weekly_usage_usd, row.group?.weekly_limit_usd) }"></div>
                </div>
                <span class="font-mono whitespace-nowrap">{{ getProgressPercentage(row.weekly_usage_usd, row.group?.weekly_limit_usd) }}</span>
              </div>
              <div class="pl-10 text-[10px] text-gray-400 dark:text-dark-500">{{ formatResetTime(row.weekly_window_start, 'weekly') }}</div>
            </div>
            <div v-if="row.group?.monthly_limit_usd" class="space-y-0.5">
              <div class="flex items-center gap-2 text-xs">
                <span class="w-8 shrink-0 text-gray-500">{{ t('admin.subscriptions.monthly') }}</span>
                <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                  <div class="h-1.5 rounded-full transition-all" :class="getProgressClass(row.monthly_usage_usd, row.group?.monthly_limit_usd)" :style="{ width: getProgressWidth(row.monthly_usage_usd, row.group?.monthly_limit_usd) }"></div>
                </div>
                <span class="font-mono whitespace-nowrap">{{ getProgressPercentage(row.monthly_usage_usd, row.group?.monthly_limit_usd) }}</span>
              </div>
              <div class="pl-10 text-[10px] text-gray-400 dark:text-dark-500">{{ formatResetTime(row.monthly_window_start, 'monthly') }}</div>
            </div>
            <div
              v-if="!row.group?.daily_limit_usd && !row.group?.weekly_limit_usd && !row.group?.monthly_limit_usd"
              class="flex items-center gap-2 rounded-md bg-emerald-50 px-2 py-1 text-xs text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
            >
              <span>∞</span>
              <span>{{ t('admin.subscriptions.unlimited') }}</span>
            </div>
          </div>
        </template>
        <template #cell-actions="{ row }">
          <!-- Reset-quota and revoke are sys-admin-only — see routes/team.go. -->
          <div v-if="source === 'admin'" class="flex items-center gap-1">
            <button
              v-if="row.status === 'active'"
              @click="askResetQuota(row)"
              class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400"
            >
              <Icon name="refresh" size="sm" />
              <span class="text-xs">{{ t('admin.subscriptions.resetQuota') }}</span>
            </button>
            <button
              v-if="row.status === 'active'"
              @click="askRevoke(row)"
              class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            >
              <Icon name="ban" size="sm" />
              <span class="text-xs">{{ t('admin.subscriptions.revoke') }}</span>
            </button>
          </div>
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

  <ConfirmDialog
    :show="resetTarget !== null"
    :title="t('admin.subscriptions.resetQuotaConfirmTitle')"
    :message="t('admin.subscriptions.resetQuotaConfirmContent', { user: resetTarget?.user?.email ?? resetTarget?.user_id })"
    @confirm="handleResetQuota"
    @cancel="resetTarget = null"
  />
  <ConfirmDialog
    :show="revokeTarget !== null"
    :title="t('admin.subscriptions.revokeConfirmTitle')"
    :message="t('admin.subscriptions.revokeConfirmContent', { user: revokeTarget?.user?.email ?? revokeTarget?.user_id })"
    danger
    @confirm="handleRevoke"
    @cancel="revokeTarget = null"
  />
</template>

<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { teamAPI, type TeamSubscription, type TeamMember, type Team, type SubscriptionPlan } from '@/api/team'
import { getRemainingDurationParts, isOneTimeDailyQuota, type RemainingDurationParts } from '@/utils/subscriptionQuota'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
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

// Layout exposes a refreshTeam fn so balance/header reflect post-purchase state.
const refreshTeam = inject<() => void>('refreshTeam', () => {})

const showPurchaseDialog = ref(false)
const resetTarget = ref<TeamSubscription | null>(null)
const revokeTarget = ref<TeamSubscription | null>(null)
const purchaseForm = ref<{ allMembers: boolean; userIds: number[]; planId: number | null }>({
  allMembers: false,
  userIds: [],
  planId: null,
})

const allColumns = computed<Column[]>(() => [
  { key: 'user', label: t('team.members.email'), sortable: false },
  { key: 'group', label: t('team.members.subscriptionStatus'), sortable: false },
  { key: 'status', label: t('common.status'), sortable: false },
  { key: 'expires_at', label: t('team.subscriptions.expiresAt'), sortable: false },
  { key: 'usage', label: t('team.subscriptions.usage'), sortable: false },
  { key: 'actions', label: t('common.actions'), sortable: false },
])

// Column visibility (localStorage). user + actions are always visible.
const HIDDEN_COLUMNS_KEY = 'team-subscriptions-hidden-columns'
const FORCED_VISIBLE = new Set(['user', 'actions'])
const hiddenColumns = reactive<Set<string>>(new Set())
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)
try {
  const raw = localStorage.getItem(HIDDEN_COLUMNS_KEY)
  if (raw) (JSON.parse(raw) as string[]).filter(k => !FORCED_VISIBLE.has(k)).forEach(k => hiddenColumns.add(k))
} catch { /* ignore */ }
function saveHiddenCols() {
  try { localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns])) } catch { /* ignore */ }
}
const columns = computed<Column[]>(() => allColumns.value.filter(c => FORCED_VISIBLE.has(c.key) || !hiddenColumns.has(c.key)))
const toggleableColumns = computed(() => allColumns.value.filter(c => !FORCED_VISIBLE.has(c.key)))
function isColumnVisible(key: string) { return !hiddenColumns.has(key) }
function toggleColumn(key: string) {
  if (FORCED_VISIBLE.has(key)) return
  if (hiddenColumns.has(key)) hiddenColumns.delete(key)
  else hiddenColumns.add(key)
  saveHiddenCols()
}

// Filter state
const filterStatus = ref<string>('')
const filterGroupId = ref<number | ''>('')
const searchQuery = ref('')
let searchDebounce: any = null
function onSearchInput() {
  clearTimeout(searchDebounce)
  searchDebounce = setTimeout(() => { page.value = 1; load() }, 300)
}
const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') },
  { value: 'active', label: t('admin.subscriptions.status.active') },
  { value: 'expired', label: t('admin.subscriptions.status.expired') },
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') },
])
const groupOptions = computed(() => {
  const seen = new Map<number, string>()
  for (const s of subscriptions.value) {
    if (s.group && !seen.has(s.group.id)) seen.set(s.group.id, s.group.name)
  }
  return [
    { value: '', label: t('admin.subscriptions.allGroups') },
    ...[...seen].map(([id, name]) => ({ value: id, label: name })),
  ]
})
function onFilterChange() { page.value = 1; load() }

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

function getDaysRemaining(expiresAt: string): number | null {
  const diff = new Date(expiresAt).getTime() - Date.now()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

function isExpiringSoon(expiresAt: string): boolean {
  const d = getDaysRemaining(expiresAt)
  return d !== null && d <= 7
}

function getProgressWidth(used: number | null | undefined, limit: number | null | undefined): string {
  const l = Number(limit ?? 0)
  if (l <= 0) return '0%'
  return `${Math.min((Number(used ?? 0) / l) * 100, 100)}%`
}

function getProgressPercentage(used: number | null | undefined, limit: number | null | undefined): string {
  const l = Number(limit ?? 0)
  if (l <= 0) return '0%'
  return `${Math.min(Math.round((Number(used ?? 0) / l) * 100), 100)}%`
}

function formatResetDuration(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return t('admin.subscriptions.resetInDaysHours', { days: parts.days, hours: parts.hours })
  }
  if (parts.hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }
  return t('admin.subscriptions.resetInMinutes', { minutes: parts.minutes })
}

function formatQuotaEndDuration(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return t('admin.subscriptions.quotaEndsInDaysHours', { days: parts.days, hours: parts.hours })
  }
  if (parts.hours > 0) {
    return t('admin.subscriptions.quotaEndsInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }
  return t('admin.subscriptions.quotaEndsInMinutes', { minutes: parts.minutes })
}

function formatResetTime(windowStart: string | null | undefined, period: 'daily' | 'weekly' | 'monthly'): string {
  if (!windowStart) return t('admin.subscriptions.windowNotActive')
  const hours = period === 'daily' ? 5 : period === 'weekly' ? 168 : 720
  const end = new Date(new Date(windowStart).getTime() + hours * 60 * 60 * 1000)
  const parts = getRemainingDurationParts(end)
  return parts ? formatResetDuration(parts) : t('admin.subscriptions.windowNotActive')
}

function formatDailyUsageWindow(row: TeamSubscription): string {
  if (isOneTimeDailyQuota(row) && row.expires_at) {
    const parts = getRemainingDurationParts(row.expires_at)
    return parts ? formatQuotaEndDuration(parts) : t('admin.subscriptions.windowNotActive')
  }
  return formatResetTime(row.daily_window_start, 'daily')
}

function getProgressClass(used: number | null | undefined, limit: number | null | undefined): string {
  const l = Number(limit ?? 0)
  if (l <= 0) return 'bg-gray-400'
  const pct = (Number(used ?? 0) / l) * 100
  if (pct >= 90) return 'bg-red-500'
  if (pct >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatDateOnly(v: string): string {
  return new Date(v).toLocaleDateString()
}

async function load() {
  loading.value = true
  try {
    const res = await teamAPI.listSubscriptions(props.source, props.teamId, page.value, pageSize.value, {
      status: filterStatus.value || undefined,
      groupID: filterGroupId.value === '' ? undefined : Number(filterGroupId.value),
      search: searchQuery.value.trim() || undefined,
    })
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

function askResetQuota(row: TeamSubscription) { resetTarget.value = row }
function askRevoke(row: TeamSubscription) { revokeTarget.value = row }

async function handleResetQuota() {
  if (!resetTarget.value) return
  const target = resetTarget.value
  try {
    await teamAPI.resetSubscriptionQuota(props.source, props.teamId, target.id, { daily: true, weekly: true, monthly: true })
    appStore.showSuccess(t('admin.subscriptions.quotaResetSuccess'))
    resetTarget.value = null
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

async function handleRevoke() {
  if (!revokeTarget.value) return
  const target = revokeTarget.value
  try {
    await teamAPI.revokeSubscription(props.source, props.teamId, target.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    revokeTarget.value = null
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
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
    refreshTeam()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to purchase')
  } finally { submitting.value = false }
}

watch(() => props.teamId, () => { page.value = 1; load() }, { immediate: true })

function handleClickOutside(e: MouseEvent) {
  if (showColumnDropdown.value && columnDropdownRef.value && !columnDropdownRef.value.contains(e.target as Node)) {
    showColumnDropdown.value = false
  }
}
onMounted(() => document.addEventListener('click', handleClickOutside))
onUnmounted(() => document.removeEventListener('click', handleClickOutside))
</script>
