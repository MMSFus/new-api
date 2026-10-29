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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { SimulatedCache } from '..'

const NOW = 1_800_000_000

// Mocked per .agent-notes/10-audit-contract.md: items are normal Log objects
// whose `other` carries admin_info.simulated_cache.
const logItem = {
  id: 7,
  user_id: 1,
  created_at: NOW - 60,
  type: 2,
  content: '',
  username: 'alice',
  token_name: 'token',
  model_name: 'claude-sonnet',
  quota: 2600,
  prompt_tokens: 12000,
  completion_tokens: 300,
  use_time: 1,
  is_stream: false,
  channel: 3,
  channel_name: 'anthropic',
  token_id: 1,
  group: 'default',
  ip: '',
  request_id: 'req-7',
  upstream_request_id: '',
  other: JSON.stringify({
    admin_info: {
      simulated_cache: {
        version: 1,
        hit_rate: 90,
        hit_rate_min: 80,
        hit_rate_max: 90,
        target_rate: 85.37,
        client_format: 'claude',
        upstream_schema: 'claude',
        matched_segments: 5,
        total_segments: 7,
        original: { prompt_tokens: 12000, cache_read_tokens: 0, quota: 4200 },
        simulated: {
          prompt_tokens: 12000,
          cache_read_tokens: 8000,
          cache_creation_tokens: 2000,
          cache_creation_5m_tokens: 2000,
          quota: 2600,
        },
        quota_delta: -1600,
      },
    },
  }),
}

const stat = {
  count: 1,
  original_quota: 4200,
  simulated_quota: 2600,
  quota_delta: -1600,
  original_cache_read_tokens: 0,
  simulated_cache_read_tokens: 8000,
  simulated_cache_creation_tokens: 2000,
}

const previousConfig = useSystemConfigStore.getState().config
let client: QueryClient

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
  useSystemConfigStore.getState().setConfig(previousConfig)
})

async function renderSection() {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <SimulatedCache />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('loads the last 24h by default and expands a row into the comparison', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url: string) => ({
    data: {
      success: true,
      data:
        url === '/api/log/simulated_cache/stat'
          ? stat
          : { page: 1, page_size: 20, total: 1, items: [logItem] },
    },
  }))
  await renderSection()

  expect(await screen.findByText('alice')).toBeVisible()
  expect(get).toHaveBeenCalledWith('/api/log/simulated_cache', {
    params: {
      start_timestamp: NOW - 24 * 3600,
      end_timestamp: NOW,
      p: 1,
      page_size: 20,
    },
  })
  expect(get).toHaveBeenCalledWith('/api/log/simulated_cache/stat', {
    params: { start_timestamp: NOW - 24 * 3600, end_timestamp: NOW },
  })
  expect(screen.getByText('$0.0052')).toBeVisible()
  expect(screen.getByText('$0.0084')).toBeVisible()

  const toggle = screen.getByRole('button', { name: 'Show details' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await userEvent.setup().click(toggle)

  const hide = screen.getByRole('button', { name: 'Hide details' })
  expect(hide).toHaveAttribute('aria-expanded', 'true')
  const cacheRow = screen
    .getAllByRole('cell', { name: 'Cache Read' })
    .map((cell) => cell.closest('tr') as HTMLElement)[0]
  expect(
    within(cacheRow)
      .getAllByRole('cell')
      .map((cell) => cell.textContent)
  ).toEqual(['Cache Read', '0', '8,000'])
  expect(screen.getByText('5 / 7')).toBeVisible()
  expect(
    screen.getByText('Configured hit rate').nextElementSibling
  ).toHaveTextContent('80%–90%')
  expect(
    screen.getByText('Target hit rate').nextElementSibling
  ).toHaveTextContent('85.37%')
})

test('the expanded row spans the table and lays comparison and meta out side by side when wide', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => ({
    data: {
      success: true,
      data:
        url === '/api/log/simulated_cache/stat'
          ? stat
          : { page: 1, page_size: 20, total: 1, items: [logItem] },
    },
  }))
  await renderSection()
  await screen.findByText('alice')
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Show details' }))

  const comparison = document.querySelector(
    '[data-slot="simulated-cache-comparison"]'
  ) as HTMLElement
  const meta = within(comparison)
    .getByText('Cost difference')
    .closest('[data-slot="simulated-cache-meta"]') as HTMLElement
  const expansionCell = comparison.closest('td') as HTMLElement
  // Full width: the expansion cell spans every column with no width cap.
  expect(expansionCell).toHaveAttribute(
    'colspan',
    String(
      within(screen.getAllByRole('row')[0]).getAllByRole('columnheader').length
    )
  )
  expect(comparison.closest('.max-w-xl')).toBeNull()
  // Stacked by default, a row when the container is wide; meta has a fixed column.
  expect(comparison).toHaveClass('flex-col', '@3xl/simulated-cache:flex-row')
  expect(meta.parentElement).toBe(comparison)
  expect(meta).toHaveClass('@3xl/simulated-cache:w-80')
  expect(
    within(comparison).getByRole('cell', { name: 'Cache Read' })
  ).not.toBeNull()
  expect(within(meta).queryByRole('table')).toBeNull()
})

test('a range longer than 7 days is rejected before any request', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  await renderSection()
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  get.mockClear()

  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /~/ }))
  await user.click(await screen.findByRole('button', { name: '30 Days' }))

  expect(
    await screen.findByText('Time range cannot exceed 7 days')
  ).toBeVisible()
  expect(get).not.toHaveBeenCalled()
})
