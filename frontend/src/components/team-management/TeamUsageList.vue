<template>
  <div class="space-y-6">
    <UsageStatsCards :stats="(stats as AdminUsageStatsResponse | null)" :hide-cost="true" />

    <!-- Charts: date range + granularity, then 4 charts in 2x2 grid -->
    <div class="space-y-4">
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.dashboard.timeRange') }}:
            </span>
            <DateRangePicker
              v-model:start-date="startDate"
              v-model:end-date="endDate"
              @change="onDateRangeChange"
            />
          </div>
          <div class="ml-auto flex items-center gap-2">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.dashboard.granularity') }}:
            </span>
            <div class="w-28">
              <Select v-model="granularity" :options="granularityOptions" @change="loadChartData" />
            </div>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <ModelDistributionChart
          v-model:source="modelDistributionSource"
          v-model:metric="modelDistributionMetric"
          :model-stats="requestedModelStats"
          :upstream-model-stats="upstreamModelStats"
          :mapping-model-stats="mappingModelStats"
          :loading="modelStatsLoading"
          :show-source-toggle="true"
          :show-metric-toggle="true"
          :hide-cost="true"
          :start-date="startDate"
          :end-date="endDate"
          :filters="breakdownFilters"
          breakdown-scope="team"
          :breakdown-source="props.source"
          :team-id="props.teamId"
        />
        <GroupDistributionChart
          v-model:metric="groupDistributionMetric"
          :group-stats="groupStats"
          :loading="chartsLoading"
          :show-metric-toggle="true"
          :hide-cost="true"
          :start-date="startDate"
          :end-date="endDate"
          :filters="breakdownFilters"
          breakdown-scope="team"
          :breakdown-source="props.source"
          :team-id="props.teamId"
        />
      </div>
      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <EndpointDistributionChart
          v-model:source="endpointDistributionSource"
          v-model:metric="endpointDistributionMetric"
          :endpoint-stats="inboundEndpointStats"
          :endpoint-path-stats="endpointPathStats"
          :loading="endpointStatsLoading"
          :show-source-toggle="true"
          :show-metric-toggle="true"
          :hide-cost="true"
          :available-sources="['inbound', 'path']"
          :title="t('usage.endpointDistribution')"
          :start-date="startDate"
          :end-date="endDate"
          :filters="breakdownFilters"
          breakdown-scope="team"
          :breakdown-source="props.source"
          :team-id="props.teamId"
        />
        <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" :hide-cost="true" />
      </div>
    </div>

    <!-- Filters bar -->
    <div class="card p-6">
      <div class="flex flex-wrap items-end justify-between gap-4">
        <div class="flex flex-1 flex-wrap items-end gap-4">
          <div class="w-full sm:w-auto sm:min-w-[220px]">
            <label class="input-label">{{ t('team.members.title') }}</label>
            <Select
              v-model="filters.user_id"
              :options="memberOptions"
              :searchable="true"
              :placeholder="t('usage.allUsers')"
              @change="applyFilters"
            />
          </div>
          <div class="w-full sm:w-auto sm:min-w-[200px]">
            <label class="input-label">{{ t('usage.apiKeyFilter') }}</label>
            <input
              v-model="filters.api_key_name"
              type="text"
              class="input"
              :placeholder="t('admin.usage.searchApiKeyPlaceholder')"
              @change="applyFilters"
            />
          </div>
          <div class="w-full sm:w-auto sm:min-w-[200px]">
            <label class="input-label">{{ t('usage.model') }}</label>
            <Select v-model="filters.model" :options="modelOptions" searchable @change="applyFilters" />
          </div>
          <div class="w-full sm:w-auto sm:min-w-[200px]">
            <label class="input-label">{{ t('admin.usage.group') }}</label>
            <Select v-model="filters.group_id" :options="groupOptions" searchable @change="applyFilters" />
          </div>
          <div class="w-full sm:w-auto sm:min-w-[180px]">
            <label class="input-label">{{ t('usage.type') }}</label>
            <Select v-model="filters.request_type" :options="requestTypeOptions" @change="applyFilters" />
          </div>
          <div v-if="isSysAdmin" class="w-full sm:w-auto sm:min-w-[200px]">
            <label class="input-label">{{ t('admin.usage.billingType') }}</label>
            <Select v-model="filters.billing_type" :options="billingTypeOptions" @change="applyFilters" />
          </div>
          <div v-if="isSysAdmin" class="w-full sm:w-auto sm:min-w-[200px]">
            <label class="input-label">{{ t('admin.usage.billingMode') }}</label>
            <Select v-model="filters.billing_mode" :options="billingModeOptions" @change="applyFilters" />
          </div>
        </div>
        <div class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
          <button type="button" @click="refreshData" class="btn btn-secondary">{{ t('common.refresh') }}</button>
          <button type="button" @click="resetFilters" class="btn btn-secondary">{{ t('common.reset') }}</button>
          <button type="button" @click="exportToExcel" :disabled="exporting" class="btn btn-primary">
            {{ t('usage.exportExcel') }}
          </button>
        </div>
      </div>
    </div>

    <UsageTable
      :data="usageLogs"
      :loading="loading"
      :columns="visibleColumns"
      :hide-cost-breakdown="true"
      :server-side-sort="false"
    />
    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="handlePageChange"
      @update:pageSize="handlePageSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import { useAppStore } from '@/stores/app'
import { teamAPI, type TeamMember, type TeamUsageStats, type TeamUsageFilters } from '@/api/team'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatReasoningEffort } from '@/utils/format'
import { requestTypeToLegacyStream, resolveUsageRequestType } from '@/utils/usageRequestType'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import UsageStatsCards from '@/components/admin/usage/UsageStatsCards.vue'
import UsageTable from '@/components/admin/usage/UsageTable.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import GroupDistributionChart from '@/components/charts/GroupDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { Column } from '@/components/common/types'
import type { TrendDataPoint, ModelStat, GroupStat, EndpointStat, UsageRequestType } from '@/types'

interface Props {
  teamId: number
  source: 'admin' | 'team_admin'
}
const props = defineProps<Props>()

const { t } = useI18n()
const appStore = useAppStore()

// Team usage views always hide the cost *breakdown* (standard cost, account
// cost, rate multiplier) — regardless of viewer role. The headline "fee"
// (actual_cost) is always kept so users can still see what was paid; only the
// underlying cost figures are suppressed. Sys admin keeps billing_type /
// billing_mode filters above for diagnostics.
const isSysAdmin = computed(() => props.source === 'admin')

type DistributionMetric = 'tokens' | 'actual_cost'
type ModelDistributionSource = 'requested' | 'upstream' | 'mapping'
type EndpointSource = 'inbound' | 'path'

const formatLD = (d: Date) => {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}
const getLast24HoursRangeDates = () => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return { start: formatLD(start), end: formatLD(end) }
}
const getGranularityForRange = (start: string, end: string): 'day' | 'hour' => {
  const startTime = new Date(`${start}T00:00:00`).getTime()
  const endTime = new Date(`${end}T00:00:00`).getTime()
  const daysDiff = Math.ceil((endTime - startTime) / (1000 * 60 * 60 * 24))
  return daysDiff <= 1 ? 'hour' : 'day'
}

const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)
const granularity = ref<'day' | 'hour'>(getGranularityForRange(defaultRange.start, defaultRange.end))

interface TeamFilterState {
  user_id?: number | null
  api_key_name?: string
  model?: string | null
  group_id?: number | null
  request_type?: UsageRequestType | null
  billing_type?: number | null
  billing_mode?: string | null
}
const filters = ref<TeamFilterState>({
  user_id: null,
  api_key_name: '',
  model: null,
  group_id: null,
  request_type: null,
  billing_type: null,
  billing_mode: null,
})

// Stats + logs
const stats = ref<TeamUsageStats | null>(null)
const usageLogs = ref<any[]>([])
const loading = ref(false)
const exporting = ref(false)
const pagination = reactive({ page: 1, page_size: getPersistedPageSize(), total: 0 })

// Charts state
const trendData = ref<TrendDataPoint[]>([])
const groupStats = ref<GroupStat[]>([])
const requestedModelStats = ref<ModelStat[]>([])
const upstreamModelStats = ref<ModelStat[]>([])
const mappingModelStats = ref<ModelStat[]>([])
const inboundEndpointStats = ref<EndpointStat[]>([])
const endpointPathStats = ref<EndpointStat[]>([])
const chartsLoading = ref(false)
const modelStatsLoading = ref(false)
const endpointStatsLoading = ref(false)
const modelDistributionMetric = ref<DistributionMetric>('tokens')
const modelDistributionSource = ref<ModelDistributionSource>('requested')
const loadedModelSources = reactive<Record<ModelDistributionSource, boolean>>({
  requested: false,
  upstream: false,
  mapping: false,
})
const groupDistributionMetric = ref<DistributionMetric>('tokens')
const endpointDistributionMetric = ref<DistributionMetric>('tokens')
const endpointDistributionSource = ref<EndpointSource>('inbound')

// Filter dropdown options
const members = ref<TeamMember[]>([])
const groupOptions = ref<SelectOption[]>([{ value: null, label: t('admin.usage.allGroups') }])

const memberOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.allUsers') },
  ...members.value.map((m) => ({ value: m.user_id, label: m.user?.email ?? String(m.user_id) })),
])

// Models the team has used in the current date range; derived from the same
// requested-source model stats the chart already loads, so the dropdown costs
// no extra round-trip.
const modelOptions = computed<SelectOption[]>(() => {
  const seen = new Set<string>()
  for (const m of requestedModelStats.value) {
    if (m.model) seen.add(m.model)
  }
  return [
    { value: null, label: t('admin.usage.allModels') },
    ...Array.from(seen).sort().map((m) => ({ value: m, label: m })),
  ]
})

const requestTypeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allTypes') },
  { value: 'ws_v2', label: t('usage.ws') },
  { value: 'stream', label: t('usage.stream') },
  { value: 'sync', label: t('usage.sync') },
])

const billingTypeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allBillingTypes') },
  { value: 0, label: t('admin.usage.billingTypeBalance') },
  { value: 1, label: t('admin.usage.billingTypeSubscription') },
])

const billingModeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allBillingModes') },
  { value: 'token', label: t('admin.usage.billingModeToken') },
  { value: 'per_request', label: t('admin.usage.billingModePerRequest') },
  { value: 'image', label: t('admin.usage.billingModeImage') },
])

const granularityOptions = computed<SelectOption[]>(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') },
])

// Drop columns that don't apply to team scope (account / debug-log actions / IP / UA).
// ALWAYS_VISIBLE intentionally omits 'actions' so the debug-log button is hidden.
// 'cost' column is kept (it shows the actual fee), but the cost breakdown
// tooltip + account-billed line are hidden via hide-cost-breakdown on the
// table. 'billing_mode' stays visible for sys admin diagnostics.
const ALWAYS_VISIBLE = ['user', 'created_at']
const TEAM_COLUMN_KEYS_BASE = [
  'user', 'api_key', 'model', 'reasoning_effort', 'endpoint',
  'group', 'stream', 'billing_mode', 'tokens', 'cost',
  'first_token', 'duration', 'created_at',
]
const teamColumnKeys = computed(() =>
  isSysAdmin.value ? TEAM_COLUMN_KEYS_BASE : TEAM_COLUMN_KEYS_BASE.filter(k => k !== 'billing_mode')
)

const allColumns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.usage.user'), sortable: false },
  { key: 'api_key', label: t('usage.apiKeyFilter'), sortable: false },
  { key: 'model', label: t('usage.model'), sortable: false },
  { key: 'reasoning_effort', label: t('usage.reasoningEffort'), sortable: false },
  { key: 'endpoint', label: t('usage.endpoint'), sortable: false },
  { key: 'group', label: t('admin.usage.group'), sortable: false },
  { key: 'stream', label: t('usage.type'), sortable: false },
  { key: 'billing_mode', label: t('admin.usage.billingMode'), sortable: false },
  { key: 'tokens', label: t('usage.tokens'), sortable: false },
  { key: 'cost', label: t('usage.cost'), sortable: false },
  { key: 'first_token', label: t('usage.firstToken'), sortable: false },
  { key: 'duration', label: t('usage.duration'), sortable: false },
  { key: 'created_at', label: t('usage.time'), sortable: false },
])

const visibleColumns = computed<Column[]>(() =>
  allColumns.value.filter((col) => ALWAYS_VISIBLE.includes(col.key) || teamColumnKeys.value.includes(col.key))
)

// Filter fields forwarded into chart drill-down calls so the per-user breakdown
// respects the same filter set the user picked above.
const breakdownFilters = computed<Record<string, any>>(() => {
  const f: Record<string, any> = {}
  if (filters.value.user_id) f.user_id = filters.value.user_id
  if (filters.value.api_key_name) f.api_key_name = filters.value.api_key_name
  if (filters.value.group_id) f.group_id = filters.value.group_id
  if (filters.value.request_type) f.request_type = filters.value.request_type
  if (filters.value.billing_type !== null && filters.value.billing_type !== undefined) {
    f.billing_type = filters.value.billing_type
  }
  if (filters.value.billing_mode) f.billing_mode = filters.value.billing_mode
  return f
})

function buildBackendParams(): TeamUsageFilters {
  const out: TeamUsageFilters = {
    start_date: startDate.value,
    end_date: endDate.value,
  }
  if (filters.value.user_id) out.user_id = filters.value.user_id
  if (filters.value.api_key_name) out.api_key_name = filters.value.api_key_name
  if (filters.value.model) out.model = filters.value.model
  if (filters.value.group_id) out.group_id = filters.value.group_id
  if (filters.value.request_type) out.request_type = filters.value.request_type
  if (filters.value.billing_type !== null && filters.value.billing_type !== undefined) {
    out.billing_type = filters.value.billing_type
  }
  if (filters.value.billing_mode) out.billing_mode = filters.value.billing_mode
  return out
}

async function loadLogs() {
  loading.value = true
  try {
    const res = await teamAPI.listUsage(props.source, props.teamId, {
      page: pagination.page,
      page_size: pagination.page_size,
      ...buildBackendParams(),
    })
    usageLogs.value = res.items ?? []
    pagination.total = res.total ?? 0
  } catch (e: any) {
    appStore.showError(e?.message ?? t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  try {
    stats.value = await teamAPI.getUsageStats(props.source, props.teamId, buildBackendParams())
  } catch {
    stats.value = null
  }
}

function resetModelStatsCache() {
  requestedModelStats.value = []
  upstreamModelStats.value = []
  mappingModelStats.value = []
  loadedModelSources.requested = false
  loadedModelSources.upstream = false
  loadedModelSources.mapping = false
}

async function loadModelStats(source: ModelDistributionSource, force = false) {
  if (!force && loadedModelSources[source]) return
  modelStatsLoading.value = true
  try {
    const res = await teamAPI.getUsageModelStats(props.source, props.teamId, {
      ...buildBackendParams(),
      model_source: source,
    })
    const models = (res.models ?? []) as ModelStat[]
    if (source === 'requested') requestedModelStats.value = models
    else if (source === 'upstream') upstreamModelStats.value = models
    else mappingModelStats.value = models
    loadedModelSources[source] = true
  } catch {
    if (source === 'requested') requestedModelStats.value = []
    else if (source === 'upstream') upstreamModelStats.value = []
    else mappingModelStats.value = []
    loadedModelSources[source] = false
  } finally {
    modelStatsLoading.value = false
  }
}

async function loadChartData() {
  chartsLoading.value = true
  endpointStatsLoading.value = true
  try {
    const params = buildBackendParams()
    const [trendRes, groupRes, endpointRes] = await Promise.all([
      teamAPI.getUsageTrend(props.source, props.teamId, { ...params, granularity: granularity.value }),
      teamAPI.getUsageGroupStats(props.source, props.teamId, params),
      teamAPI.getUsageEndpointStats(props.source, props.teamId, params),
    ])
    trendData.value = (trendRes.trend ?? []) as TrendDataPoint[]
    groupStats.value = (groupRes.groups ?? []) as GroupStat[]
    inboundEndpointStats.value = (endpointRes.endpoints ?? []) as EndpointStat[]
    endpointPathStats.value = (endpointRes.endpoint_paths ?? []) as EndpointStat[]
  } catch {
    trendData.value = []
    groupStats.value = []
    inboundEndpointStats.value = []
    endpointPathStats.value = []
  } finally {
    chartsLoading.value = false
    endpointStatsLoading.value = false
  }
}

async function loadMembers() {
  try {
    const res = await teamAPI.listMembers(props.source, props.teamId, 1, 500)
    members.value = res.items ?? []
  } catch {
    members.value = []
  }
}

// Group options are derived from the team's subscriptions (groups the team has
// purchased), so a team admin only sees groups relevant to their team — not the
// platform-wide list.
async function loadGroupOptions() {
  try {
    const res = await teamAPI.listSubscriptions(props.source, props.teamId, 1, 500)
    const seen = new Map<number, string>()
    for (const sub of res.items ?? []) {
      if (sub.group?.id != null && !seen.has(sub.group.id)) {
        seen.set(sub.group.id, sub.group.name || `#${sub.group.id}`)
      }
    }
    groupOptions.value = [
      { value: null, label: t('admin.usage.allGroups') },
      ...Array.from(seen.entries()).map(([id, name]) => ({ value: id, label: name })),
    ]
  } catch {
    // keep the default option
  }
}


function applyFilters() {
  pagination.page = 1
  resetModelStatsCache()
  loadLogs()
  loadStats()
  loadModelStats(modelDistributionSource.value, true)
  loadChartData()
}

function refreshData() {
  resetModelStatsCache()
  loadLogs()
  loadStats()
  loadModelStats(modelDistributionSource.value, true)
  loadChartData()
}

function resetFilters() {
  const range = getLast24HoursRangeDates()
  startDate.value = range.start
  endDate.value = range.end
  granularity.value = getGranularityForRange(range.start, range.end)
  filters.value = {
    user_id: null,
    api_key_name: '',
    model: null,
    group_id: null,
    request_type: null,
    billing_type: null,
    billing_mode: null,
  }
  applyFilters()
}

function onDateRangeChange(range: { startDate: string; endDate: string }) {
  startDate.value = range.startDate
  endDate.value = range.endDate
  granularity.value = getGranularityForRange(range.startDate, range.endDate)
  applyFilters()
}

function handlePageChange(p: number) {
  pagination.page = p
  loadLogs()
}
function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  loadLogs()
}

function getRequestTypeLabel(log: any): string {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return t('usage.ws')
  if (requestType === 'stream') return t('usage.stream')
  if (requestType === 'sync') return t('usage.sync')
  return t('usage.unknown')
}

async function exportToExcel() {
  if (exporting.value) return
  exporting.value = true
  try {
    const XLSX = await import('xlsx')
    // Columns aligned with admin export, minus account / IP / UserAgent / Request ID
    // (intentionally dropped to keep the team export team-scoped and safe).
    // Upstream model / upstream endpoint are also dropped — team users shouldn't
    // see the routing target. Cost breakdown is omitted; only actual_cost stays.
    const headers = [
      t('usage.time'), t('admin.usage.user'), t('usage.apiKeyFilter'),
      t('usage.model'), t('usage.reasoningEffort'), t('admin.usage.group'),
      t('usage.inboundEndpoint'), t('usage.type'),
      t('admin.usage.inputTokens'), t('admin.usage.outputTokens'),
      t('admin.usage.cacheReadTokens'), t('admin.usage.cacheCreationTokens'),
      t('usage.userBilled'),
      t('usage.firstToken'), t('usage.duration'),
    ]
    const ws = XLSX.utils.aoa_to_sheet([headers])
    let p = 1
    let exported = 0
    while (true) {
      const baseParams = buildBackendParams()
      const requestType = baseParams.request_type
      const legacyStream = requestType ? requestTypeToLegacyStream(requestType) : undefined
      const res = await teamAPI.listUsage(props.source, props.teamId, {
        ...baseParams,
        stream: legacyStream === null ? undefined : legacyStream,
        page: p,
        page_size: 100,
      })
      const items = res.items ?? []
      const rows = items.map((log: any) => [
        log.created_at,
        log.user?.email || '',
        log.api_key?.name || '',
        log.model,
        formatReasoningEffort(log.reasoning_effort),
        log.group?.name || '',
        log.inbound_endpoint || '',
        getRequestTypeLabel(log),
        log.input_tokens,
        log.output_tokens,
        log.cache_read_tokens,
        log.cache_creation_tokens,
        log.actual_cost?.toFixed(6) || '0.000000',
        log.first_token_ms ?? '',
        log.duration_ms,
      ])
      if (rows.length) XLSX.utils.sheet_add_aoa(ws, rows, { origin: -1 })
      exported += rows.length
      if (items.length < 100) break
      p++
      if (p > 200) break // safety cap
    }
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, ws, 'Usage')
    saveAs(
      new Blob([XLSX.write(wb, { bookType: 'xlsx', type: 'array' })], {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      }),
      `team-${props.teamId}-usage_${startDate.value}_to_${endDate.value}.xlsx`
    )
    appStore.showSuccess(t('usage.exportSuccess'))
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Export failed')
  } finally {
    exporting.value = false
  }
}

watch(modelDistributionSource, (s) => {
  void loadModelStats(s)
})

watch(
  () => props.teamId,
  async () => {
    pagination.page = 1
    await Promise.all([loadMembers(), loadGroupOptions()])
    refreshData()
  },
  { immediate: true }
)
</script>
