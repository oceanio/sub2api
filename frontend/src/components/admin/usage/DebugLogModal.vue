<template>
  <BaseDialog :show="show" :title="t('admin.usage.debugLogModal.title')" width="wide" @close="handleClose">
    <div v-if="loading" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      <Icon name="refresh" size="md" class="mr-2 animate-spin" />
      <span>{{ t('common.loading') }}</span>
    </div>

    <div v-else-if="loadError" class="space-y-3">
      <p class="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-300">
        {{ loadError }}
      </p>
    </div>

    <div v-else-if="data && !data.found" class="space-y-3">
      <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ t('admin.usage.debugLogModal.notFoundTitle') }}
      </p>
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.usage.debugLogModal.notFoundReasons') }}
      </p>
    </div>

    <div v-else-if="data" class="space-y-4">
      <!-- Metadata -->
      <div class="rounded-md border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-800/50">
        <h4 class="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
          {{ t('admin.usage.debugLogModal.meta') }}
        </h4>
        <dl class="grid grid-cols-1 gap-x-6 gap-y-1.5 text-xs sm:grid-cols-2">
          <div class="flex justify-between sm:block">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.requestId') }}</dt>
            <dd class="break-all font-mono text-gray-800 dark:text-gray-200">{{ data.request_id }}</dd>
          </div>
          <div class="flex justify-between sm:block">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.model') }}</dt>
            <dd class="font-mono text-gray-800 dark:text-gray-200">{{ data.model || '-' }}</dd>
          </div>
          <div class="flex justify-between sm:block">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.stream') }}</dt>
            <dd class="text-gray-800 dark:text-gray-200">{{ data.stream ? '✓' : '-' }}</dd>
          </div>
          <div class="flex justify-between sm:block">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.bodyBytes') }}</dt>
            <dd class="text-gray-800 dark:text-gray-200">
              {{ data.body_bytes ? formatBytes(data.body_bytes) : '-' }}
              <span v-if="data.truncated" class="ml-2 inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">
                ⚠ {{ t('admin.usage.debugLogModal.truncated') }}
              </span>
            </dd>
          </div>
          <div class="flex justify-between sm:block">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.createdAt') }}</dt>
            <dd class="text-gray-800 dark:text-gray-200">{{ formatTime(data.created_at) }}</dd>
          </div>
          <div class="flex justify-between sm:block">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.expiresAt') }}</dt>
            <dd class="text-gray-800 dark:text-gray-200">{{ formatTime(data.expires_at) }}</dd>
          </div>
        </dl>
      </div>

      <!-- Tabs -->
      <div class="border-b border-gray-200 dark:border-dark-700">
        <nav class="-mb-px flex gap-4">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            :class="[
              'border-b-2 px-1 pb-2 text-sm font-medium transition-colors',
              activeTab === tab.key
                ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
          </button>
        </nav>
      </div>

      <!-- Tab content -->
      <div v-show="activeTab === 'request'">
        <div class="mb-2 flex justify-end">
          <button
            type="button"
            class="text-xs text-primary-600 hover:underline dark:text-primary-400"
            :disabled="!hasRequestPayload"
            @click="copyContent(formattedRequest)"
          >
            {{ copiedTab === 'request' ? t('admin.usage.debugLogModal.copied') : t('admin.usage.debugLogModal.copyJson') }}
          </button>
        </div>
        <pre v-if="hasRequestPayload" class="max-h-96 overflow-auto rounded-md bg-gray-50 px-3 py-2 text-xs leading-relaxed text-gray-800 dark:bg-dark-800 dark:text-gray-200">{{ formattedRequest }}</pre>
        <p v-else class="px-1 py-2 text-xs italic text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.empty') }}</p>
      </div>

      <div v-show="activeTab === 'response'">
        <div class="mb-2 flex justify-end gap-2">
          <span v-if="data.response_text && !data.response_body" class="text-xs italic text-gray-500 dark:text-gray-400">
            {{ t('admin.usage.debugLogModal.rawText') }}
          </span>
          <button
            type="button"
            class="text-xs text-primary-600 hover:underline dark:text-primary-400"
            :disabled="!hasResponsePayload"
            @click="copyContent(formattedResponse)"
          >
            {{ copiedTab === 'response' ? t('admin.usage.debugLogModal.copied') : t('admin.usage.debugLogModal.copyJson') }}
          </button>
        </div>
        <pre v-if="hasResponsePayload" class="max-h-96 overflow-auto rounded-md bg-gray-50 px-3 py-2 text-xs leading-relaxed text-gray-800 dark:bg-dark-800 dark:text-gray-200">{{ formattedResponse }}</pre>
        <p v-else class="px-1 py-2 text-xs italic text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.empty') }}</p>
      </div>

      <div v-show="activeTab === 'headers'">
        <div v-if="headerEntries.length > 0" class="overflow-hidden rounded-md border border-gray-200 dark:border-dark-700">
          <table class="min-w-full divide-y divide-gray-200 text-xs dark:divide-dark-700">
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
              <tr v-for="[name, value] in headerEntries" :key="name" class="hover:bg-gray-50 dark:hover:bg-dark-800/40">
                <th class="w-1/3 break-all px-3 py-2 text-left align-top font-medium text-gray-700 dark:text-gray-300">{{ name }}</th>
                <td class="break-all px-3 py-2 align-top font-mono text-gray-600 dark:text-gray-400">{{ value }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="px-1 py-2 text-xs italic text-gray-500 dark:text-gray-400">{{ t('admin.usage.debugLogModal.empty') }}</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          type="button"
          class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
          @click="handleClose"
        >
          {{ t('admin.usage.debugLogModal.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminUsageAPI, type DebugLogResponse } from '@/api/admin/usage'
import { formatBytes, formatDateTime } from '@/utils/format'
import { useClipboard } from '@/composables/useClipboard'

interface Props {
  show: boolean
  requestId: string | null
}

interface Emits {
  (e: 'close'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()

type TabKey = 'request' | 'response' | 'headers'

const activeTab = ref<TabKey>('request')
const data = ref<DebugLogResponse | null>(null)
const loading = ref(false)
const loadError = ref<string | null>(null)
const copiedTab = ref<TabKey | null>(null)

const tabs = computed<{ key: TabKey; label: string }[]>(() => [
  { key: 'request', label: t('admin.usage.debugLogModal.tabRequest') },
  { key: 'response', label: t('admin.usage.debugLogModal.tabResponse') },
  { key: 'headers', label: t('admin.usage.debugLogModal.tabHeaders') }
])

function formatTime(value: string | undefined | null): string {
  if (!value) return '-'
  return formatDateTime(value) || value
}

const formattedRequest = computed(() => {
  const body = data.value?.request_body
  if (body == null) return ''
  try {
    return JSON.stringify(body, null, 2)
  } catch {
    return String(body)
  }
})

const formattedResponse = computed(() => {
  const body = data.value?.response_body
  if (body != null) {
    try {
      return JSON.stringify(body, null, 2)
    } catch {
      return String(body)
    }
  }
  return data.value?.response_text || ''
})

const hasRequestPayload = computed(() => formattedRequest.value.length > 0)
const hasResponsePayload = computed(() => formattedResponse.value.length > 0)

const headerEntries = computed<Array<[string, string]>>(() => {
  const h = data.value?.request_headers
  if (!h) return []
  return Object.entries(h).map(([k, v]) => [k, String(v)])
})

async function load(requestId: string) {
  loading.value = true
  loadError.value = null
  data.value = null
  try {
    data.value = await adminUsageAPI.getRequestDebugLog(requestId)
  } catch (err) {
    const message = err instanceof Error ? err.message : t('admin.usage.debugLogModal.loadFailed')
    loadError.value = message
  } finally {
    loading.value = false
  }
}

async function copyContent(text: string) {
  if (!text) return
  const ok = await copyToClipboard(text, t('admin.usage.debugLogModal.copied'))
  if (!ok) return
  copiedTab.value = activeTab.value
  setTimeout(() => {
    if (copiedTab.value === activeTab.value) copiedTab.value = null
  }, 1500)
}

function handleClose() {
  emit('close')
}

watch(
  () => [props.show, props.requestId] as const,
  ([show, requestId]) => {
    if (show && requestId) {
      activeTab.value = 'request'
      copiedTab.value = null
      void load(requestId)
    } else if (!show) {
      data.value = null
      loadError.value = null
    }
  },
  { immediate: true }
)
</script>
