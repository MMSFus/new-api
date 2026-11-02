/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { formatPricingNumber } from '@/features/system-settings/models/pricing-format'
import { amountCurrencySymbol, type AmountCurrency } from '@/lib/currency'
import type { CurrencyConfig } from '@/stores/system-config-store'

export type PricingCurrency = {
  label: string
  symbol: string
  /** Display units per unit of the stored value. */
  exchangeRate: number
  /**
   * Currency the stored values are written in. Absent means USD. When set to
   * CNY or CUSTOM, the editor shows the stored values unchanged (rate 1).
   */
  code?: AmountCurrency
}

export const USD_PRICING_CURRENCY: PricingCurrency = {
  label: 'USD',
  symbol: '$',
  exchangeRate: 1,
}

export function getSitePricingCurrency(
  config: CurrencyConfig
): PricingCurrency | null {
  if (config.quotaDisplayType === 'CNY') {
    return { label: 'CNY', symbol: '¥', exchangeRate: config.usdExchangeRate }
  }
  if (config.quotaDisplayType === 'CUSTOM') {
    const symbol = config.customCurrencySymbol?.trim() || '¤'
    return {
      label: symbol,
      symbol,
      exchangeRate: config.customCurrencyExchangeRate,
    }
  }
  return null
}

export function isValidPricingCurrency(
  currency: PricingCurrency | null
): currency is PricingCurrency {
  return (
    currency !== null &&
    Number.isFinite(currency.exchangeRate) &&
    currency.exchangeRate > 0
  )
}

export function formatPricingAmount(
  value: string | number,
  currency = USD_PRICING_CURRENCY
): string {
  if (value === '') return ''
  const amount = Number(value) * currency.exchangeRate
  if (!Number.isFinite(amount)) return '—'
  return `${currency.symbol}${formatPricingNumber(amount)}`
}

/** Edit values exactly as stored in a model's price currency. */
export function storedPricingCurrency(
  code: Exclude<AmountCurrency, 'USD'>,
  config: CurrencyConfig
): PricingCurrency {
  const symbol = amountCurrencySymbol(code, config)
  return {
    label: code === 'CNY' ? 'CNY' : symbol,
    symbol,
    exchangeRate: 1,
    code,
  }
}

/** Label of the currency that raw expression coefficients are written in. */
export function pricingCoefficientLabel(currency: PricingCurrency): string {
  return currency.code && currency.code !== 'USD' ? currency.label : 'USD'
}
