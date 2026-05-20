<template>
  <div class="flex min-w-0 flex-1 items-start justify-between gap-3">
    <!-- Left: name + description -->
    <div
      class="flex min-w-0 flex-1 flex-col items-start"
      :title="description || undefined"
    >
      <!-- Row 1: platform badge (name bold) -->
      <GroupBadge
        :name="name"
        :platform="platform"
        :subscription-type="subscriptionType"
        :show-rate="false"
        class="groupOptionItemBadge"
      />
      <!-- Row 2: description with top spacing -->
      <span
        v-if="description"
        class="mt-1.5 w-full text-left text-xs leading-relaxed text-gray-500 dark:text-gray-400 line-clamp-2"
      >
        {{ description }}
      </span>
    </div>

    <!-- Right: rate pill + checkmark (vertically centered to first row) -->
    <div class="flex shrink-0 items-center gap-2 pt-0.5">
      <!-- Discount / multiplier pill (platform color)，后缀统一拼接 -->
      <span v-if="rateMultiplier !== undefined" :class="['inline-flex items-center whitespace-nowrap rounded-full px-3 py-1 text-xs font-semibold', ratePillClass]">
        <template v-if="hasCustomRate">
          <span class="mr-1 line-through opacity-50">{{ formatDiscount(rateMultiplier) }} {{ rateUnit }}</span>
          <span class="font-bold">{{ formatDiscount(userRateMultiplier) }} {{ rateUnit }}</span>
        </template>
        <template v-else>
          {{ formatDiscount(rateMultiplier) }} {{ rateUnit }}
        </template>
      </span>
      <!-- Checkmark -->
      <svg
        v-if="showCheckmark && selected"
        class="h-4 w-4 shrink-0 text-primary-600 dark:text-primary-400"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
        stroke-width="2"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
      </svg>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import GroupBadge from './GroupBadge.vue'
import type { SubscriptionType, GroupPlatform } from '@/types'

interface Props {
  name: string
  platform: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  userRateMultiplier?: number | null
  description?: string | null
  selected?: boolean
  showCheckmark?: boolean
  /** 是否使用"折扣"显示模式；关闭时显示原始倍率 Nx */
  displayDiscount?: boolean
  /** 1 USD = exchangeRate 本币；displayDiscount 开启且 >0 时才用 */
  exchangeRate?: number
}

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  selected: false,
  showCheckmark: true,
  userRateMultiplier: null,
  displayDiscount: false,
  exchangeRate: undefined
})

// Whether user has a custom rate different from default
const hasCustomRate = computed(() => {
  return (
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier
  )
})

// 当前是否真正以"折扣"形式渲染（开关开启 + 汇率存在）；否则回退到倍率
const useDiscount = computed(() => {
  return props.displayDiscount === true && !!props.exchangeRate && props.exchangeRate > 0
})

// 后缀文案，与 formatDiscount 渲染模式保持同步
const rateUnit = computed(() => (useDiscount.value ? '折扣' : '倍率'))

// 按当前 useDiscount 渲染：倍率（Nx）或 折扣（N%）；不附后缀，后缀在外层模板统一拼接
const formatDiscount = (multiplier: number | null | undefined): string => {
  if (multiplier === null || multiplier === undefined) return ''
  if (!useDiscount.value) return `${multiplier}x`
  const discount = Math.round((multiplier / (props.exchangeRate as number)) * 100)
  return `${discount}%`
}

// Rate pill color matches platform badge color
const ratePillClass = computed(() => {
  switch (props.platform) {
    case 'anthropic':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400'
    case 'openai':
      return 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400'
    case 'gemini':
      return 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400'
    default: // antigravity and others
      return 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-400'
  }
})
</script>

<style scoped>
/* Bold the group name inside GroupBadge when used in dropdown option */
.groupOptionItemBadge :deep(span.truncate) {
  font-weight: 600;
}
</style>
