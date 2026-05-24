<template>
  <div class="space-y-4 p-6">
    <div class="card">
      <div class="card-header px-6 py-4">
        <h3 class="card-title">{{ t('team.tags.title') }}</h3>
      </div>
      <div class="p-6">
        <div class="mb-4 flex flex-wrap gap-2">
          <span
            v-for="(tag, i) in tags"
            :key="i"
            class="flex items-center gap-1 rounded-full bg-primary-100 px-3 py-1 text-sm text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
          >
            {{ tag }}
            <button @click="removeTag(i)" class="ml-1 text-primary-500 hover:text-red-500">×</button>
          </span>
          <span v-if="tags.length === 0" class="text-sm text-gray-400">{{ t('team.tags.noTags') }}</span>
        </div>

        <div class="flex gap-2">
          <input v-model="newTag" type="text" class="input flex-1" :placeholder="t('team.tags.tagName')" @keydown.enter.prevent="addTag" />
          <button class="btn btn-secondary" @click="addTag" :disabled="!newTag.trim()">{{ t('team.tags.addTag') }}</button>
        </div>

        <div class="mt-4 flex justify-end">
          <button class="btn btn-primary" :disabled="saving || !dirty" @click="save">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { teamAPI } from '@/api/team'

interface Props {
  teamId: number
  source: 'admin' | 'team_admin'
}
const props = defineProps<Props>()

const { t } = useI18n()
const appStore = useAppStore()

const tags = ref<string[]>([])
const originalTags = ref<string[]>([])
const newTag = ref('')
const saving = ref(false)

const dirty = computed(() => JSON.stringify(tags.value) !== JSON.stringify(originalTags.value))

function addTag() {
  const v = newTag.value.trim()
  if (!v) return
  if (tags.value.includes(v)) {
    newTag.value = ''
    return
  }
  tags.value.push(v)
  newTag.value = ''
}

function removeTag(i: number) {
  tags.value.splice(i, 1)
}

async function load() {
  try {
    const team = await teamAPI.getTeam(props.source, props.teamId)
    tags.value = [...(team.available_tags ?? [])]
    originalTags.value = [...tags.value]
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed to load team')
  }
}

async function save() {
  saving.value = true
  try {
    const team = await teamAPI.updateAvailableTags(props.source, props.teamId, tags.value)
    tags.value = [...(team.available_tags ?? [])]
    originalTags.value = [...tags.value]
    appStore.showSuccess(t('team.tags.tagAdded'))
  } catch (e: any) {
    appStore.showError(e?.message ?? 'Failed')
  } finally { saving.value = false }
}

watch(() => props.teamId, load, { immediate: true })
</script>
