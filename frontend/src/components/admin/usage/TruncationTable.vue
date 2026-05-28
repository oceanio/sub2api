<template>
  <div class="overflow-hidden rounded-md border border-amber-300 dark:border-amber-900/50">
    <table class="min-w-full divide-y divide-amber-300 text-xs dark:divide-amber-900/40">
      <thead class="bg-amber-100/60 dark:bg-amber-900/30">
        <tr>
          <th class="px-3 py-1.5 text-left font-medium text-amber-900 dark:text-amber-200">
            {{ t('admin.usage.debugLogModal.truncationFieldPath') }}
          </th>
          <th class="px-3 py-1.5 text-right font-medium text-amber-900 dark:text-amber-200">
            {{ t('admin.usage.debugLogModal.truncationFieldOriginal') }}
          </th>
          <th class="px-3 py-1.5 text-right font-medium text-amber-900 dark:text-amber-200">
            {{ t('admin.usage.debugLogModal.truncationFieldCut') }}
          </th>
          <th class="px-3 py-1.5 text-center font-medium text-amber-900 dark:text-amber-200">
            {{ t('admin.usage.debugLogModal.truncationFieldMode') }}
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-amber-200 dark:divide-amber-900/30">
        <tr v-for="(row, idx) in rows" :key="idx">
          <td class="break-all px-3 py-1.5 font-mono text-amber-900 dark:text-amber-100">{{ row.path }}</td>
          <td class="px-3 py-1.5 text-right text-amber-800 dark:text-amber-200">{{ formatBytes(row.original_bytes) }}</td>
          <td class="px-3 py-1.5 text-right text-red-700 dark:text-red-300">-{{ formatBytes(row.cut_bytes) }}</td>
          <td class="px-3 py-1.5 text-center text-amber-800 dark:text-amber-200">
            {{ row.mode === 'head_tail'
              ? t('admin.usage.debugLogModal.truncationModeHeadTail')
              : t('admin.usage.debugLogModal.truncationModeTail') }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { formatBytes } from '@/utils/format'
import type { DebugLogFieldCut } from '@/api/admin/usage'

defineProps<{ rows: DebugLogFieldCut[] }>()

const { t } = useI18n()
</script>
