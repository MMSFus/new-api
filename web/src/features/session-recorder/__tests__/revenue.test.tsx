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
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { RevenueSummary } from '../api'
import {
  fail,
  FULL_GRANTS,
  mockApi,
  NOW,
  ok,
  renderPage,
  rules,
  setUser,
  summary,
} from './harness'

const chart = vi.hoisted(() =>
  vi.fn(
    (_props: {
      onClick?: (event: { datum?: Record<string, unknown> }) => void
    }) => null
  )
)
vi.mock('@visactor/react-vchart', () => ({ VChart: chart }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const RULES = 'GET /api/session_recorder/revenue/rules'
const SUMMARY = 'GET /api/session_recorder/revenue/summary'
const savedAuth = useAuthStore.getState().auth
const savedConfig = useSystemConfigStore.getState().config

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  setUser(ROLE.ADMIN, FULL_GRANTS)
  chart.mockClear()
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  useSystemConfigStore.getState().setConfig(savedConfig)
  vi.restoreAllMocks()
})

function card(title: string) {
  return screen.getByRole('group', { name: title })
}

describe('KPIs and units', () => {
  test('compressed data is shown in TB of 10^12 bytes and labelled as compressed', async () => {
    mockApi()
    await renderPage()

    const archive = await screen.findByRole('group', {
      name: 'Compressed data',
    })
    expect(within(archive).getByText('2.5 TB')).toBeVisible()
    expect(
      within(archive).getByText(
        'Compressed size uploaded to R2; 1 TB = 10^12 bytes'
      )
    ).toBeInTheDocument()
    expect(within(archive).getByText('9 TB')).toBeInTheDocument()
  })

  test('without a currency label revenue uses the site currency symbol unconverted', async () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CUSTOM',
        customCurrencySymbol: 'MC',
        customCurrencyExchangeRate: 6.65,
      },
    })
    mockApi()
    await renderPage()

    const revenue = await screen.findByRole('group', { name: 'Revenue' })
    await waitFor(() =>
      expect(within(revenue).getByText('MC 2,700')).toBeVisible()
    )
  })

  test('a non-empty currency label replaces the site symbol as the unit', async () => {
    mockApi({ [RULES]: ok({ ...rules, currency_label: 'CNY' }) })
    await renderPage()

    const revenue = await screen.findByRole('group', { name: 'Revenue' })
    await waitFor(() =>
      expect(within(revenue).getByText('2,700 CNY')).toBeVisible()
    )
    expect(within(revenue).getByText('CNY')).toBeInTheDocument()
  })

  test('record and unarchived counts come from the totals', async () => {
    mockApi()
    await renderPage()

    await screen.findByRole('group', { name: 'Records' })
    expect(within(card('Records')).getByText('30')).toBeVisible()
    const unarchived = card('Unarchived records')
    expect(within(unarchived).getByText('4')).toBeVisible()
    expect(within(unarchived).getByText('3 MB')).toBeInTheDocument()
  })

  test('a tiny compressed size keeps significant digits instead of reading 0', async () => {
    mockApi({
      [SUMMARY]: ok({
        ...summary,
        totals: { ...summary.totals, archive_bytes: 1_234_000 },
      }),
    })
    await renderPage()

    const archive = await screen.findByRole('group', {
      name: 'Compressed data',
    })
    expect(within(archive).getByText('0.00000123 TB')).toBeVisible()
  })
})

describe('breakdown and drill-down', () => {
  test('the rule table lists rules in order with the unmatched records last', async () => {
    mockApi()
    await renderPage()

    const section = await screen.findByRole('region', { name: 'Breakdown' })
    await within(section).findByRole('button', { name: 'Claude premium' })
    const names = within(section)
      .getAllByRole('row')
      .slice(1)
      .map((row) => within(row).getAllByRole('cell')[0].textContent)
    expect(names).toEqual(['Claude premium', 'GPT', 'Unmatched'])
  })

  test('a rule opens the drill sheet with its models; a model there links to its sessions', async () => {
    mockApi()
    const { router } = await renderPage()
    const user = userEvent.setup()

    await user.click(
      await screen.findByRole('button', { name: 'Claude premium' })
    )
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByText('$1,200')).toBeVisible()
    expect(within(sheet).queryByRole('button', { name: 'gpt-5' })).toBeNull()

    await user.click(
      within(sheet).getByRole('button', { name: 'claude-opus-4' })
    )
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: 'View sessions',
      })
    )

    await waitFor(() =>
      expect(router.state.location.search).toEqual({
        tab: 'content',
        model: 'claude-opus-4',
      })
    )
  })

  test('the model filter icon narrows the page to that model', async () => {
    mockApi()
    const { router } = await renderPage('/session-recorder?breakdown=model')

    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'Filter by gpt-5' }))

    await waitFor(() =>
      expect(router.state.location.search).toEqual({
        breakdown: 'model',
        model: 'gpt-5',
      })
    )
  })

  test('clicking a trend point narrows the range to that day', async () => {
    mockApi()
    const { router } = await renderPage('/session-recorder?range=30d')
    await screen.findByText('2.5 TB')
    await waitFor(() => expect(chart).toHaveBeenCalled())
    const props = chart.mock.calls.at(-1)?.[0]
    // Day buckets are UTC-aligned and labelled with their UTC date.
    const day = Math.floor((NOW - 86_400) / 86_400) * 86_400
    const label = new Date(day * 1000).toISOString().slice(0, 10)

    act(() => {
      props?.onClick?.({ datum: { Time: label } })
    })

    await waitFor(() =>
      expect(router.state.location.search).toEqual({
        start: day,
        end: day + 86_399,
      })
    )
  })
})

describe('loading, empty and error states', () => {
  test('KPIs show skeletons while the summary loads', async () => {
    mockApi({ [SUMMARY]: () => new Promise(() => {}) })
    await renderPage()

    const archive = await screen.findByRole('group', {
      name: 'Compressed data',
    })
    expect(archive.querySelector('[data-slot="skeleton"]')).toBeInTheDocument()
    expect(within(archive).queryByText(/TB$/)).toBeNull()
  })

  test('an empty summary shows zero KPIs and empty breakdowns', async () => {
    const empty: RevenueSummary = {
      totals: {
        records: 0,
        raw_bytes: 0,
        archive_bytes: 0,
        revenue: 0,
        unarchived_records: 0,
        unarchived_raw_bytes: 0,
      },
      by_rule: null,
      unmatched: { records: 0, archive_bytes: 0 },
      by_model: null,
      trend: null,
    }
    mockApi({ [SUMMARY]: ok(empty) })
    await renderPage()

    const archive = await screen.findByRole('group', {
      name: 'Compressed data',
    })
    expect(within(archive).getByText('0 TB')).toBeVisible()
    expect(screen.getAllByText('No data available').length).toBeGreaterThan(0)
  })

  test('records waiting for upload explain why revenue is empty', async () => {
    mockApi({
      [SUMMARY]: ok({
        ...summary,
        totals: { ...summary.totals, records: 0, archive_bytes: 0, revenue: 0 },
      }),
    })
    await renderPage()

    expect(
      await screen.findByText(
        'No records in this range are archived yet. Revenue appears after their batches are uploaded.'
      )
    ).toBeVisible()
  })

  test('a failed summary shows the server message and retries on demand', async () => {
    const api = mockApi({ [SUMMARY]: fail('Time range exceeds 92 days') })
    await renderPage()

    expect(await screen.findByText('Time range exceeds 92 days')).toBeVisible()
    const before = api.params('/api/session_recorder/revenue/summary').length

    await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))

    await waitFor(() =>
      expect(
        api.params('/api/session_recorder/revenue/summary').length
      ).toBeGreaterThan(before)
    )
  })
})
