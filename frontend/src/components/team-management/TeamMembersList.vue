<template>
  <TablePageLayout>
    <template #filters>
      <div class="flex flex-wrap items-center gap-3">
        <!-- Left: search + tag filter -->
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-64">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('team.members.searchPlaceholder')"
              class="input pl-10"
              @input="onSearchInput"
            />
          </div>
          <div v-if="allTags.length > 0" class="w-full sm:w-40">
            <Select
              v-model="filterTag"
              :options="tagOptions"
              :placeholder="t('team.members.allTags')"
              @change="onTagChange"
            />
          </div>
        </div>
        <!-- Right: actions -->
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
          <button class="btn btn-primary" @click="showCreateDialog = true">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('team.members.addMember') }}
          </button>
          <button class="btn btn-secondary" @click="openBatchImport">
            <Icon name="upload" size="md" class="mr-2" />
            {{ t('team.members.batchImport') }}
          </button>
          <button v-if="source === 'admin'" class="btn btn-secondary" @click="openAddExisting">
            <Icon name="userPlus" size="md" class="mr-2" />
            {{ t('team.adminTeams.addMember') }}
          </button>
        </div>
      </div>
    </template>

    <template #table>
      <DataTable :columns="columns" :data="members" :loading="loading">
        <template #cell-user="{ row }">
          <span class="font-medium">{{ row.user?.email ?? row.user_id }}</span>
        </template>
        <template #cell-is_admin="{ row }">
          <span :class="row.is_admin ? 'text-primary-600 font-medium' : 'text-gray-400'">
            {{ row.is_admin ? t('team.members.roleAdmin') : '—' }}
          </span>
        </template>
        <template #cell-tags="{ value }">
          <div class="flex flex-wrap gap-1">
            <span
              v-for="tag in (value as string[] || [])"
              :key="tag"
              class="rounded-full bg-primary-100 px-2 py-0.5 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >{{ tag }}</span>
          </div>
        </template>
        <template #cell-sub_quota="{ row }">
          <div class="flex items-center gap-1">
            <template v-if="inlineEditId === row.id">
              <input
                v-model.number="inlineEditValue"
                type="number"
                min="0"
                step="1"
                class="input h-8 w-24 px-2 text-sm"
                @keyup.enter="saveInline(row)"
                @keyup.esc="cancelInline"
              />
              <button class="btn btn-primary btn-xs" :disabled="saving" @click="saveInline(row)">✓</button>
              <button class="btn btn-secondary btn-xs" @click="cancelInline">×</button>
            </template>
            <template v-else>
              <span class="font-mono text-sm">{{ row.sub_quota === 0 ? '∞' : `$${row.sub_quota_used.toFixed(2)}/$${row.sub_quota.toFixed(2)}` }}</span>
              <button class="text-xs text-primary-600 hover:underline" @click="startInline(row)">{{ t('common.edit') }}</button>
            </template>
          </div>
        </template>
        <template #cell-active_subscriptions="{ row }">
          <span :class="(row.active_subscriptions ?? 0) > 0 ? 'text-green-600 font-medium' : 'text-gray-400'">
            {{ row.active_subscriptions ?? 0 }}
          </span>
        </template>
        <template #cell-last_active_at="{ row }">
          <span v-if="row.last_active_at" class="text-sm">{{ formatDateTime(row.last_active_at) }}</span>
          <span v-else class="text-gray-400">—</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex flex-wrap items-center gap-1">
            <button
              @click="toggleAdmin(row)"
              class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="row.is_admin ? t('team.members.demoteAdmin') : t('team.members.promoteAdmin')"
            >
              <Icon name="userPlus" size="sm" />
              <span class="text-xs">{{ row.is_admin ? t('team.members.demoteAdmin') : t('team.members.promoteAdmin') }}</span>
            </button>
            <button
              @click="openTags(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-purple-600 dark:hover:bg-dark-700 dark:hover:text-purple-400"
                :title="t('team.members.manageTags')"
              >
                <Icon name="badge" size="sm" />
                <span class="text-xs">{{ t('team.members.manageTags') }}</span>
              </button>
              <button
                @click="openResetPwd(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-amber-600 dark:hover:bg-dark-700 dark:hover:text-amber-400"
                :title="t('team.members.resetPassword')"
              >
                <Icon name="lock" size="sm" />
                <span class="text-xs">{{ t('team.members.resetPassword') }}</span>
              </button>
              <button
                @click="toggleStatus(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-orange-600 dark:hover:bg-dark-700 dark:hover:text-orange-400"
                :title="row.user?.status === 'disabled' ? t('team.members.enable') : t('team.members.disable')"
              >
                <Icon :name="row.user?.status === 'disabled' ? 'check' : 'ban'" size="sm" />
                <span class="text-xs">{{ row.user?.status === 'disabled' ? t('team.members.enable') : t('team.members.disable') }}</span>
            </button>
            <button
              @click="confirmRemove(row)"
              class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('team.members.remove')"
            >
              <Icon name="trash" size="sm" />
              <span class="text-xs">{{ t('team.members.remove') }}</span>
            </button>
          </div>
        </template>
      </DataTable>
      <Pagination :total="total" :page="page" :page-size="pageSize" @change="onPageChange" />
    </template>
  </TablePageLayout>

  <!-- team_admin: create new user dialog -->
  <BaseDialog :show="showCreateDialog" :title="t('team.members.addMember')" width="narrow" @close="showCreateDialog = false">
    <form class="space-y-4">
      <div>
        <label class="input-label">{{ t('team.members.email') }}</label>
        <input v-model="createForm.email" type="email" class="input" required />
      </div>
      <div>
        <label class="input-label">{{ t('team.members.username') }}</label>
        <input v-model="createForm.username" type="text" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('team.members.password') }}</label>
        <input v-model="createForm.password" type="password" class="input" required minlength="8" />
      </div>
    </form>
    <template #footer>
      <button class="btn btn-secondary" @click="showCreateDialog = false">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="saving" @click="handleCreate">
        {{ saving ? t('common.saving') : t('common.create') }}
      </button>
    </template>
  </BaseDialog>

  <!-- admin: add existing user dialog -->
  <BaseDialog v-if="source === 'admin'" :show="showAddExistingDialog" :title="t('team.adminTeams.addMember')" width="normal" @close="showAddExistingDialog = false">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('team.adminTeams.addMemberHint') }}</p>
      <div>
        <label class="input-label">{{ t('team.adminTeams.searchUserPlaceholder') }}</label>
        <input v-model="addExistingForm.search" type="text" class="input" :placeholder="t('team.adminTeams.searchUserPlaceholder')" @input="onUserSearch" />
        <ul v-if="userCandidates.length > 0" class="mt-1 max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-dark-600">
          <li
            v-for="u in userCandidates"
            :key="u.id"
            class="cursor-pointer px-3 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
            :class="{ 'bg-primary-50 dark:bg-primary-900/20': addExistingForm.userId === u.id }"
            @click="pickUser(u)"
          >
            {{ u.email }} <span class="text-gray-400">#{{ u.id }}</span>
          </li>
        </ul>
        <p v-if="addExistingForm.userId" class="mt-2 text-xs text-green-600">
          {{ t('team.adminTeams.selectedAdmin', { email: addExistingForm.email }) }}
        </p>
      </div>
    </div>
    <template #footer>
      <button class="btn btn-secondary" @click="showAddExistingDialog = false">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="saving || !addExistingForm.userId" @click="handleAddExisting">
        {{ saving ? t('common.saving') : t('common.confirm') }}
      </button>
    </template>
  </BaseDialog>

  <!-- Batch import dialog -->
  <BaseDialog :show="showBatchImportDialog" :title="t('team.members.batchImport')" width="normal" @close="closeBatchImport">
    <div class="space-y-4">
      <div class="rounded-md border border-gray-200 bg-gray-50 p-3 text-xs text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300">
        <p class="font-medium text-gray-700 dark:text-gray-200">{{ t('team.members.batchImportHint') }}</p>
        <pre class="mt-2 font-mono leading-5">alice@example.com,Alice123!,Alice
bob@example.com,Bob123!
carol@example.com,Carol123!,Carol</pre>
      </div>
      <div>
        <label class="input-label">{{ t('team.members.batchImportInput') }}</label>
        <textarea
          v-model="batchImportText"
          rows="10"
          class="input font-mono text-sm"
          :placeholder="'alice@example.com,Alice123!,Alice'"
        ></textarea>
        <p v-if="batchPreview.invalidCount > 0" class="mt-1 text-xs text-red-600">
          {{ t('team.members.batchImportInvalid', { count: batchPreview.invalidCount }) }}
        </p>
        <p v-else-if="batchPreview.validRows.length > 0" class="mt-1 text-xs text-emerald-600">
          {{ t('team.members.batchImportParsed', { count: batchPreview.validRows.length }) }}
        </p>
      </div>
      <div v-if="batchResult" class="space-y-2 rounded-md border border-gray-200 p-3 dark:border-dark-700">
        <p class="text-sm font-medium">
          {{ t('team.members.batchImportSummary', { succeeded: batchResult.succeeded, failed: batchResult.failed, total: batchResult.total }) }}
        </p>
        <div v-if="batchResult.failed > 0" class="max-h-40 overflow-y-auto text-xs">
          <div v-for="r in batchFailedRows" :key="r.index" class="flex items-start gap-2 py-1">
            <span class="font-mono text-gray-500">#{{ r.index + 1 }}</span>
            <span class="font-medium">{{ r.email }}</span>
            <span class="text-red-600">{{ r.error }}</span>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <button class="btn btn-secondary" @click="closeBatchImport">{{ t('common.cancel') }}</button>
      <button
        class="btn btn-primary"
        :disabled="saving || batchPreview.validRows.length === 0 || batchPreview.invalidCount > 0"
        @click="handleBatchImport"
      >
        {{ saving ? t('common.saving') : t('team.members.batchImportSubmit', { count: batchPreview.validRows.length }) }}
      </button>
    </template>
  </BaseDialog>

  <!-- Tags dialog (team_admin only) -->
  <BaseDialog :show="showTagsDialog" :title="t('team.members.manageTags')" width="narrow" @close="showTagsDialog = false">
    <div class="space-y-3">
      <div class="flex flex-wrap gap-2">
        <span
          v-for="(tag, i) in tagsForm.tags"
          :key="i"
          class="flex items-center gap-1 rounded-full bg-gray-100 px-3 py-1 text-sm dark:bg-dark-700"
        >
          {{ tag }}
          <button @click="removeTag(i)" class="text-gray-400 hover:text-red-500 ml-1">×</button>
        </span>
      </div>
      <div class="flex gap-2">
        <input v-model="newTag" type="text" class="input flex-1" :placeholder="t('team.tags.tagName')" @keydown.enter.prevent="addTag" />
        <button class="btn btn-secondary btn-sm" @click="addTag">{{ t('team.tags.addTag') }}</button>
      </div>
    </div>
    <template #footer>
      <button class="btn btn-secondary" @click="showTagsDialog = false">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="saving" @click="handleTags">
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
    </template>
  </BaseDialog>

  <!-- Reset password dialog (team_admin only) -->
  <BaseDialog :show="showPwdDialog" :title="t('team.members.resetPassword')" width="narrow" @close="showPwdDialog = false">
    <div>
      <label class="input-label">{{ t('team.members.newPasswordLabel') }}</label>
      <input v-model="pwdForm.newPassword" type="password" class="input" required minlength="8" />
    </div>
    <template #footer>
      <button class="btn btn-secondary" @click="showPwdDialog = false">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="saving" @click="handleResetPwd">
        {{ saving ? t('common.saving') : t('common.confirm') }}
      </button>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showRemoveConfirm"
    :title="t('team.members.remove')"
    :message="removeTarget ? t('team.members.confirmRemove', { name: removeTarget.user?.email ?? String(removeTarget.user_id) }) : ''"
    @confirm="handleRemove"
    @cancel="showRemoveConfirm = false"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { teamAPI, type TeamMember, type Team } from '@/api/team'
import * as adminUsers from '@/api/admin/users'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
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

const team = ref<Team | null>(null)
const members = ref<TeamMember[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const saving = ref(false)

const showCreateDialog = ref(false)
const showAddExistingDialog = ref(false)
const showBatchImportDialog = ref(false)
const showTagsDialog = ref(false)
const showPwdDialog = ref(false)

const batchImportText = ref('')
const batchResult = ref<import('@/api/team').BulkCreateMembersResult | null>(null)
const showRemoveConfirm = ref(false)
const removeTarget = ref<TeamMember | null>(null)
const activeTarget = ref<TeamMember | null>(null)
const newTag = ref('')

const filterTag = ref<string>('')
const searchQuery = ref('')
let searchDebounce: any = null
const inlineEditId = ref<number | null>(null)
const inlineEditValue = ref<number>(0)

// Column visibility: hidden set persisted in localStorage. Default: nothing hidden.
const HIDDEN_COLUMNS_KEY = 'team-members-hidden-columns'
const FORCED_VISIBLE = new Set(['user', 'actions'])
const hiddenColumns = reactive<Set<string>>(new Set())
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)
try {
  const raw = localStorage.getItem(HIDDEN_COLUMNS_KEY)
  if (raw) {
    const arr = JSON.parse(raw) as string[]
    arr.filter(k => !FORCED_VISIBLE.has(k)).forEach(k => hiddenColumns.add(k))
  }
} catch { /* ignore */ }
function saveHiddenCols() {
  try { localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns])) } catch { /* ignore */ }
}

const createForm = ref({ email: '', username: '', password: '' })
const tagsForm = ref({ tags: [] as string[] })
const pwdForm = ref({ newPassword: '' })

const addExistingForm = ref({ search: '', userId: 0, email: '' })
const userCandidates = ref<Array<{ id: number; email: string }>>([])
let userSearchTimer: any = null

const allTags = computed(() => {
  const fromTeam = team.value?.available_tags ?? []
  if (fromTeam.length > 0) return [...fromTeam].sort()
  const set = new Set<string>()
  for (const m of members.value) for (const tag of (m.tags ?? [])) set.add(tag)
  return Array.from(set).sort()
})

const allColumns = computed<Column[]>(() => [
  { key: 'user', label: t('team.members.email'), sortable: false },
  { key: 'is_admin', label: t('team.members.role'), sortable: false },
  { key: 'tags', label: t('team.members.tags'), sortable: false },
  { key: 'sub_quota', label: t('team.members.subQuota'), sortable: false },
  { key: 'active_subscriptions', label: t('team.members.subscriptionStatus'), sortable: false },
  { key: 'last_active_at', label: t('team.members.lastActive'), sortable: false },
  { key: 'actions', label: t('common.actions'), sortable: false },
])
const columns = computed<Column[]>(() => allColumns.value.filter(c => FORCED_VISIBLE.has(c.key) || !hiddenColumns.has(c.key)))
const toggleableColumns = computed(() => allColumns.value.filter(c => !FORCED_VISIBLE.has(c.key)))
function isColumnVisible(key: string) { return !hiddenColumns.has(key) }
function toggleColumn(key: string) {
  if (FORCED_VISIBLE.has(key)) return
  if (hiddenColumns.has(key)) hiddenColumns.delete(key)
  else hiddenColumns.add(key)
  saveHiddenCols()
}

function onSearchInput() {
  clearTimeout(searchDebounce)
  searchDebounce = setTimeout(() => { page.value = 1; load() }, 300)
}

const tagOptions = computed(() => [
  { value: '', label: t('team.members.allTags') },
  ...allTags.value.map(tag => ({ value: tag, label: tag })),
])
function onTagChange() { page.value = 1; load() }

function startInline(m: TeamMember) { inlineEditId.value = m.id; inlineEditValue.value = m.sub_quota }
function cancelInline() { inlineEditId.value = null }

async function saveInline(m: TeamMember) {
  if (inlineEditValue.value < 0) return
  saving.value = true
  try {
    await teamAPI.updateSubQuota(props.source, props.teamId, m.id, inlineEditValue.value)
    appStore.showSuccess(t('team.members.subQuotaUpdated'))
    cancelInline()
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

async function load() {
  loading.value = true
  try {
    const [tm, res] = await Promise.all([
      teamAPI.getTeam(props.source, props.teamId),
      teamAPI.listMembers(props.source, props.teamId, page.value, pageSize.value, {
        tags: filterTag.value ? [filterTag.value] : undefined,
        search: searchQuery.value.trim() || undefined,
      }),
    ])
    team.value = tm
    members.value = (res as any).items ?? []
    total.value = (res as any).total ?? 0
  } finally { loading.value = false }
}

function onPageChange(p: number) { page.value = p; load() }

async function handleCreate() {
  saving.value = true
  try {
    await teamAPI.createMember(props.source, props.teamId, createForm.value)
    appStore.showSuccess(t('team.members.memberCreated'))
    showCreateDialog.value = false
    createForm.value = { email: '', username: '', password: '' }
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

// ── Batch import (CSV: email,password[,username] per line) ─────────────────
function openBatchImport() {
  batchImportText.value = ''
  batchResult.value = null
  showBatchImportDialog.value = true
}

function closeBatchImport() {
  showBatchImportDialog.value = false
  batchResult.value = null
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const batchPreview = computed(() => {
  const lines = batchImportText.value.split(/\r?\n/)
  const validRows: { email: string; password: string; username: string }[] = []
  let invalidCount = 0
  for (const raw of lines) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const cells = line.split(',').map(c => c.trim())
    const email = cells[0] ?? ''
    const password = cells[1] ?? ''
    const username = cells[2] ?? ''
    if (!EMAIL_RE.test(email) || password.length < 8) {
      invalidCount++
      continue
    }
    validRows.push({ email, password, username })
  }
  return { validRows, invalidCount }
})

const batchFailedRows = computed(() => batchResult.value?.rows.filter(r => r.error) ?? [])

async function handleBatchImport() {
  const rows = batchPreview.value.validRows
  if (rows.length === 0) return
  saving.value = true
  batchResult.value = null
  try {
    const result = await teamAPI.bulkCreateMembers(props.source, props.teamId, rows)
    batchResult.value = result
    if (result.failed === 0) {
      appStore.showSuccess(t('team.members.batchImportAllOk', { count: result.succeeded }))
      showBatchImportDialog.value = false
    } else {
      appStore.showError(t('team.members.batchImportPartial', { succeeded: result.succeeded, failed: result.failed }))
    }
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

function openTags(m: TeamMember) {
  activeTarget.value = m
  tagsForm.value.tags = [...(m.tags ?? [])]
  showTagsDialog.value = true
}

function addTag() {
  const tag = newTag.value.trim()
  if (tag && !tagsForm.value.tags.includes(tag)) tagsForm.value.tags.push(tag)
  newTag.value = ''
}

function removeTag(i: number) { tagsForm.value.tags.splice(i, 1) }

async function handleTags() {
  if (!activeTarget.value) return
  saving.value = true
  try {
    await teamAPI.updateMemberTags(props.source, props.teamId, activeTarget.value.id, tagsForm.value.tags)
    appStore.showSuccess(t('team.members.tagsUpdated'))
    showTagsDialog.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

function openResetPwd(m: TeamMember) {
  activeTarget.value = m
  pwdForm.value.newPassword = ''
  showPwdDialog.value = true
}

async function handleResetPwd() {
  if (!activeTarget.value) return
  saving.value = true
  try {
    await teamAPI.resetMemberPassword(props.source, props.teamId, activeTarget.value.id, pwdForm.value.newPassword)
    appStore.showSuccess(t('team.members.passwordReset'))
    showPwdDialog.value = false
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

function confirmRemove(m: TeamMember) {
  removeTarget.value = m
  showRemoveConfirm.value = true
}

async function handleRemove() {
  if (!removeTarget.value) return
  try {
    await teamAPI.removeMember(props.source, props.teamId, removeTarget.value.id)
    appStore.showSuccess(t('team.members.memberRemoved'))
    showRemoveConfirm.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

async function toggleStatus(m: TeamMember) {
  const next = m.user?.status === 'disabled' ? 'active' : 'disabled'
  try {
    await teamAPI.setMemberStatus(props.source, props.teamId, m.id, next as 'active' | 'disabled')
    appStore.showSuccess(t(next === 'active' ? 'team.members.enabled' : 'team.members.disabled'))
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

async function toggleAdmin(m: TeamMember) {
  try {
    if (m.is_admin) {
      await teamAPI.removeAdmin(props.source, props.teamId, m.user_id)
      appStore.showSuccess(t('common.saved'))
    } else {
      await teamAPI.addAdmin(props.source, props.teamId, m.user_id)
      appStore.showSuccess(t('common.saved'))
    }
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

// Admin: add existing user dialog
function openAddExisting() {
  addExistingForm.value = { search: '', userId: 0, email: '' }
  userCandidates.value = []
  showAddExistingDialog.value = true
}

function onUserSearch() {
  const q = addExistingForm.value.search.trim()
  if (userSearchTimer) clearTimeout(userSearchTimer)
  if (!q) { userCandidates.value = []; return }
  userSearchTimer = setTimeout(async () => {
    try {
      const res = await adminUsers.list(1, 10, { search: q })
      userCandidates.value = (res.items ?? []).map(u => ({ id: u.id, email: u.email }))
    } catch { userCandidates.value = [] }
  }, 250)
}

function pickUser(u: { id: number; email: string }) {
  addExistingForm.value.userId = u.id
  addExistingForm.value.email = u.email
  addExistingForm.value.search = u.email
  userCandidates.value = []
}

async function handleAddExisting() {
  if (!addExistingForm.value.userId) return
  saving.value = true
  try {
    await teamAPI.adminAddMember(props.teamId, addExistingForm.value.userId)
    appStore.showSuccess(t('common.saved'))
    showAddExistingDialog.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

watch(() => props.teamId, () => { page.value = 1; load() }, { immediate: true })

// Close column dropdown on outside click.
function handleClickOutside(e: MouseEvent) {
  if (showColumnDropdown.value && columnDropdownRef.value && !columnDropdownRef.value.contains(e.target as Node)) {
    showColumnDropdown.value = false
  }
}
onMounted(() => document.addEventListener('click', handleClickOutside))
onUnmounted(() => document.removeEventListener('click', handleClickOutside))
</script>
