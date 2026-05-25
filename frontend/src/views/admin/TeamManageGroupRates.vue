<template>
  <TeamManageLayout :team-id="teamId" source="admin">
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500" />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('team.groupRates.searchPlaceholder')"
                class="input pl-10"
              />
            </div>
            <Select
              v-model="platformFilter"
              :options="platformOptions"
              :placeholder="t('admin.groups.allPlatforms')"
              class="w-44"
            />
            <Select
              v-model="overrideFilter"
              :options="overrideFilterOptions"
              class="w-44"
            />
          </div>
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button @click="reload" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredRows" :loading="loading">
          <template #cell-name="{ row }">
            <span class="font-medium text-gray-900 dark:text-white">{{ row.groupName }}</span>
          </template>

          <template #cell-platform="{ row }">
            <span
              :class="[
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                row.platform === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : row.platform === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : row.platform === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
              ]"
            >
              <PlatformIcon :platform="row.platform" size="xs" />
              {{ t('admin.groups.platforms.' + row.platform) }}
            </span>
          </template>

          <template #cell-rate="{ row }">
            <div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
              <span
                v-if="row.teamRate !== null"
                class="rounded-md bg-primary-50 px-2 py-0.5 font-mono text-sm font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
              >
                {{ row.teamRate }}x
              </span>
              <span v-else class="font-mono text-sm text-gray-400">—</span>
              <span
                v-if="showDiscountHint && row.teamRate !== null && row.teamRate > 0"
                class="whitespace-nowrap text-xs text-gray-400"
              >
                ≈ {{ formatDiscount(row.teamRate) }} {{ t('team.groupRates.discountSuffix') }}
              </span>
              <span class="whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">
                {{ t('team.groupRates.col.defaultShort') }} {{ row.defaultRate }}x
                <span v-if="showDiscountHint && row.defaultRate > 0" class="text-gray-400">
                  ({{ formatDiscount(row.defaultRate) }})
                </span>
              </span>
            </div>
          </template>

          <template #cell-rpm="{ row }">
            <div class="flex items-baseline gap-2">
              <span
                v-if="row.teamRpm !== null"
                :class="[
                  'rounded-md px-2 py-0.5 font-mono text-sm font-semibold',
                  row.teamRpm === 0
                    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                    : 'bg-orange-50 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
                ]"
              >
                {{ row.teamRpm === 0 ? t('team.groupRates.exempt') : row.teamRpm }}
              </span>
              <span v-else class="font-mono text-sm text-gray-400">—</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('team.groupRates.col.defaultShort') }}
                {{ row.defaultRpm > 0 ? row.defaultRpm : t('team.groupRates.unlimited') }}
              </span>
            </div>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="openRateDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-purple-600 dark:hover:bg-dark-700 dark:hover:text-purple-400"
                :title="t('team.groupRates.actions.setRate')"
              >
                <Icon name="dollar" size="sm" />
                <span class="text-xs">{{ t('team.groupRates.actions.setRate') }}</span>
              </button>
              <button
                @click="openRpmDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-orange-600 dark:hover:bg-dark-700 dark:hover:text-orange-400"
                :title="t('team.groupRates.actions.setRpm')"
              >
                <Icon name="bolt" size="sm" />
                <span class="text-xs">{{ t('team.groupRates.actions.setRpm') }}</span>
              </button>
              <button
                v-if="row.teamRate !== null || row.teamRpm !== null"
                @click="confirmClear(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('team.groupRates.actions.clear')"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('team.groupRates.actions.clear') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState :title="t('team.groupRates.noGroups')" />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Rate edit dialog -->
    <BaseDialog
      :show="showRateDialog"
      :title="t('team.groupRates.dialog.rateTitle')"
      width="narrow"
      @close="showRateDialog = false"
    >
      <div v-if="dialogRow" class="space-y-4">
        <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-700">
          <div class="font-medium text-gray-900 dark:text-white">{{ dialogRow.groupName }}</div>
          <div class="mt-1 text-xs text-gray-500">
            {{ t('team.groupRates.dialog.defaultRate') }}:
            <span class="font-mono">{{ dialogRow.defaultRate }}x</span>
            <span v-if="showDiscountHint && dialogRow.defaultRate > 0" class="ml-1 text-gray-400">
              ({{ formatDiscount(dialogRow.defaultRate) }} {{ t('team.groupRates.discountSuffix') }})
            </span>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('team.groupRates.dialog.teamRate') }}</label>
          <div class="flex items-center gap-2">
            <input
              v-model="rateInput"
              type="number"
              step="0.001"
              min="0"
              class="input hide-spinner flex-1"
              :placeholder="String(dialogRow.defaultRate)"
            />
            <span
              v-if="showDiscountHint && rateInputNumber !== null && rateInputNumber > 0"
              class="whitespace-nowrap text-xs text-gray-400"
            >
              ≈ {{ formatDiscount(rateInputNumber) }} {{ t('team.groupRates.discountSuffix') }}
            </span>
          </div>
          <p class="mt-1 text-xs text-gray-500">{{ t('team.groupRates.dialog.rateHint') }}</p>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showRateDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="saveRate">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <!-- RPM edit dialog -->
    <BaseDialog
      :show="showRpmDialog"
      :title="t('team.groupRates.dialog.rpmTitle')"
      width="narrow"
      @close="showRpmDialog = false"
    >
      <div v-if="dialogRow" class="space-y-4">
        <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-700">
          <div class="font-medium text-gray-900 dark:text-white">{{ dialogRow.groupName }}</div>
          <div class="mt-1 text-xs text-gray-500">
            {{ t('team.groupRates.dialog.defaultRpm') }}:
            <span class="font-mono">
              {{ dialogRow.defaultRpm > 0 ? dialogRow.defaultRpm : t('team.groupRates.unlimited') }}
            </span>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('team.groupRates.dialog.teamRpm') }}</label>
          <input
            v-model="rpmInput"
            type="number"
            step="1"
            min="0"
            class="input hide-spinner"
            :placeholder="t('team.groupRates.dialog.rpmPlaceholder')"
          />
          <p class="mt-1 text-xs text-gray-500">{{ t('team.groupRates.dialog.rpmHint') }}</p>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showRpmDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="saveRpm">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showClearConfirm"
      :title="t('team.groupRates.actions.clear')"
      :message="clearMessage"
      @confirm="performClear"
      @cancel="showClearConfirm = false"
    />
  </TeamManageLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { teamAPI } from '@/api/team'
import TeamManageLayout from '@/components/team-management/TeamManageLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { Group } from '@/types'
import type { Column } from '@/components/common/types'

interface Row {
  groupId: number
  groupName: string
  platform: string
  defaultRate: number
  defaultRpm: number
  teamRate: number | null
  teamRpm: number | null
}

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

const teamId = computed(() => Number(route.params.id))
const loading = ref(false)
const saving = ref(false)
const rows = ref<Row[]>([])

// 折扣预览：与 admin GroupRateMultipliersModal / UserAllowedGroupsModal 完全一致 ——
// 仅当 sys 开启 display_discount_enabled 且配置了 usd_exchange_rate 时显示。
const displayDiscountEnabled = computed(
  () => appStore.cachedPublicSettings?.display_discount_enabled === true,
)
const usdExchangeRate = computed(() => appStore.cachedPublicSettings?.usd_exchange_rate ?? null)
const showDiscountHint = computed(
  () => displayDiscountEnabled.value && (usdExchangeRate.value ?? 0) > 0,
)
const formatDiscount = (multiplier: number | null | undefined): string => {
  if (multiplier == null || !showDiscountHint.value) return ''
  const rate = usdExchangeRate.value as number
  return `${Math.round((multiplier / rate) * 100)}%`
}

const searchQuery = ref('')
const platformFilter = ref('')
const overrideFilter = ref<'all' | 'has' | 'none'>('all')

const showRateDialog = ref(false)
const showRpmDialog = ref(false)
const showClearConfirm = ref(false)
const dialogRow = ref<Row | null>(null)
const rateInput = ref<string>('')
const rpmInput = ref<string>('')

// 折扣预览用：把 rateInput 解析成 number；非法/空返回 null。computed 保证 dialog
// 里"≈ X% 折扣"会随输入实时更新。
const rateInputNumber = computed<number | null>(() => {
  const raw = rateInput.value == null ? '' : String(rateInput.value).trim()
  if (raw === '') return null
  const n = parseFloat(raw)
  return isNaN(n) ? null : n
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('team.groupRates.col.group'), sortable: false },
  { key: 'platform', label: t('admin.groups.form.platform'), sortable: false },
  { key: 'rate', label: t('team.groupRates.col.teamRate'), sortable: false },
  { key: 'rpm', label: t('team.groupRates.col.teamRpm'), sortable: false },
  { key: 'actions', label: t('common.actions'), sortable: false },
])

const platformOptions = computed(() => {
  const set = new Set(rows.value.map(r => r.platform))
  const opts: Array<{ label: string; value: string }> = [{ label: t('admin.groups.allPlatforms'), value: '' }]
  for (const p of set) opts.push({ label: t('admin.groups.platforms.' + p), value: p })
  return opts
})

const overrideFilterOptions = computed(() => [
  { label: t('team.groupRates.filter.all'), value: 'all' },
  { label: t('team.groupRates.filter.hasOverride'), value: 'has' },
  { label: t('team.groupRates.filter.noOverride'), value: 'none' },
])

const filteredRows = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return rows.value.filter(r => {
    if (q && !r.groupName.toLowerCase().includes(q)) return false
    if (platformFilter.value && r.platform !== platformFilter.value) return false
    if (overrideFilter.value === 'has' && r.teamRate === null && r.teamRpm === null) return false
    if (overrideFilter.value === 'none' && (r.teamRate !== null || r.teamRpm !== null)) return false
    return true
  })
})

async function reload() {
  loading.value = true
  try {
    const [groupsRes, ratesRes] = await Promise.all([
      adminAPI.groups.list(1, 1000),
      teamAPI.adminGetTeamGroupRates(teamId.value),
    ])
    const groups: Group[] = (groupsRes.items ?? []).filter(g => g.status === 'active')
    rows.value = groups.map(g => ({
      groupId: g.id,
      groupName: g.name,
      platform: g.platform,
      defaultRate: g.rate_multiplier,
      defaultRpm: g.rpm_limit ?? 0,
      teamRate: ratesRes.group_rates[g.id] ?? null,
      teamRpm: ratesRes.group_rpm_overrides[g.id] ?? null,
    }))
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to load group rates')
  } finally {
    loading.value = false
  }
}

function openRateDialog(row: Row) {
  dialogRow.value = row
  rateInput.value = row.teamRate !== null ? String(row.teamRate) : ''
  showRateDialog.value = true
}

function openRpmDialog(row: Row) {
  dialogRow.value = row
  rpmInput.value = row.teamRpm !== null ? String(row.teamRpm) : ''
  showRpmDialog.value = true
}

async function saveRate() {
  if (!dialogRow.value) return
  // v-model on <input type="number"> can yield either string or number at runtime
  // depending on Vue's vModelDynamic resolution — coerce to string first.
  const raw = rateInput.value == null ? '' : String(rateInput.value)
  const trimmed = raw.trim()
  let newVal: number | null
  if (trimmed === '') {
    newVal = null
  } else {
    const n = parseFloat(trimmed)
    if (isNaN(n) || n < 0) {
      appStore.showError(t('team.groupRates.errors.invalidRate'))
      return
    }
    newVal = n
  }
  // 没改不发请求
  if (newVal === dialogRow.value.teamRate) {
    showRateDialog.value = false
    return
  }
  saving.value = true
  try {
    await teamAPI.adminUpdateTeam(teamId.value, {
      name: '',
      group_rates: { [dialogRow.value.groupId]: newVal },
    })
    appStore.showSuccess(t('common.saved'))
    showRateDialog.value = false
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally {
    saving.value = false
  }
}

async function saveRpm() {
  if (!dialogRow.value) return
  const raw = rpmInput.value == null ? '' : String(rpmInput.value)
  const trimmed = raw.trim()
  let newVal: number | null
  if (trimmed === '') {
    newVal = null
  } else {
    const n = parseInt(trimmed, 10)
    if (isNaN(n) || n < 0) {
      appStore.showError(t('team.groupRates.errors.invalidRpm'))
      return
    }
    newVal = n
  }
  if (newVal === dialogRow.value.teamRpm) {
    showRpmDialog.value = false
    return
  }
  saving.value = true
  try {
    await teamAPI.adminUpdateTeam(teamId.value, {
      name: '',
      group_rpm_overrides: { [dialogRow.value.groupId]: newVal },
    })
    appStore.showSuccess(t('common.saved'))
    showRpmDialog.value = false
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally {
    saving.value = false
  }
}

const clearMessage = computed(() => {
  if (!dialogRow.value) return ''
  return t('team.groupRates.dialog.clearConfirm', { name: dialogRow.value.groupName })
})

function confirmClear(row: Row) {
  dialogRow.value = row
  showClearConfirm.value = true
}

async function performClear() {
  if (!dialogRow.value) return
  const groupID = dialogRow.value.groupId
  const payload: any = { name: '' }
  if (dialogRow.value.teamRate !== null) payload.group_rates = { [groupID]: null }
  if (dialogRow.value.teamRpm !== null) payload.group_rpm_overrides = { [groupID]: null }
  try {
    await teamAPI.adminUpdateTeam(teamId.value, payload)
    appStore.showSuccess(t('common.saved'))
    showClearConfirm.value = false
    await reload()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

// DataTable expects columns to be reactive — exposed via the computed above.

onMounted(reload)
</script>

<style scoped>
.hide-spinner::-webkit-outer-spin-button,
.hide-spinner::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.hide-spinner {
  -moz-appearance: textfield;
}
</style>
