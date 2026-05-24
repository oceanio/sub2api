<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500" />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('team.adminTeams.searchTeams')"
                class="input pl-10"
              />
            </div>
          </div>
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button @click="load" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="showCreateDialog = true">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('team.adminTeams.createTeam') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredTeams" :loading="loading">
          <template #cell-member_count="{ row }">
            <span class="font-mono text-sm">
              {{ row.member_count ?? 0 }}<span class="text-gray-400"> / {{ row.max_members > 0 ? row.max_members : '∞' }}</span>
            </span>
          </template>
          <template #cell-balance="{ value }">
            <span class="font-mono">{{ formatUSD(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <router-link
                :to="`/admin/teams/${row.id}`"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('common.detail')"
              >
                <Icon name="cog" size="sm" />
                <span class="text-xs">{{ t('common.manage') }}</span>
              </router-link>
              <button
                @click="openActionMenu(row, $event)"
                class="action-menu-trigger flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white"
                :class="{ 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white': activeMenuId === row.id }"
              >
                <Icon name="more" size="sm" />
                <span class="text-xs">{{ t('common.more') }}</span>
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState
              :title="t('team.adminTeams.noTeams')"
              :action-text="t('team.adminTeams.createTeam')"
              @action="showCreateDialog = true"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize" @change="onPageChange" />
      </template>
    </TablePageLayout>

    <!-- Action Menu (Teleported) -->
    <Teleport to="body">
      <div
        v-if="activeMenuId !== null && menuPosition"
        class="action-menu-content fixed z-[9999] w-48 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
        :style="{ top: menuPosition.top + 'px', left: menuPosition.left + 'px' }"
      >
        <div class="py-1">
          <template v-for="team in teams" :key="team.id">
            <template v-if="team.id === activeMenuId">
              <button
                @click="openEdit(team); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="edit" size="sm" class="text-gray-400" :stroke-width="2" />
                {{ t('common.edit') }}
              </button>
              <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>
              <button
                @click="openBalanceOp(team, 'add'); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="plus" size="sm" class="text-emerald-500" :stroke-width="2" />
                {{ t('team.adminTeams.recharge') }}
              </button>
              <button
                @click="openBalanceOp(team, 'subtract'); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg class="h-4 w-4 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
                </svg>
                {{ t('team.adminTeams.refund') }}
              </button>
              <router-link
                :to="`/admin/teams/${team.id}/balance`"
                @click="closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="dollar" size="sm" class="text-gray-400" :stroke-width="2" />
                {{ t('team.adminTeams.balanceHistory') }}
              </router-link>
              <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>
              <button
                @click="confirmDelete(team); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
              >
                <Icon name="trash" size="sm" :stroke-width="2" />
                {{ t('common.delete') }}
              </button>
            </template>
          </template>
        </div>
      </div>
    </Teleport>

    <!-- Create dialog -->
    <BaseDialog :show="showCreateDialog" :title="t('team.adminTeams.createTeam')" width="normal" @close="showCreateDialog = false">
      <form class="space-y-4">
        <div>
          <label class="input-label">{{ t('team.adminTeams.teamName') }}</label>
          <input v-model="createForm.name" type="text" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('team.adminTeams.initialAdmin') }} <span class="text-red-500">*</span></label>
          <input
            v-model="createForm.adminSearch"
            type="text"
            class="input"
            :placeholder="t('team.adminTeams.searchUserPlaceholder')"
            @input="onAdminSearch"
          />
          <ul v-if="adminCandidates.length > 0" class="mt-1 max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-dark-600">
            <li
              v-for="u in adminCandidates"
              :key="u.id"
              class="cursor-pointer px-3 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
              :class="{ 'bg-primary-50 dark:bg-primary-900/20': createForm.adminUserId === u.id }"
              @click="pickAdmin(u)"
            >
              {{ u.email }} <span class="text-gray-400">#{{ u.id }}</span>
            </li>
          </ul>
          <p v-if="createForm.adminUserId" class="mt-2 text-xs text-green-600">
            {{ t('team.adminTeams.selectedAdmin', { email: createForm.adminEmail }) }}
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('team.adminTeams.initialBalance') }}</label>
          <input v-model.number="createForm.initialBalance" type="number" min="0" step="0.01" class="input" />
          <p class="mt-1 text-xs text-gray-500">{{ t('team.adminTeams.initialBalanceHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('team.adminTeams.maxMembers') }}</label>
          <input v-model.number="createForm.maxMembers" type="number" min="0" step="1" class="input" />
          <p class="mt-1 text-xs text-gray-500">{{ t('team.adminTeams.maxMembersHint') }}</p>
        </div>
        <div>
          <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" v-model="createForm.alsoAddAsMember" />
            {{ t('team.adminTeams.alsoAddAsMember') }}
          </label>
          <p class="mt-1 text-xs text-gray-500">{{ t('team.adminTeams.alsoAddAsMemberHint') }}</p>
        </div>
      </form>
      <template #footer>
        <button class="btn btn-secondary" @click="showCreateDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || !canCreate" @click="handleCreate">
          {{ saving ? t('common.saving') : t('common.create') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Edit dialog -->
    <BaseDialog :show="showEditDialog" :title="t('common.edit')" width="narrow" @close="showEditDialog = false">
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

    <!-- Balance op dialog (recharge / refund) -->
    <BaseDialog
      :show="showBalanceDialog"
      :title="balanceForm.operation === 'add' ? t('team.adminTeams.recharge') : t('team.adminTeams.refund')"
      width="narrow"
      @close="showBalanceDialog = false"
    >
      <div v-if="balanceTarget" class="space-y-4">
        <div class="flex items-center justify-between rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-700">
          <span class="font-medium text-gray-700 dark:text-gray-200">{{ balanceTarget.name }}</span>
          <span class="text-gray-500 dark:text-gray-400">
            {{ t('team.overview.balance') }}: <span class="font-mono">${{ Number(balanceTarget.balance).toFixed(2) }}</span>
          </span>
        </div>
        <div>
          <label class="input-label">
            {{ balanceForm.operation === 'add' ? t('team.adminTeams.rechargeAmount') : t('team.adminTeams.refundAmount') }}
          </label>
          <div class="flex gap-2">
            <input v-model.number="balanceForm.amount" type="number" min="0.01" step="0.01" class="input flex-1" required />
            <button v-if="balanceForm.operation === 'subtract'" type="button" class="btn btn-secondary whitespace-nowrap" @click="fillAllBalance">
              {{ t('team.adminTeams.refundAll') }}
            </button>
          </div>
          <p v-if="balanceForm.operation === 'subtract' && balanceForm.amount > Number(balanceTarget.balance)" class="mt-1 text-xs text-red-600">
            {{ t('team.adminTeams.insufficientBalance') }}
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('team.adminTeams.rechargeNote') }}</label>
          <input v-model="balanceForm.note" type="text" class="input" />
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showBalanceDialog = false">{{ t('common.cancel') }}</button>
        <button
          class="btn"
          :class="balanceForm.operation === 'add' ? 'btn-primary' : 'btn-danger'"
          :disabled="saving || !canSubmitBalance"
          @click="handleBalanceSubmit"
        >
          {{ saving ? t('common.saving') : t('common.confirm') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('common.delete')"
      :message="deleteTarget ? t('team.adminTeams.confirmDelete', { name: deleteTarget.name }) : ''"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useTeamStore } from '@/stores/team'
import { teamAPI, type Team } from '@/api/team'
import * as adminUsers from '@/api/admin/users'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'

const { t } = useI18n()
const appStore = useAppStore()
const teamStore = useTeamStore()

const teams = ref<Team[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const searchQuery = ref('')

// Client-side search across name; cheap because list size is small.
const filteredTeams = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return teams.value
  return teams.value.filter(t => t.name.toLowerCase().includes(q))
})

const showCreateDialog = ref(false)
const showEditDialog = ref(false)
const showBalanceDialog = ref(false)
const showDeleteConfirm = ref(false)
const saving = ref(false)
const editTarget = ref<Team | null>(null)
const editForm = ref({ name: '', maxMembers: 0 })
const balanceTarget = ref<Team | null>(null)
const balanceForm = ref<{ amount: number; note: string; operation: 'add' | 'subtract' }>({ amount: 0, note: '', operation: 'add' })
const deleteTarget = ref<Team | null>(null)

// Action menu state (mirrors UsersView pattern)
const activeMenuId = ref<number | null>(null)
const menuPosition = ref<{ top: number; left: number } | null>(null)
const createForm = ref({
  name: '',
  adminSearch: '',
  adminUserId: 0,
  adminEmail: '',
  initialBalance: 0,
  maxMembers: 200,
  alsoAddAsMember: false,
})
const adminCandidates = ref<Array<{ id: number; email: string }>>([])
let adminSearchTimer: any = null

const canCreate = computed(() => !!createForm.value.name && createForm.value.adminUserId > 0)
const canSubmitBalance = computed(() => {
  if (!balanceTarget.value || balanceForm.value.amount <= 0) return false
  if (balanceForm.value.operation === 'subtract' && balanceForm.value.amount > Number(balanceTarget.value.balance)) return false
  return true
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('team.adminTeams.teamName'), sortable: false },
  { key: 'member_count', label: t('team.adminTeams.memberCount'), sortable: false },
  { key: 'balance', label: t('team.adminTeams.balance'), sortable: false },
  { key: 'actions', label: t('common.actions'), sortable: false },
])

function formatUSD(v: number) { return `$${Number(v).toFixed(2)}` }

function onAdminSearch() {
  const q = createForm.value.adminSearch.trim()
  if (adminSearchTimer) clearTimeout(adminSearchTimer)
  if (!q) { adminCandidates.value = []; return }
  adminSearchTimer = setTimeout(async () => {
    try {
      const res = await adminUsers.list(1, 10, { search: q })
      adminCandidates.value = (res.items ?? []).map(u => ({ id: u.id, email: u.email }))
    } catch { adminCandidates.value = [] }
  }, 250)
}

function pickAdmin(u: { id: number; email: string }) {
  createForm.value.adminUserId = u.id
  createForm.value.adminEmail = u.email
  createForm.value.adminSearch = u.email
  adminCandidates.value = []
}

async function load() {
  loading.value = true
  try {
    const res = await teamAPI.adminListTeams(page.value, pageSize.value)
    teams.value = (res as any).items ?? []
    total.value = (res as any).total ?? 0
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to load teams')
  } finally { loading.value = false }
}

function onPageChange(p: number) { page.value = p; load() }

async function handleCreate() {
  if (!canCreate.value) return
  saving.value = true
  try {
    await teamAPI.adminCreateTeam({
      name: createForm.value.name,
      initial_admin_user_id: createForm.value.adminUserId,
      initial_balance: createForm.value.initialBalance > 0 ? createForm.value.initialBalance : 0,
      max_members: createForm.value.maxMembers > 0 ? createForm.value.maxMembers : 0,
      also_add_as_member: createForm.value.alsoAddAsMember,
    })
    appStore.showSuccess(t('team.adminTeams.teamCreated'))
    showCreateDialog.value = false
    createForm.value = { name: '', adminSearch: '', adminUserId: 0, adminEmail: '', initialBalance: 0, maxMembers: 200, alsoAddAsMember: false }
    adminCandidates.value = []
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

function openEdit(team: Team) {
  editTarget.value = team
  editForm.value = { name: team.name, maxMembers: team.max_members ?? 0 }
  showEditDialog.value = true
}

async function handleEdit() {
  if (!editTarget.value) return
  saving.value = true
  try {
    await teamAPI.adminUpdateTeam(editTarget.value.id, { name: editForm.value.name, max_members: editForm.value.maxMembers })
    appStore.showSuccess(t('common.saved'))
    showEditDialog.value = false
    teamStore.invalidateAdminTeam(editTarget.value.id)
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

function openBalanceOp(team: Team, operation: 'add' | 'subtract') {
  balanceTarget.value = team
  balanceForm.value = { amount: 0, note: '', operation }
  showBalanceDialog.value = true
}

function fillAllBalance() {
  if (balanceTarget.value) balanceForm.value.amount = Number(balanceTarget.value.balance)
}

async function handleBalanceSubmit() {
  if (!balanceTarget.value || !canSubmitBalance.value) return
  saving.value = true
  try {
    if (balanceForm.value.operation === 'add') {
      await teamAPI.adminRechargeTeam(balanceTarget.value.id, balanceForm.value.amount, balanceForm.value.note)
      appStore.showSuccess(t('team.adminTeams.recharged'))
    } else {
      await teamAPI.adminRefundTeam(balanceTarget.value.id, balanceForm.value.amount, balanceForm.value.note)
      appStore.showSuccess(t('team.adminTeams.refunded'))
    }
    showBalanceDialog.value = false
    teamStore.invalidateAdminTeam(balanceTarget.value.id)
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

// ── Action menu (mirrors UsersView pattern) ───────────────────────────────
function openActionMenu(team: Team, e: MouseEvent) {
  if (activeMenuId.value === team.id) { closeActionMenu(); return }
  const target = e.currentTarget as HTMLElement
  if (!target) { closeActionMenu(); return }
  const rect = target.getBoundingClientRect()
  const menuWidth = 200
  const menuHeight = 240
  const padding = 8
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  let left, top
  if (viewportWidth < 768) {
    left = Math.max(padding, Math.min(rect.left + rect.width / 2 - menuWidth / 2, viewportWidth - menuWidth - padding))
    top = rect.bottom + 4
    if (top + menuHeight > viewportHeight - padding) {
      top = rect.top - menuHeight - 4
      if (top < padding) top = padding
    }
  } else {
    left = Math.max(padding, Math.min(e.clientX - menuWidth, viewportWidth - menuWidth - padding))
    top = e.clientY
    if (top + menuHeight > viewportHeight - padding) top = viewportHeight - menuHeight - padding
  }
  menuPosition.value = { top, left }
  activeMenuId.value = team.id
}

function closeActionMenu() {
  activeMenuId.value = null
  menuPosition.value = null
}

function handleClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.action-menu-trigger') && !target.closest('.action-menu-content')) {
    closeActionMenu()
  }
}

function confirmDelete(team: Team) { deleteTarget.value = team; showDeleteConfirm.value = true }

async function handleDelete() {
  if (!deleteTarget.value) return
  try {
    await teamAPI.adminDeleteTeam(deleteTarget.value.id)
    appStore.showSuccess(t('team.adminTeams.teamDeleted'))
    showDeleteConfirm.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  }
}

onMounted(() => {
  load()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
