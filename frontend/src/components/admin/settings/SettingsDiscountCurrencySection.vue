<!--
  Fork addition: discount-display + USD exchange-rate settings card. Extracted
  from SettingsView.vue so the parent view's diff vs upstream stays minimal.
-->
<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ localText('折扣显示与汇率', 'Discount Display & Exchange Rate') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ localText('开启后，用户在选择分组时看到的是折扣百分比；关闭则显示原始倍率（如 3.4x）。', 'When enabled, users see a discount percentage when picking groups; when disabled, the raw multiplier (e.g. 3.4x) is shown.') }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ localText('启用折扣显示', 'Display Discount Instead of Multiplier') }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ localText('折扣 = 倍率 / 汇率 × 100%。例如汇率 7.2、倍率 3.6 → 50% 折扣。', 'Discount = Multiplier / Rate × 100%. E.g. rate 7.2, multiplier 3.6 → 50% discount.') }}
          </p>
        </div>
        <Toggle v-model="form.display_discount_enabled" />
      </div>

      <div v-if="form.display_discount_enabled" class="space-y-4 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ localText('本币币种', 'Local Currency') }}
          </label>
          <select v-model="form.local_currency" class="input">
            <option value="CNY">{{ localText('人民币 (CNY)', 'Chinese Yuan (CNY)') }}</option>
            <option value="HKD">{{ localText('港币 (HKD)', 'Hong Kong Dollar (HKD)') }}</option>
          </select>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ localText('选择本币后，"从网络获取"按钮会拉取 1 USD 对应的该币种汇率。', 'After selecting a local currency, "Fetch from Network" pulls the latest 1 USD → currency rate.') }}
          </p>
        </div>

        <div class="flex items-end gap-3">
          <div class="flex-1">
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ localText(`汇率 (1 USD = ? ${form.local_currency})`, `Exchange Rate (1 USD = ? ${form.local_currency})`) }}
            </label>
            <input
              v-model.number="form.usd_exchange_rate"
              type="number"
              step="0.01"
              min="0.01"
              class="input"
              placeholder="7.20"
            />
          </div>
          <button
            type="button"
            class="btn btn-secondary shrink-0"
            :disabled="refreshing"
            @click="handleRefresh"
          >
            <span v-if="refreshing" class="mr-1.5 inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent"></span>
            {{ localText('从网络获取', 'Fetch from Network') }}
          </button>
        </div>
        <p v-if="(form.usd_exchange_rate ?? 0) > 0" class="text-xs text-gray-500 dark:text-gray-400">
          {{ localText(`当前汇率：1 USD = ${form.usd_exchange_rate} ${form.local_currency}`, `Current rate: 1 USD = ${form.usd_exchange_rate} ${form.local_currency}`) }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import Toggle from '@/components/common/Toggle.vue'

interface DiscountCurrencyFormSlice {
  display_discount_enabled: boolean
  local_currency: string
  usd_exchange_rate: number
}

const props = defineProps<{ form: DiscountCurrencyFormSlice }>()

const { locale } = useI18n()
const appStore = useAppStore()

function localText(zh: string, en: string): string {
  return locale.value === 'zh' ? zh : en
}

const refreshing = ref(false)

async function handleRefresh() {
  refreshing.value = true
  try {
    const currency = props.form.local_currency === 'HKD' ? 'HKD' : 'CNY'
    const result = await adminAPI.settings.refreshExchangeRate(currency)
    props.form.local_currency = result.local_currency
    props.form.usd_exchange_rate = result.usd_exchange_rate
    appStore.showSuccess(
      localText(
        `汇率已更新: 1 USD = ${result.usd_exchange_rate} ${result.local_currency}`,
        `Exchange rate updated: 1 USD = ${result.usd_exchange_rate} ${result.local_currency}`,
      ),
    )
  } catch {
    appStore.showError(
      localText('获取汇率失败，请稍后重试', 'Failed to fetch exchange rate, please try again'),
    )
  } finally {
    refreshing.value = false
  }
}
</script>
