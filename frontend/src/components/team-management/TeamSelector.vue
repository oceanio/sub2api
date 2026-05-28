<template>
  <div class="flex items-center gap-2">
    <label class="text-sm font-medium text-gray-600 dark:text-dark-300">
      {{ t('team.selector.label') }}
    </label>
    <Select
      :model-value="modelValue"
      :options="options"
      :searchable="searchable"
      :placeholder="placeholder"
      class="min-w-[260px]"
      @update:modelValue="onChange"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { teamAPI, type Team } from '@/api/team'
import { useTeamStore } from '@/stores/team'
import Select from '@/components/common/Select.vue'

interface Props {
  modelValue: number | null
  source: 'admin' | 'team_admin'
}

const props = defineProps<Props>()
const emit = defineEmits<{ 'update:modelValue': [value: number] }>()

const { t } = useI18n()
const teamStore = useTeamStore()

// For team_admin we read from the shared store (populated once on login).
// For sys admin we still need to fetch since they can manage any team.
const adminTeams = ref<Team[]>([])

const teams = computed<Team[]>(() => {
  if (props.source === 'team_admin') return teamStore.managedTeams
  return adminTeams.value
})

const options = computed(() =>
  teams.value.map(team => ({ value: team.id, label: team.name }))
)

const searchable = computed(() => teams.value.length > 5 || props.source === 'admin')

const placeholder = computed(() =>
  props.source === 'admin' ? t('team.selector.searchAll') : t('team.selector.placeholder')
)

async function loadAdminTeams() {
  if (props.source !== 'admin') return
  try {
    const res = await teamAPI.adminListTeams(1, 500)
    adminTeams.value = (res as any).items ?? []
  } catch {
    adminTeams.value = []
  }
}

function onChange(value: number | string | boolean | null) {
  if (typeof value === 'number') {
    emit('update:modelValue', value)
  }
}

onMounted(loadAdminTeams)
watch(() => props.source, loadAdminTeams)
defineExpose({ refresh: loadAdminTeams })
</script>
