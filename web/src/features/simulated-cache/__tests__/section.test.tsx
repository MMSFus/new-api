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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ThemeProvider } from '@/context/theme-provider'
import en from '@/i18n/locales/en.json'
import { api } from '@/lib/api'
import { logRangeSearchSchema } from '@/lib/log-range-search'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { SimulatedCache } from '..'

const chart = vi.hoisted(() =>
  vi.fn(
    (_props: {
      spec: { data: { id: string; values: { Metric: string }[] }[] }
      onClick?: () => void
    }) => null
  )
)
vi.mock('@visactor/react-vchart', () => ({ VChart: chart }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

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
  original_cache_creation_tokens: 2000,
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
  chart.mockClear()
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
  useSystemConfigStore.getState().setConfig(previousConfig)
})

async function renderSection(path = '/simulated-cache') {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  // Same route id as the real route so `getRouteApi` resolves.
  const root = createRootRoute()
  const layout = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => layout,
    path: '/simulated-cache/',
    validateSearch: logRangeSearchSchema,
    component: SimulatedCache,
  })
  const router = createRouter({
    routeTree: root.addChildren([layout.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await router.load()
  render(
    <I18nextProvider i18n={i18n}>
      <ThemeProvider>
        <QueryClientProvider client={client}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ThemeProvider>
    </I18nextProvider>
  )
  return router
}

/** Answers stat and list requests; `page` defaults to the single fixture log. */
function mockApi(
  options: {
    stat?: () => Promise<unknown>
    list?: () => Promise<unknown>
  } = {}
) {
  return vi.spyOn(api, 'get').mockImplementation(((url: string) => {
    if (url === '/api/log/simulated_cache/stat') {
      return options.stat
        ? options.stat()
        : Promise.resolve({ data: { success: true, data: stat } })
    }
    return options.list
      ? options.list()
      : Promise.resolve({
          data: { success: true, data: { total: 1, items: [logItem] } },
        })
  }) as typeof api.get)
}

function listParams(get: ReturnType<typeof mockApi>) {
  return get.mock.calls
    .filter((call) => call[0] === '/api/log/simulated_cache')
    .map((call) => (call[1] as { params: Record<string, unknown> }).params)
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
  expect(get).toHaveBeenCalledWith(
    '/api/log/simulated_cache',
    expect.objectContaining({
      params: {
        start_timestamp: NOW - 24 * 3600,
        end_timestamp: NOW,
        p: 1,
        page_size: 20,
      },
    })
  )
  expect(get).toHaveBeenCalledWith(
    '/api/log/simulated_cache/stat',
    expect.objectContaining({
      params: { start_timestamp: NOW - 24 * 3600, end_timestamp: NOW },
    })
  )
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

test('text edits do not query until Search commits them', async () => {
  const get = mockApi()
  await renderSection()
  await screen.findByText('alice')
  const initialCalls = get.mock.calls.length

  await userEvent
    .setup()
    .type(screen.getByRole('textbox', { name: 'Model Name' }), 'claude')
  expect(get).toHaveBeenCalledTimes(initialCalls)

  await userEvent.setup().click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(listParams(get)).toContainEqual(
      expect.objectContaining({ model_name: 'claude' })
    )
  )
})

test('changing quick range aborts obsolete list and statistic requests', async () => {
  const get = mockApi({
    stat: () => new Promise(() => {}),
    list: () => new Promise(() => {}),
  })
  await renderSection()
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  const previousSignals = get.mock.calls.map(
    (call) => (call[1] as { signal: AbortSignal }).signal
  )

  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Last hour' }))

  await waitFor(() => expect(get).toHaveBeenCalledTimes(4))
  expect(previousSignals.every((signal) => signal.aborted)).toBe(true)
})

test('the cache write chart includes original upstream tokens', async () => {
  mockApi()
  await renderSection()
  await screen.findByText('alice')
  const tokenChart = [...chart.mock.calls]
    .reverse()
    .find(([props]) => props.spec.data[0].values[0]?.Metric === 'Cache Read')

  expect(tokenChart?.[0].spec.data[0].values).toContainEqual({
    Metric: 'Cache Write',
    Side: 'Original',
    Value: 2000,
  })
})

test('a URL page size above the server limit leaves later cache logs reachable', async () => {
  const get = mockApi({
    list: async () => ({
      data: { success: true, data: { items: [logItem], total: 150 } },
    }),
  })
  await renderSection('/simulated-cache?pageSize=200')

  await screen.findByText('alice')
  await waitFor(() => expect(listParams(get)[0].page_size).toBe(100))
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeEnabled()
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
  await user.click(screen.getAllByRole('button', { name: 'Search' })[0])

  expect(
    await screen.findByText('Time range cannot exceed 7 days')
  ).toBeVisible()
  expect(get).not.toHaveBeenCalled()
})

test('URL filters seed the requests and a quick range rewrites the URL', async () => {
  const get = mockApi()
  const router = await renderSection(
    '/simulated-cache?channel=3&username=alice'
  )
  await screen.findByRole('button', { name: 'anthropic #3' })
  expect(listParams(get)[0]).toEqual({
    start_timestamp: NOW - 24 * 3600,
    end_timestamp: NOW,
    channel: 3,
    username: 'alice',
    p: 1,
    page_size: 20,
  })
  expect(screen.getByRole('button', { name: 'Last 24 hours' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )

  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Last hour' }))

  await waitFor(() =>
    expect(router.state.location.search).toEqual({
      channel: 3,
      username: 'alice',
      range: '1h',
    })
  )
  await waitFor(() =>
    expect(listParams(get).at(-1)).toMatchObject({
      start_timestamp: NOW - 3600,
      end_timestamp: NOW,
    })
  )
})

test('clicking a channel cell opens its requests in the drill sheet; applying it filters the page', async () => {
  const get = mockApi()
  const router = await renderSection()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'anthropic #3' }))

  const sheet = await screen.findByRole('dialog', {
    name: 'Channel: anthropic #3',
  })
  await waitFor(() =>
    expect(listParams(get)).toContainEqual({
      start_timestamp: NOW - 24 * 3600,
      end_timestamp: NOW,
      channel: 3,
      p: 1,
      page_size: 10,
    })
  )
  expect(await within(sheet).findByText('alice')).toBeVisible()

  await user.click(
    within(sheet).getByRole('button', { name: 'Apply as filter' })
  )

  await waitFor(() =>
    expect(router.state.location.search).toEqual({ channel: 3 })
  )
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('a KPI card opens the whole search with expandable per-request details', async () => {
  mockApi()
  await renderSection()
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', {
      name: 'View details of Simulated cost',
    })
  )

  const sheet = await screen.findByRole('dialog', { name: 'Simulated cost' })
  expect(
    within(sheet).queryByRole('button', { name: 'Apply as filter' })
  ).toBeNull()
  await user.click(
    await within(sheet).findByRole('button', { name: 'Show details' })
  )
  expect(within(sheet).getByText('5 / 7')).toBeVisible()
})

test('clicking a comparison chart bar opens the drill sheet', async () => {
  mockApi()
  await renderSection()
  await screen.findByText('alice')
  const cost = [...chart.mock.calls]
    .reverse()
    .find(([props]) => props.spec.data[0].values[0]?.Metric === 'Cost')

  cost?.[0].onClick?.()

  expect(
    await screen.findByRole('dialog', { name: 'Cost comparison' })
  ).toBeVisible()
})

test('while requests load, KPIs show skeletons and no empty message appears', async () => {
  mockApi({
    stat: () => new Promise(() => {}),
    list: () => new Promise(() => {}),
  })
  await renderSection()

  await waitFor(() =>
    expect(
      document.querySelectorAll('[data-slot="skeleton"]').length
    ).toBeGreaterThan(0)
  )
  expect(screen.queryByText('No records')).toBeNull()
  expect(screen.queryByRole('button', { name: /View details of/ })).toBeNull()
})

test('an empty search shows empty charts and an empty table', async () => {
  mockApi({
    stat: async () => ({
      data: { success: true, data: { ...stat, count: 0 } },
    }),
    list: async () => ({
      data: { success: true, data: { total: 0, items: [] } },
    }),
  })
  await renderSection()

  expect(await screen.findByText('No records')).toBeVisible()
  expect(screen.getAllByText('No data available')).toHaveLength(2)
})

test('a failed request shows the server reason and Retry requests again', async () => {
  let fail = true
  const get = mockApi({
    list: async () =>
      fail
        ? { data: { success: false, message: 'log database unavailable' } }
        : { data: { success: true, data: { total: 1, items: [logItem] } } },
  })
  await renderSection()

  expect(await screen.findByText('log database unavailable')).toBeVisible()
  fail = false
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))

  expect(await screen.findByText('alice')).toBeVisible()
  expect(listParams(get)).toHaveLength(2)
})
