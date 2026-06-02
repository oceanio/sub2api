<!--
  Fork addition (CLAUDE.md §5): isolated in a fork/ subdir so the upstream
  OpsErrorDetailModal.vue only carries a 1-line hook and stays merge-clean.

  Links an ops error record to its request debug log. usage_logs / request_debug_logs
  are keyed by ResolveCorrelationID, which — because the ClientRequestID middleware
  always sets a client id — resolves to "client:<client_request_id>". So we look the
  debug log up by that key (falling back to the raw request_id), reusing the existing
  DebugLogModal viewer and its /admin/usage/:request_id/debug-log endpoint.
-->
<template>
  <div v-if="resolvedKey">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 rounded-lg bg-primary-50 px-3 py-1.5 text-xs font-bold text-primary-700 ring-1 ring-inset ring-primary-600/20 transition-colors hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-300 dark:ring-primary-500/30 dark:hover:bg-primary-900/50"
      @click="open"
    >
      <Icon name="document" size="xs" :stroke-width="2" />
      <span>{{ label }}</span>
    </button>

    <!-- DebugLogModal emits `close` (not `update:show`), so bind @close. -->
    <DebugLogModal :show="showModal" :request-id="resolvedKey" @close="showModal = false" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import DebugLogModal from '@/components/admin/usage/DebugLogModal.vue'

interface Props {
  clientRequestId?: string | null
  requestId?: string | null
}

const props = defineProps<Props>()
const { locale } = useI18n()

const showModal = ref(false)

// debug_log / usage_logs 落库用的是 ResolveCorrelationID:client 优先 → "client:<id>"，
// 否则回退到原始 request_id。ops 错误记录里存的是未加前缀的 client_request_id。
const resolvedKey = computed(() => {
  const cid = (props.clientRequestId || '').trim()
  if (cid) return `client:${cid}`
  return (props.requestId || '').trim()
})

const label = computed(() => (locale.value.startsWith('zh') ? '查看调试日志' : 'View debug log'))

function open() {
  showModal.value = true
}
</script>
