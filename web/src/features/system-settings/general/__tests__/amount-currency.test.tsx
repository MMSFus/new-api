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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { QuotaSettingsSection } from '../quota-settings-section'

// 1 USD = 8 CNY and 500000 quota per USD keep every conversion exact.
function Fixture(props: {
  rewards?: number
  trust?: number
  amountCurrency?: string
}) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <QuotaSettingsSection
          defaultValues={{
            QuotaForNewUser: props.rewards ?? 500000,
            QuotaForInviter: 0,
            QuotaForInvitee: 0,
            TopUpLink: '',
            quota_setting: {
              enable_free_model_pre_consume: true,
              trust_quota_usd: props.trust ?? 10,
              pre_consume_multiplier: 1,
              amount_currency: props.amountCurrency ?? '{}',
            },
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

async function renderSettings(props: Parameters<typeof Fixture>[0] = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: () => <Fixture {...props} /> }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('spinbutton', { name: 'New User Quota' })
}

async function chooseCurrency(field: string, option: string) {
  await userEvent.click(screen.getByRole('combobox', { name: field }))
  await userEvent.click(await screen.findByRole('option', { name: option }))
}

function saved() {
  return vi
    .mocked(api.put)
    .mock.calls.map(([, body]) => body as { key: string; value: unknown })
}

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 8,
      quotaPerUnit: 500000,
    },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

describe('switching an amount field currency', () => {
  test('switching a quota reward to CNY converts quota units to the same CNY value', async () => {
    const input = await renderSettings({ rewards: 500000 })
    await chooseCurrency('New User Quota currency', 'Chinese yuan (CNY)')
    expect(input).toHaveValue(8)
    expect(screen.getByText('≈ $1')).toBeInTheDocument()
  })

  test('switching the threshold to CNY converts its USD amount', async () => {
    await renderSettings({ trust: 10 })
    await chooseCurrency('Bypass threshold currency', 'Chinese yuan (CNY)')
    expect(
      screen.getByRole('spinbutton', {
        name: 'Wallet pre-consume bypass threshold (CNY)',
      })
    ).toHaveValue(80)
  })

  test('switching a CNY reward back to USD rounds to whole quota units', async () => {
    // 0.33 CNY = 0.04125 USD = 20625 quota.
    const input = await renderSettings({
      rewards: 0.33,
      amountCurrency: '{"QuotaForNewUser":"CNY"}',
    })
    expect(input).toHaveValue(0.33)
    await chooseCurrency('New User Quota currency', 'US dollar (USD)')
    expect(input).toHaveValue(20625)
  })

  test('a non-terminating conversion back to USD still saves an integer for Atoi', async () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        usdExchangeRate: 7.3,
        quotaPerUnit: 500000,
      },
    })
    const input = await renderSettings({
      rewards: 1,
      amountCurrency: '{"QuotaForNewUser":"CNY"}',
    })
    await chooseCurrency('New User Quota currency', 'US dollar (USD)')
    // 1 / 7.3 × 500000 = 68493.15… → 68493
    expect(input).toHaveValue(68493)
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(saved()).toContainEqual({ key: 'QuotaForNewUser', value: 68493 })
    )
  })
})

describe('saving amount currencies', () => {
  test('saves the mark before the converted amount', async () => {
    await renderSettings({ rewards: 500000 })
    await chooseCurrency('New User Quota currency', 'Chinese yuan (CNY)')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(saved()).toHaveLength(2))
    expect(saved()).toEqual([
      {
        key: 'quota_setting.amount_currency',
        value: '{"QuotaForNewUser":"CNY"}',
      },
      { key: 'QuotaForNewUser', value: 8 },
    ])
  })

  test('clearing the last mark saves an empty map rather than a USD entry', async () => {
    await renderSettings({
      rewards: 8,
      amountCurrency: '{"QuotaForNewUser":"CNY"}',
    })
    await chooseCurrency('New User Quota currency', 'US dollar (USD)')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(saved()).toHaveLength(2))
    expect(saved()[0]).toEqual({
      key: 'quota_setting.amount_currency',
      value: '{}',
    })
    expect(saved()[1]).toEqual({ key: 'QuotaForNewUser', value: 500000 })
  })

  test('a rejected mark stops the amount from being saved in the new unit', async () => {
    vi.mocked(api.put).mockResolvedValueOnce({
      data: {
        success: false,
        message: 'The selected currency has no valid exchange rate',
      },
    })
    await renderSettings({ rewards: 500000 })
    await chooseCurrency('New User Quota currency', 'Chinese yuan (CNY)')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(saved()).toHaveLength(1))
    expect(saved()[0].key).toBe('quota_setting.amount_currency')
  })

  test('a decimal reward without a currency mark is rejected before saving', async () => {
    const input = await renderSettings({ rewards: 500000 })
    fireEvent.change(input, { target: { value: '0.5' } })
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(
      await screen.findByText('Quota must be a whole number')
    ).toBeInTheDocument()
    expect(api.put).not.toHaveBeenCalled()
  })

  test('a decimal reward written in CNY is saved as is', async () => {
    const input = await renderSettings({
      rewards: 8,
      amountCurrency: '{"QuotaForNewUser":"CNY"}',
    })
    fireEvent.change(input, { target: { value: '0.5' } })
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(saved()).toEqual([{ key: 'QuotaForNewUser', value: 0.5 }])
    )
  })
})

describe('display types without a site currency', () => {
  test('a USD site offers only USD and flags an existing CNY mark', async () => {
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
    })
    await renderSettings({
      trust: 80,
      amountCurrency: '{"trust_quota_usd":"CNY"}',
    })
    expect(
      screen.getByText(/which is not the current display currency/)
    ).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('combobox', { name: 'New User Quota currency' })
    )
    expect(
      (await screen.findAllByRole('option')).map((item) => item.textContent)
    ).toEqual(['US dollar (USD)'])
  })
})
