<!--
  Fork addition: request-debug-log settings card. Extracted from SettingsView.vue
  so the parent view's diff vs upstream stays minimal even as origin/main churns
  the surrounding settings UI.

  The parent passes its reactive `form` object as a prop and the child mutates
  the debug-log fields directly — Vue 3 keeps the reactivity link intact, and
  this matches the inline pattern used elsewhere in the parent.
-->
<template>
  <div>
    <!-- Request Debug Log feature card -->
    <div class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.features.debugLog.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.features.debugLog.description') }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.features.debugLog.enabled') }}
            </label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.debugLog.enabledHint') }}
            </p>
          </div>
          <Toggle v-model="form.debug_request_log_enabled" />
        </div>

        <div v-if="form.debug_request_log_enabled" class="space-y-5">
          <div>
            <label class="input-label">
              {{ t('admin.settings.features.debugLog.ttlHours') }}
            </label>
            <input
              v-model.number="form.debug_request_log_ttl_hours"
              type="number"
              min="1"
              max="720"
              step="1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.debugLog.ttlHoursHint') }}
            </p>
          </div>

          <div>
            <label class="input-label">
              {{ t('admin.settings.features.debugLog.sampleRate') }}
            </label>
            <input
              v-model.number="form.debug_request_log_sample_rate"
              type="number"
              min="1"
              max="100"
              step="1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.debugLog.sampleRateHint') }}
            </p>
          </div>

          <div>
            <label class="input-label">
              {{ t('admin.settings.features.debugLog.bodyLimit') }}
            </label>
            <input
              v-model.number="form.debug_request_log_body_limit_bytes"
              type="number"
              min="0"
              max="1048576"
              step="1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.debugLog.bodyLimitHint') }}
            </p>
          </div>

          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.features.debugLog.redactHeaders') }}
              </label>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.features.debugLog.redactHeadersHint') }}
              </p>
            </div>
            <Toggle v-model="form.debug_request_log_redact_headers" @update:model-value="handleRedactHeadersChange" />
          </div>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :show="redactConfirmShow"
      :title="t('admin.settings.features.debugLog.redactConfirmTitle')"
      :message="t('admin.settings.features.debugLog.redactConfirmContent')"
      :confirm-text="t('admin.settings.features.debugLog.redactConfirmOk')"
      :cancel-text="t('admin.settings.features.debugLog.redactConfirmCancel')"
      danger
      @confirm="confirmDisableRedact"
      @cancel="cancelDisableRedact"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

interface DebugLogFormSlice {
  debug_request_log_enabled: boolean
  debug_request_log_ttl_hours: number
  debug_request_log_sample_rate: number
  debug_request_log_redact_headers: boolean
  debug_request_log_body_limit_bytes: number
}

const props = defineProps<{ form: DebugLogFormSlice }>()

const { t } = useI18n()

// Confirm dialog state for "disable redact_headers" — disabling header
// redaction is a security risk, so we require explicit user confirmation.
const redactConfirmShow = ref(false)
function handleRedactHeadersChange(next: boolean) {
  if (!next) {
    redactConfirmShow.value = true
  }
}
function confirmDisableRedact() {
  redactConfirmShow.value = false
}
function cancelDisableRedact() {
  redactConfirmShow.value = false
  props.form.debug_request_log_redact_headers = true
}
</script>
