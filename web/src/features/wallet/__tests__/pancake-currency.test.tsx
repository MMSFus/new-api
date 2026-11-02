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
import { act, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { INTERFACE_LANGUAGE_OPTIONS, toIntlLocale } from '@/i18n/languages'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { formatSettlementCurrencyAmount } from '@/lib/currency'

import { RechargeFormCard } from '../components/recharge-form-card'
import { PAYMENT_TYPES } from '../constants'
import { getPaymentSettlementCurrency } from '../lib'
import type { TopupInfo } from '../types'

const topupInfo: TopupInfo = {
  enable_online_topup: false,
  enable_stripe_topup: false,
  enable_waffo_pancake_topup: true,
  waffo_pancake_currency: 'JPY',
  pay_methods: [],
  min_topup: 1,
  stripe_min_topup: 1,
  amount_options: [],
  discount: {},
}

const i18n = createInstance()

beforeEach(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: false,
    nsSeparator: false,
    resources: { en, zh },
    interpolation: { escapeValue: false },
  })
})

function renderCard(paymentAmount: number, paymentCurrency?: string) {
  render(
    <I18nextProvider i18n={i18n}>
      <RechargeFormCard
        topupInfo={topupInfo}
        presetAmounts={[]}
        selectedPreset={null}
        topupAmount={10}
        paymentAmount={paymentAmount}
        paymentCurrency={paymentCurrency}
        enableWaffoPancakeTopup
        calculating={false}
        paymentLoading={null}
        redemptionCode=''
        redeeming={false}
        onSelectPreset={vi.fn()}
        onTopupAmountChange={vi.fn()}
        onPaymentMethodSelect={vi.fn()}
        onRedemptionCodeChange={vi.fn()}
        onRedeem={vi.fn()}
      />
    </I18nextProvider>
  )
  return screen.getByText(i18n.t('Amount to pay:')).nextElementSibling
}

describe('Waffo Pancake settlement currency on the wallet page', () => {
  it('uses the configured Pancake currency only for the Pancake payment type', () => {
    expect(
      getPaymentSettlementCurrency(PAYMENT_TYPES.WAFFO_PANCAKE, topupInfo)
    ).toBe('JPY')
    expect(
      getPaymentSettlementCurrency(PAYMENT_TYPES.WAFFO_PANCAKE, {
        ...topupInfo,
        waffo_pancake_currency: undefined,
      })
    ).toBe('USD')
    expect(
      getPaymentSettlementCurrency(PAYMENT_TYPES.STRIPE, topupInfo)
    ).toBeUndefined()
  })

  it('shows the Pancake amount in JPY without decimals and follows language changes', async () => {
    const amount = renderCard(1500, 'JPY')

    expect(amount).toHaveTextContent(
      formatSettlementCurrencyAmount(1500, 'JPY', { locale: 'en' })
    )
    expect(amount?.textContent).toMatch(/1,500/)
    expect(amount?.textContent).not.toMatch(/\.\d/)

    await act(() => i18n.changeLanguage('zh'))
    expect(amount).toHaveTextContent(
      formatSettlementCurrencyAmount(1500, 'JPY', { locale: 'zh' })
    )
  })

  it('keeps the plain amount when the gateway has no settlement currency', () => {
    const amount = renderCard(12.5)

    expect(amount).toHaveTextContent('12.5')
    expect(amount?.textContent).not.toMatch(/[$¥€]/)
  })

  it.each(INTERFACE_LANGUAGE_OPTIONS.map((lang) => lang.code))(
    'formats settlement amounts with each currency minor unit in the %s interface language',
    (code) => {
      const locale = toIntlLocale(code)
      expect(locale).toBeDefined()

      const jpy = formatSettlementCurrencyAmount(1234, 'JPY', { locale })
      const bhd = formatSettlementCurrencyAmount(1.5, 'BHD', { locale })

      expect(jpy).not.toMatch(/\d[.,]\d{1,2}(?!\d)/)
      expect(bhd).toMatch(/1[.,]500/)
    }
  )

  it('falls back to the number plus the code for an unknown currency or locale', () => {
    expect(
      formatSettlementCurrencyAmount(9.5, 'not-a-code', { locale: 'en' })
    ).toBe('9.5 NOT-A-CODE')
    expect(
      formatSettlementCurrencyAmount(9.5, 'USD', {
        locale: toIntlLocale('zz_bad'),
      })
    ).toMatch(/9\.50/)
    expect(formatSettlementCurrencyAmount(Number.NaN, 'USD')).toBe('-')
  })
})
