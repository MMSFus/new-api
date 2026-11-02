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
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useState } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import type { AmountCurrency } from '@/lib/currency'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
  type CurrencyDisplayType,
} from '@/stores/system-config-store'

import {
  CurrencyAmountInput,
  CurrencyUSDReference,
} from '../currency-amount-input'

function setSite(
  quotaDisplayType: CurrencyDisplayType,
  overrides: Partial<typeof DEFAULT_CURRENCY_CONFIG> = {}
) {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType,
      usdExchangeRate: 7.3,
      customCurrencySymbol: '€',
      customCurrencyExchangeRate: 0.9,
      ...overrides,
    },
  })
}

function Fixture(props: {
  initial: AmountCurrency
  amount?: number
  onCurrencyChange?: (currency: AmountCurrency) => void
}) {
  const [currency, setCurrency] = useState(props.initial)
  const [value, setValue] = useState<number | ''>(props.amount ?? 10)
  return (
    <CurrencyAmountInput
      value={value}
      onValueChange={setValue}
      currency={currency}
      currencyLabel='Reward currency'
      onCurrencyChange={(next) => {
        props.onCurrencyChange?.(next)
        setCurrency(next)
      }}
    />
  )
}

async function openOptions() {
  await userEvent.click(
    screen.getByRole('combobox', { name: 'Reward currency' })
  )
  return screen.findAllByRole('option')
}

afterEach(async () => {
  cleanup()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  await act(() => i18next.changeLanguage('en'))
})

describe('currency options', () => {
  test('a CNY site offers USD and CNY and reports the chosen currency', async () => {
    setSite('CNY')
    const onCurrencyChange = vi.fn()
    render(<Fixture initial='USD' onCurrencyChange={onCurrencyChange} />)
    const options = await openOptions()
    expect(options.map((option) => option.textContent)).toEqual([
      'US dollar (USD)',
      'Chinese yuan (CNY)',
    ])
    await userEvent.click(
      screen.getByRole('option', { name: 'Chinese yuan (CNY)' })
    )
    expect(onCurrencyChange).toHaveBeenCalledWith('CNY')
    expect(
      screen.getByRole('combobox', { name: 'Reward currency' })
    ).toHaveTextContent('Chinese yuan (CNY)')
  })

  test('a custom-currency site labels the option with its symbol', async () => {
    setSite('CUSTOM')
    render(<Fixture initial='USD' />)
    expect((await openOptions()).map((option) => option.textContent)).toContain(
      'Custom currency (€)'
    )
  })

  test.each<CurrencyDisplayType>(['USD', 'TOKENS'])(
    'a %s display type hides the site currency option',
    async (displayType) => {
      setSite(displayType)
      render(<Fixture initial='USD' />)
      expect((await openOptions()).map((option) => option.textContent)).toEqual(
        ['US dollar (USD)']
      )
    }
  )

  test('an invalid site rate disables adding a new mark', async () => {
    setSite('CNY', { usdExchangeRate: 0 })
    render(<Fixture initial='USD' />)
    await openOptions()
    expect(
      screen.getByRole('option', { name: 'Chinese yuan (CNY)' })
    ).toHaveAttribute('aria-disabled', 'true')
  })
})

describe('mismatched marks', () => {
  test('a CNY mark on a USD display site stays selectable and shows a notice', async () => {
    setSite('USD')
    render(<Fixture initial='CNY' />)
    expect(
      screen.getByText(/which is not the current display currency/)
    ).toBeInTheDocument()
    expect((await openOptions()).map((option) => option.textContent)).toEqual([
      'US dollar (USD)',
      'Chinese yuan (CNY)',
    ])
  })

  test('a mark matching the display currency shows no notice', () => {
    setSite('CNY')
    render(<Fixture initial='CNY' />)
    expect(
      screen.queryByText(/which is not the current display currency/)
    ).not.toBeInTheDocument()
  })
})

describe('USD reference', () => {
  test('a CNY amount shows its live USD value', () => {
    setSite('CNY')
    render(<Fixture initial='CNY' amount={10} />)
    expect(screen.getByText('≈ $1.37')).toBeInTheDocument()
  })

  test('typing a new amount updates the reference', async () => {
    setSite('CNY')
    render(<Fixture initial='CNY' amount={10} />)
    const input = screen.getByRole('spinbutton')
    await userEvent.clear(input)
    await userEvent.type(input, '73')
    expect(screen.getByText('≈ $10')).toBeInTheDocument()
  })

  test('a USD amount shows no reference', () => {
    setSite('CNY')
    render(<Fixture initial='USD' amount={10} />)
    expect(screen.queryByText(/^≈/)).not.toBeInTheDocument()
  })

  test('an invalid rate shows no reference instead of guessing', () => {
    setSite('CNY', { usdExchangeRate: Number.NaN })
    render(<Fixture initial='CNY' amount={10} />)
    expect(screen.queryByText(/^≈/)).not.toBeInTheDocument()
  })
})

describe('reference number formatting by interface language', () => {
  async function renderIn(language: string) {
    if (!i18next.hasResourceBundle(language, 'translation')) {
      i18next.addResourceBundle(language, 'translation', {})
    }
    await act(() => i18next.changeLanguage(language))
    setSite('CNY', { usdExchangeRate: 1 })
    render(<CurrencyUSDReference amount={1234.5} currency='CNY' />)
  }
  // Exact text: French and Russian group with narrow no-break spaces.
  const referenceText = () =>
    document.querySelector('[data-slot="currency-usd-reference"]')?.textContent

  test.each([
    ['zhCN', 'zh-CN'],
    ['zhTW', 'zh-TW'],
    ['en', 'en'],
    ['fr', 'fr'],
    ['ru', 'ru'],
    ['ja', 'ja'],
    ['vi', 'vi'],
  ])('%s formats like the %s Intl locale', async (language, locale) => {
    await renderIn(language)
    const expected = new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: 'USD',
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(1234.5)
    expect(referenceText()).toBe(`≈ ${expected}`)
  })

  test('an unknown interface language falls back without throwing', async () => {
    await renderIn('not_a_locale!')
    expect(referenceText()).toBe('≈ $1,234.5')
  })

  test('switching language reformats the reference', async () => {
    await renderIn('en')
    expect(referenceText()).toBe('≈ $1,234.5')
    await act(() => i18next.changeLanguage('fr'))
    const french = new Intl.NumberFormat('fr', {
      style: 'currency',
      currency: 'USD',
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(1234.5)
    expect(referenceText()).toBe(`≈ ${french}`)
  })
})
