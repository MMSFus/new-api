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
import dayjs from '@/lib/dayjs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { ChannelCost } from '..'
import type {
  ChannelCostAmount,
  ChannelCostLog,
  ChannelCostReport,
} from '../api'
import { channelCostSearchSchema } from '../lib'

const chart = vi.hoisted(() =>
  vi.fn(
    (_props: {
      spec: { type: string }
      onClick?: (event: { datum?: Record<string, unknown> }) => void
      onDimensionClick?: (event: {
        dimensionInfo?: { value?: unknown }[]
      }) => void
    }) => null
  )
)
vi.mock('@visactor/react-vchart', () => ({ VChart: chart }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const NOW = 1_800_000_000
const WEEK = 7 * 24 * 3600

function amount(patch: Partial<ChannelCostAmount> = {}): ChannelCostAmount {
  return {
    requests: 0,
    refunds: 0,
    uncosted_requests: 0,
    revenue: 0,
    costed_revenue: 0,
    cost: 0,
    profit: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    simulated_cache_delta: 0,
    ...patch,
  }
}

const anthropic = amount({
  requests: 3,
  uncosted_requests: 1,
  revenue: 1_000_000,
  costed_revenue: 800_000,
  cost: 400_000,
  profit: 400_000,
})
const report: ChannelCostReport = {
  total: anthropic,
  trend: [{ time: NOW - 3600, ...anthropic }],
  channels: [
    {
      channel_id: 3,
      channel_name: 'anthropic',
      upstream_ratio: 0.5,
      ...anthropic,
    },
  ],
  models: [{ model_name: 'claude-sonnet', ...anthropic }],
  model_count: 1,
  groups: [{ group: 'vip', ...anthropic }],
  users: [{ user_id: 1, username: 'alice', ...anthropic }],
  user_count: 1,
}
const emptyReport: ChannelCostReport = {
  total: amount(),
  trend: null,
  channels: null,
  models: null,
  model_count: 0,
  groups: null,
  users: null,
  user_count: 0,
}
const log: ChannelCostLog = {
  id: 11,
  created_at: NOW - 600,
  type: 2,
  user_id: 1,
  username: 'alice',
  model_name: 'claude-sonnet',
  channel_id: 3,
  channel_name: 'anthropic',
  group: 'vip',
  quota: 1_000_000,
  prompt_tokens: 1200,
  completion_tokens: 300,
  request_id: 'req-11',
  cost: {
    upstream_ratio: 0.5,
    group_ratio: 1,
    base_quota: 800_000,
    cost_quota: 400_000,
  },
  simulated_cache_delta: null,
}

type Responses = Partial<Record<string, () => Promise<unknown>>>

/** Answers each endpoint with a fixture; `overrides` replaces one by URL. */
function mockApi(overrides: Responses = {}) {
  const defaults: Responses = {
    '/api/channel_cost/report': async () => ({
      data: { success: true, data: report },
    }),
    '/api/channel_cost/logs': async () => ({
      data: { success: true, data: { items: [log], total: 1 } },
    }),
    '/api/channel_cost/backfill': async () => ({
      data: { success: true, data: { supported: true, task: null } },
    }),
  }
  return vi.spyOn(api, 'get').mockImplementation(((url: string) => {
    const respond = overrides[url] ?? defaults[url]
    if (!respond) throw new Error(`unexpected GET ${url}`)
    return respond()
  }) as typeof api.get)
}

function paramsOf(get: ReturnType<typeof mockApi>, url: string) {
  return get.mock.calls
    .filter((call) => call[0] === url)
    .map((call) => (call[1] as { params: Record<string, unknown> }).params)
}

const savedAuth = useAuthStore.getState().auth
const savedConfig = useSystemConfigStore.getState().config
let client: QueryClient

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  useAuthStore.setState({
    auth: {
      ...savedAuth,
      user: { id: 2, username: 'admin', role: ROLE.ADMIN },
    },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  chart.mockClear()
})

afterEach(() => {
  client.clear()
  useAuthStore.setState({ auth: savedAuth })
  useSystemConfigStore.getState().setConfig(savedConfig)
})

async function renderPage(path = '/channel-cost') {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  const root = createRootRoute()
  const layout = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => layout,
    path: '/channel-cost/',
    validateSearch: channelCostSearchSchema,
    component: ChannelCost,
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

/** Latest props of the chart whose spec has `type`. */
function chartProps(type: string) {
  const call = [...chart.mock.calls]
    .reverse()
    .find(([props]) => props.spec.type === type)
  if (!call) throw new Error(`no ${type} chart rendered`)
  return call[0]
}

test('URL filters seed the report and a quick range rewrites the URL', async () => {
  const get = mockApi()
  const router = await renderPage('/channel-cost?channel=3&model=claude')
  await screen.findByRole('button', { name: 'anthropic #3' })
  expect(paramsOf(get, '/api/channel_cost/report')[0]).toEqual({
    start_timestamp: NOW - WEEK,
    end_timestamp: NOW,
    channel: 3,
    model_name: 'claude',
  })
  expect(screen.getByRole('textbox', { name: 'Model Name' })).toHaveValue(
    'claude'
  )

  const quick = screen.getByRole('button', { name: 'Last 7 days' })
  expect(quick).toHaveAttribute('aria-pressed', 'true')
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Last 24 hours' }))

  await waitFor(() =>
    expect(router.state.location.search).toEqual({
      channel: 3,
      model: 'claude',
      range: '24h',
    })
  )
  await waitFor(() =>
    expect(paramsOf(get, '/api/channel_cost/report').at(-1)).toEqual({
      start_timestamp: NOW - 24 * 3600,
      end_timestamp: NOW,
      channel: 3,
      model_name: 'claude',
    })
  )
})

test('a URL page size above the server limit uses 100 rows and keeps later pages reachable', async () => {
  const get = mockApi({
    '/api/channel_cost/logs': async () => ({
      data: { success: true, data: { items: [log], total: 150 } },
    }),
  })
  const router = await renderPage('/channel-cost?pageSize=200')

  await waitFor(() =>
    expect(paramsOf(get, '/api/channel_cost/logs')[0].page_size).toBe(100)
  )
  await waitFor(() => expect(router.state.location.search.pageSize).toBe(100))
  await screen.findAllByRole('button', { name: 'View details' })
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeEnabled()
})

test('typing a filter keeps edits local until Search submits them', async () => {
  mockApi()
  const router = await renderPage('/channel-cost?page=3')
  await screen.findByRole('button', { name: 'anthropic #3' })

  await userEvent
    .setup()
    .type(screen.getByRole('textbox', { name: 'Username' }), 'bob')

  expect(router.state.location.search).toEqual({ page: 3 })
  await userEvent
    .setup()
    .click(screen.getAllByRole('button', { name: 'Search' })[0])
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ username: 'bob' })
  )
  expect(router.state.location.search.page).toBeUndefined()
})

test('the breakdown dimension is kept in the URL and restored on load', async () => {
  mockApi()
  const router = await renderPage('/channel-cost?dimension=user')
  const tabs = await screen.findByRole('tablist', {
    name: 'Breakdown dimension',
  })
  expect(within(tabs).getByRole('tab', { name: 'User' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(await screen.findByRole('button', { name: 'alice #1' })).toBeVisible()

  await userEvent
    .setup()
    .click(within(tabs).getByRole('tab', { name: 'Model' }))

  await waitFor(() =>
    expect(router.state.location.search).toEqual({ dimension: 'model' })
  )
  expect(chartProps('pie').spec).toMatchObject({
    data: [{ values: [{ key: 'claude-sonnet', value: 1_000_000 }] }],
  })
})

test('clicking a breakdown name opens the drill sheet with that channel’s logs', async () => {
  const get = mockApi()
  await renderPage()
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'anthropic #3' }))

  const sheet = await screen.findByRole('dialog', {
    name: 'Channel: anthropic #3',
  })
  await waitFor(() =>
    expect(paramsOf(get, '/api/channel_cost/logs')).toContainEqual({
      start_timestamp: NOW - WEEK,
      end_timestamp: NOW,
      channel: 3,
      p: 1,
      page_size: 10,
    })
  )
  expect(
    within(sheet).getByText('Upstream cost ratio').nextSibling
  ).toHaveTextContent('0.5')
  expect(
    within(sheet).getByRole('button', { name: 'Apply as filter' })
  ).toBeVisible()
})

test('opening a log in the drill sheet shows its cost calculation', async () => {
  mockApi()
  await renderPage()
  await screen.findByRole('button', { name: 'anthropic #3' })
  const user = userEvent.setup()
  // The page table and the sheet table both list the log; use the page one.
  await user.click(
    (await screen.findAllByRole('button', { name: 'View details' }))[0]
  )

  const sheet = await screen.findByRole('dialog', { name: 'Cost details' })
  expect(within(sheet).getByText('Cost calculation')).toBeVisible()
  // Upstream cost = official price × channel upstream ratio.
  expect(
    within(sheet).getByText(/^= Official price \$1\.6\d* × 0\.5$/)
  ).toBeVisible()
  expect(within(sheet).getByText('req-11')).toBeVisible()
})

test('clicking a revenue share slice drills into that row', async () => {
  const get = mockApi()
  await renderPage('/channel-cost?dimension=group')
  await screen.findByRole('button', { name: 'vip' })

  chartProps('pie').onClick?.({ datum: { key: 'vip' } })

  expect(
    await screen.findByRole('dialog', { name: 'Group: vip' })
  ).toBeVisible()
  await waitFor(() =>
    expect(paramsOf(get, '/api/channel_cost/logs')).toContainEqual(
      expect.objectContaining({ group: 'vip', page_size: 10 })
    )
  )
})

test('user breakdown drills and applies filters by stable user ID', async () => {
  const get = mockApi()
  await renderPage('/channel-cost?dimension=user')
  await screen.findByRole('button', { name: 'alice #1' })
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'alice #1' }))

  const sheet = await screen.findByRole('dialog', { name: 'User: alice #1' })
  await userEvent
    .setup()
    .click(within(sheet).getByRole('button', { name: 'Apply as filter' }))

  await waitFor(() =>
    expect(paramsOf(get, '/api/channel_cost/logs')).toContainEqual(
      expect.objectContaining({ user_id: 1 })
    )
  )
  expect(screen.getByRole('textbox', { name: 'User ID' })).toHaveValue('1')
})

test('clicking a trend point drills into its day within the range', async () => {
  const get = mockApi()
  await renderPage()
  await screen.findByRole('button', { name: 'anthropic #3' })
  // The default week-long range plots days, keyed by the viewer-local date.
  const dayKey = dayjs.unix(NOW - 3600).format('YYYY-MM-DD')

  chartProps('line').onDimensionClick?.({
    dimensionInfo: [{ value: dayKey }],
  })

  expect(await screen.findByRole('dialog', { name: dayKey })).toBeVisible()
  await waitFor(() =>
    expect(paramsOf(get, '/api/channel_cost/logs')).toContainEqual({
      start_timestamp: Math.max(NOW - WEEK, NOW - 3600),
      end_timestamp: Math.min(NOW, NOW - 3600 + 3599),
      p: 1,
      page_size: 10,
    })
  )
})

test('a KPI card opens the totals of the whole search', async () => {
  mockApi()
  await renderPage()
  await userEvent.setup().click(
    await screen.findByRole('button', {
      name: 'View details of Gross profit',
    })
  )

  const sheet = await screen.findByRole('dialog', { name: 'Gross profit' })
  expect(within(sheet).getByText('Margin').nextSibling).toHaveTextContent('50%')
  expect(
    within(sheet).queryByRole('button', { name: 'Apply as filter' })
  ).toBeNull()
})

test('while the report loads, KPI values are skeletons and no empty message shows', async () => {
  mockApi({
    '/api/channel_cost/report': () => new Promise(() => {}),
    '/api/channel_cost/logs': () => new Promise(() => {}),
  })
  await renderPage()

  await waitFor(() =>
    expect(
      document.querySelectorAll('[data-slot="skeleton"]').length
    ).toBeGreaterThan(0)
  )
  expect(screen.queryByText('No data available')).toBeNull()
  expect(
    screen.queryByRole('button', { name: 'View details of Revenue' })
  ).toBeNull()
})

test('an empty report shows empty panels and an empty log table', async () => {
  mockApi({
    '/api/channel_cost/report': async () => ({
      data: { success: true, data: emptyReport },
    }),
    '/api/channel_cost/logs': async () => ({
      data: { success: true, data: { items: [], total: 0 } },
    }),
  })
  await renderPage()

  await waitFor(() =>
    expect(
      screen.getAllByText('No data available').length
    ).toBeGreaterThanOrEqual(3)
  )
  expect(await screen.findByText('No records')).toBeVisible()
})

test('a failed report shows the server reason and Retry requests it again', async () => {
  let fail = true
  const get = mockApi({
    '/api/channel_cost/report': async () =>
      fail
        ? { data: { success: false, message: 'log database unavailable' } }
        : { data: { success: true, data: report } },
  })
  await renderPage()

  expect(await screen.findByText('log database unavailable')).toBeVisible()
  fail = false
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))

  expect(
    await screen.findByRole('button', { name: 'anthropic #3' })
  ).toBeVisible()
  expect(paramsOf(get, '/api/channel_cost/report')).toHaveLength(2)
})

test('a range longer than 31 days is rejected before any request', async () => {
  const get = mockApi()
  await renderPage(`/channel-cost?start=${NOW - 40 * 24 * 3600}&end=${NOW}`)

  expect(
    await screen.findByText('Time range cannot exceed 31 days')
  ).toBeVisible()
  expect(paramsOf(get, '/api/channel_cost/report')).toHaveLength(0)
})

test('the root user reaches backfill from the header entry', async () => {
  useAuthStore.setState({
    auth: {
      ...savedAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
  mockApi()
  await renderPage()
  await screen.findByRole('button', { name: 'anthropic #3' })
  expect(
    screen.queryByRole('button', { name: 'Backfill historical costs' })
  ).toBeNull()

  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: /Historical cost backfill/ }))

  const dialog = await screen.findByRole('dialog', {
    name: 'Historical cost backfill',
  })
  expect(
    within(dialog).getByText('The current range has 1 uncosted requests')
  ).toBeVisible()
  await waitFor(() =>
    expect(
      within(dialog).getByRole('button', { name: 'Backfill historical costs' })
    ).toBeEnabled()
  )
})

test('other administrators see no backfill entry', async () => {
  const get = mockApi()
  await renderPage()
  await screen.findByRole('button', { name: 'anthropic #3' })

  expect(
    screen.queryByRole('button', { name: /Historical cost backfill/ })
  ).toBeNull()
  expect(
    get.mock.calls.some((call) => call[0] === '/api/channel_cost/backfill')
  ).toBe(false)
})
