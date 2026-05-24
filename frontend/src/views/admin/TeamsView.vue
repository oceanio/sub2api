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
                @click="openRecharge(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-emerald-600 dark:hover:bg-dark-700 dark:hover:text-emerald-400"
                :title="t('team.adminTeams.recharge')"
              >
                <Icon name="dollar" size="sm" />
                <span class="text-xs">{{ t('team.adminTeams.recharge') }}</span>
              </button>
              <button
                @click="confirmDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') }}</span>
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

    <!-- Recharge dialog -->
    <BaseDialog :show="showRechargeDialog" :title="t('team.adminTeams.recharge')" width="narrow" @close="showRechargeDialog = false">
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('team.adminTeams.rechargeAmount') }}</label>
          <input v-model.number="rechargeForm.amount" type="number" min="0.01" step="0.01" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('team.adminTeams.rechargeNote') }}</label>
          <input v-model="rechargeForm.note" type="text" class="input" />
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showRechargeDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving" @click="handleRecharge">
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
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
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
const showRechargeDialog = ref(false)
const showDeleteConfirm = ref(false)
const saving = ref(false)
const rechargeTarget = ref<Team | null>(null)
const deleteTarget = ref<Team | null>(null)
const createForm = ref({
  name: '',
  adminSearch: '',
  adminUserId: 0,
  adminEmail: '',
  initialBalance: 0,
  maxMembers: 0,
  alsoAddAsMember: false,
})
const adminCandidates = ref<Array<{ id: number; email: string }>>([])
let adminSearchTimer: any = null
const rechargeForm = ref({ amount: 0, note: '' })

const canCreate = computed(() => !!createForm.value.name && createForm.value.adminUserId > 0)

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
    createForm.value = { name: '', adminSearch: '', adminUserId: 0, adminEmail: '', initialBalance: 0, maxMembers: 0, alsoAddAsMember: false }
    adminCandidates.value = []
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

function openRecharge(team: Team) {
  rechargeTarget.value = team
  rechargeForm.value = { amount: 0, note: '' }
  showRechargeDialog.value = true
}

async function handleRecharge() {
  if (!rechargeTarget.value || rechargeForm.value.amount <= 0) return
  saving.value = true
  try {
    await teamAPI.adminRechargeTeam(rechargeTarget.value.id, rechargeForm.value.amount, rechargeForm.value.note)
    appStore.showSuccess(t('team.adminTeams.recharged'))
    showRechargeDialog.value = false
    load()
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
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

onMounted(load)
</script>
