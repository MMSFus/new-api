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
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { amountCurrencySymbol, siteAmountCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { formatDecimalBytes, formatTerabytes } from './lib'

/**
 * Formatters of the revenue tab. Revenue amounts are shown as entered in the
 * rules, never converted: a non-empty `currency_label` is their unit,
 * otherwise the site currency symbol.
 */
export function useRevenueFormat(currencyLabel: string | undefined) {
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const currency = useSystemConfigStore((state) => state.config.currency)
  const label = currencyLabel?.trim() ?? ''
  const symbol = amountCurrencySymbol(
    siteAmountCurrency(currency.quotaDisplayType) ?? 'USD',
    currency
  )
  const unit = label || symbol

  const revenue = (value: number): string => {
    const amount = formatNumber(value, locale)
    if (label) return `${amount} ${label}`
    // Multi-letter custom symbols read better apart, as in @/lib/currency.
    return symbol.length > 1 ? `${symbol} ${amount}` : `${symbol}${amount}`
  }

  return {
    locale,
    unit,
    revenue,
    count: (value: number) => formatNumber(value, locale),
    terabytes: (bytes: number) => `${formatTerabytes(bytes, locale)} TB`,
    bytes: (bytes: number) => formatDecimalBytes(bytes, locale),
  }
}

export type RevenueFormat = ReturnType<typeof useRevenueFormat>
