/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { UserQuotaDialog } from '../user-quota-dialog'

const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en }, initAsync: false })

afterEach(() => {
  cleanup()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

test('adding to a bucket previews debt repayment before its net credit', async () => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <UserQuotaDialog
        open
        onOpenChange={() => undefined}
        userId={1}
        currentQuota={-500_000}
        buckets={{
          topup: 0,
          aff_rebate: 0,
          invite_bonus: 0,
          gift: 0,
          debt: -500_000,
        }}
        onSuccess={() => undefined}
      />
    </I18nextProvider>
  )
  const amount = screen.getByRole('spinbutton')
  await userEvent.setup().type(amount, '0.5')

  expect(screen.getByText(/Current quota:/)).toHaveTextContent(
    'Current quota: $0 +$0 = $0'
  )
  expect(screen.getByText(/Outstanding Debt:/)).toHaveTextContent(
    'Outstanding Debt: -$1 +$0.5 = -$0.5'
  )
  expect(screen.getByText(/Total Balance:/)).toHaveTextContent(
    'Total Balance: -$1 → -$0.5'
  )
})
