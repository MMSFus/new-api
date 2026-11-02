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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import type { ExchangeRateSyncStatusValue } from '../exchange-rate-sync-status'
import { PricingSection } from '../pricing-section'

const neverSynced: ExchangeRateSyncStatusValue = {
  lastSyncedAt: 0,
  lastError: '',
  lastSource: '',
}

function Fixture(props: {
  autoSyncEnabled: boolean
  status: ExchangeRateSyncStatusValue
}) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <PricingSection
          defaultValues={{
            QuotaPerUnit: 500000,
            USDExchangeRate: 7.3,
            DisplayInCurrencyEnabled: true,
            DisplayTokenStatEnabled: true,
            general_setting: {
              quota_display_type: 'USD',
              custom_currency_symbol: '¤',
              custom_currency_exchange_rate: 1,
            },
            exchange_rate_setting: {
              auto_sync_enabled: props.autoSyncEnabled,
              sync_interval_hours: 6,
            },
          }}
          exchangeRateSyncStatus={props.status}
        />
      </SettingsPageProvider>
    </>
  )
}

async function renderPricing(
  autoSyncEnabled = false,
  status: ExchangeRateSyncStatusValue = neverSynced
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <Fixture autoSyncEnabled={autoSyncEnabled} status={status} />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('switch', {
    name: 'Automatically sync USD exchange rate',
  })
}

beforeEach(() => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        usd_exchange_rate: 7.12,
        last_synced_at: 1767225600,
        last_error: '',
        last_source: 'open.er-api.com',
      },
    },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

test('auto sync is off by default and hides the interval field', async () => {
  const toggle = await renderPricing()
  expect(toggle).toHaveAttribute('aria-checked', 'false')
  expect(
    screen.queryByRole('spinbutton', { name: 'Sync interval (hours)' })
  ).not.toBeInTheDocument()
})

test('turning auto sync on saves the switch as a sub-config option', async () => {
  const user = userEvent.setup()
  const toggle = await renderPricing()
  await user.click(toggle)
  expect(
    screen.getByRole('spinbutton', { name: 'Sync interval (hours)' })
  ).toHaveValue(6)
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith('/api/option/', {
      key: 'exchange_rate_setting.auto_sync_enabled',
      value: 'true',
    })
  )
})

test.each(['0', '169', '1.5'])(
  'interval "%s" outside 1-168 whole hours shows an error and blocks saving',
  async (value) => {
    await renderPricing(true)
    const input = screen.getByRole('spinbutton', {
      name: 'Sync interval (hours)',
    })
    fireEvent.change(input, { target: { value } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
    expect(api.put).not.toHaveBeenCalled()
  }
)

test('Sync Now posts to the sync endpoint', async () => {
  const user = userEvent.setup()
  await renderPricing()
  await user.click(screen.getByRole('button', { name: 'Sync Now' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith('/api/option/exchange_rate/sync')
  )
})

test('Sync Now is disabled while the form has unsaved changes', async () => {
  const user = userEvent.setup()
  const toggle = await renderPricing()
  expect(screen.getByRole('button', { name: 'Sync Now' })).toBeEnabled()
  await user.click(toggle)
  expect(screen.getByRole('button', { name: 'Sync Now' })).toBeDisabled()
  expect(
    screen.getByText('Save or discard your changes before syncing.')
  ).toBeInTheDocument()
})

test('a failed last sync shows its reason and source', async () => {
  await renderPricing(true, {
    lastSyncedAt: 1767225600,
    lastError: 'Synced exchange rate is outside the accepted range',
    lastSource: 'open.er-api.com',
  })
  expect(
    screen.getByText('Synced exchange rate is outside the accepted range')
  ).toBeInTheDocument()
  expect(screen.getByText('Source: open.er-api.com')).toBeInTheDocument()
  expect(screen.queryByText('Synced')).not.toBeInTheDocument()
})

test('a successful last sync shows the synced badge', async () => {
  await renderPricing(true, {
    lastSyncedAt: 1767225600,
    lastError: '',
    lastSource: 'open.er-api.com',
  })
  expect(screen.getByText('Synced')).toBeInTheDocument()
})
