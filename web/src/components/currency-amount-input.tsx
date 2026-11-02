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
import type { ReactNode, Ref } from 'react'
import { useTranslation } from 'react-i18next'

import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toIntlLocale } from '@/i18n/languages'
import {
  amountCurrencyRate,
  amountCurrencySymbol,
  amountCurrencyUSDFactor,
  formatUSDAmount,
  siteAmountCurrency,
  type AmountCurrency,
  type CurrencyFormatOptions,
} from '@/lib/currency'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

/*
 * Amounts that administrators may write in USD or in the site currency
 * (model prices, quota rewards). The existing PricingCurrencySelector only
 * switches how USD prices are displayed and stores nothing, so it cannot
 * select the stored currency; these pieces do.
 */

type CurrencyOption = {
  value: AmountCurrency
  label: string
  disabled: boolean
}

function useCurrencyOptions(value: AmountCurrency): CurrencyOption[] {
  const { t } = useTranslation()
  const config = useSystemConfigStore((state) => state.config.currency)
  const site = siteAmountCurrency(config.quotaDisplayType)
  const codes: AmountCurrency[] = ['USD']
  if (site) codes.push(site)
  // A stale mark stays visible so the stored value is never misrepresented.
  if (value !== 'USD' && value !== site) codes.push(value)
  return codes.map((code) => {
    let label = t('US dollar (USD)')
    if (code === 'CNY') label = t('Chinese yuan (CNY)')
    if (code === 'CUSTOM') {
      label = t('Custom currency ({{symbol}})', {
        symbol: amountCurrencySymbol(code, config),
      })
    }
    return {
      value: code,
      label,
      // A new mark needs a usable rate; an existing one can be kept.
      disabled: code !== value && amountCurrencyRate(code, config) === null,
    }
  })
}

export function AmountCurrencySelect(props: {
  value: AmountCurrency
  onValueChange: (currency: AmountCurrency) => void
  id?: string
  'aria-label'?: string
  disabled?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const options = useCurrencyOptions(props.value)
  return (
    <Select
      items={options.map((option) => ({
        value: option.value,
        label: option.label,
      }))}
      value={props.value}
      disabled={props.disabled}
      onValueChange={(next) => {
        const option = options.find((item) => item.value === next)
        if (option && !option.disabled && option.value !== props.value) {
          props.onValueChange(option.value)
        }
      }}
    >
      <SelectTrigger
        id={props.id}
        aria-label={props['aria-label'] ?? t('Currency')}
        className={cn('w-full sm:w-56', props.className)}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false}>
        {options.map((option) => (
          <SelectItem
            key={option.value}
            value={option.value}
            disabled={option.disabled}
          >
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Explains a mark that no longer matches the display type or has no rate. */
export function AmountCurrencyNotice(props: {
  currency: AmountCurrency
  id?: string
  className?: string
}) {
  const { t } = useTranslation()
  const config = useSystemConfigStore((state) => state.config.currency)
  if (props.currency === 'USD') return null
  let message = ''
  if (props.currency !== siteAmountCurrency(config.quotaDisplayType)) {
    message = t(
      'This amount is written in {{currency}}, which is not the current display currency. It is still converted to USD at the {{currency}} exchange rate.',
      { currency: props.currency }
    )
  } else if (amountCurrencyRate(props.currency, config) === null) {
    message = t('The selected currency has no valid exchange rate')
  }
  if (!message) return null
  return (
    <p
      id={props.id}
      data-slot='amount-currency-notice'
      className={cn(
        'text-xs text-amber-700 dark:text-amber-300',
        props.className
      )}
    >
      {message}
    </p>
  )
}

/** "≈ $1.37": the live USD value of an amount written in another currency. */
export function CurrencyUSDReference(props: {
  amount: number | string | null | undefined
  currency: AmountCurrency
  formatOptions?: CurrencyFormatOptions
  className?: string
}) {
  const { i18n } = useTranslation()
  const config = useSystemConfigStore((state) => state.config.currency)
  if (props.currency === 'USD') return null
  if (props.amount === '' || props.amount == null) return null
  const amount = Number(props.amount)
  const factor = amountCurrencyUSDFactor(props.currency, config)
  if (factor === null || !Number.isFinite(amount)) return null
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <span
      data-slot='currency-usd-reference'
      className={cn(
        'text-muted-foreground text-xs tabular-nums',
        props.className
      )}
    >
      ≈{' '}
      {formatUSDAmount(amount * factor, {
        digitsLarge: 2,
        digitsSmall: 4,
        abbreviate: false,
        ...props.formatOptions,
        locale,
      })}
    </span>
  )
}

type CurrencyAmountInputProps = {
  value: number | ''
  onValueChange: (value: number | '') => void
  currency: AmountCurrency
  /** Omit to show the currency without letting the user change it. */
  onCurrencyChange?: (currency: AmountCurrency) => void
  currencyLabel?: string
  /** Replaces the currency symbol; `null` shows no prefix. */
  prefix?: ReactNode | null
  id?: string
  name?: string
  ref?: Ref<HTMLInputElement>
  onBlur?: () => void
  min?: number
  step?: number | 'any'
  placeholder?: string
  disabled?: boolean
  'aria-describedby'?: string
  'aria-invalid'?: boolean
  className?: string
}

/** Amount input with a currency switcher and its live USD reference. */
export function CurrencyAmountInput(props: CurrencyAmountInputProps) {
  const config = useSystemConfigStore((state) => state.config.currency)
  const prefix =
    props.prefix === undefined
      ? amountCurrencySymbol(props.currency, config)
      : props.prefix
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', props.className)}>
      <div className='flex min-w-0 flex-col gap-2 sm:flex-row'>
        <InputGroup className='min-w-0 flex-1'>
          {prefix !== null && <InputGroupAddon>{prefix}</InputGroupAddon>}
          <InputGroupInput
            ref={props.ref}
            id={props.id}
            name={props.name}
            type='number'
            inputMode='decimal'
            min={props.min ?? 0}
            step={props.step ?? 'any'}
            placeholder={props.placeholder}
            disabled={props.disabled}
            aria-describedby={props['aria-describedby']}
            aria-invalid={props['aria-invalid'] || undefined}
            value={props.value}
            onBlur={props.onBlur}
            onChange={(event) => {
              const next = event.currentTarget.valueAsNumber
              props.onValueChange(Number.isNaN(next) ? '' : next)
            }}
          />
        </InputGroup>
        {props.onCurrencyChange && (
          <AmountCurrencySelect
            value={props.currency}
            onValueChange={props.onCurrencyChange}
            aria-label={props.currencyLabel}
            disabled={props.disabled}
            className='sm:w-48'
          />
        )}
      </div>
      <CurrencyUSDReference amount={props.value} currency={props.currency} />
      <AmountCurrencyNotice currency={props.currency} />
    </div>
  )
}
