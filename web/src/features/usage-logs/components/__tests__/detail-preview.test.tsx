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
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterAll, afterEach, beforeEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { useCommonLogsColumns } from '../columns/common-logs-columns'

vi.mock('@lobehub/icons', () => ({}))
vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})
afterAll(() => vi.unstubAllGlobals())

function makeLog(other: LogOtherData, type = 2): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'wan2.5-i2v-preview',
    quota: 5000,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
  }
}

function DetailPreview(props: {
  other: LogOtherData
  isAdmin: boolean
  type?: number
  columnId?: string
  log?: Partial<UsageLog>
}) {
  const columnId = props.columnId ?? 'content'
  const table = useReactTable({
    data: [{ ...makeLog(props.other, props.type), ...props.log }],
    columns: useCommonLogsColumns(props.isAdmin, false),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === columnId)
  if (!cell) throw new Error(`The log must have a ${columnId} column`)
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}
const plugin = {
  key: 'incho',
  name: 'Incho',
  version: '1.0.1',
  author: { name: 'Plugin maintainer' },
}
const previousConfig = useSystemConfigStore.getState().config
let client: QueryClient
const i18n = createInstance()
beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['status'], {}, { updatedAt: Date.now() + 60_000 })
  client.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: Date.now() + 60_000 }
  )
})
afterEach(() => {
  client.clear()
  useSystemConfigStore.getState().setConfig(previousConfig)
})
function renderPreview(other: LogOtherData, isAdmin = true, type?: number) {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <DetailPreview other={other} isAdmin={isAdmin} type={type} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  return screen.getByRole('button', { name: /./ })
}

test('keeps log details open when the parent refreshes with unchanged data', async () => {
  const other = { model_price: 0.25 }
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    </I18nextProvider>
  )
  const { rerender } = render(<DetailPreview other={other} isAdmin />, {
    wrapper,
  })
  fireEvent.click(screen.getByRole('button', { name: 'Per-call · $0.25' }))
  expect(await screen.findByRole('dialog')).toBeVisible()
  rerender(<DetailPreview other={other} isAdmin />)
  expect(screen.getByRole('dialog')).toBeVisible()
})

test.each([
  {
    name: 'fixed expression zero price',
    other: {
      billing_mode: 'tiered_expr',
      billing_unit: 'request' as const,
      fixed_price: 0,
      matched_tier: 'free',
      expr_b64: btoa('tier("free", fixed(0))'),
    },
    expected: 'free · Per-call $0/request',
  },
  {
    name: 'fixed expression trace outside the display grammar',
    other: {
      billing_mode: 'tiered_expr',
      billing_unit: 'request' as const,
      fixed_price: 0.01,
      matched_tier: 'priority',
      expr_b64: btoa(
        'param("fast") == true ? tier("priority", fixed(0.01)) : tier("tokens", p * 2)'
      ),
    },
    expected: 'priority · Per-call $0.01/request',
  },
  {
    name: 'parsed token expression',
    other: {
      billing_mode: 'tiered_expr',
      matched_tier: 'standard',
      expr_b64: btoa('tier("standard", p * 2 + c * 10)'),
    },
    expected: 'standard · $2 / $10/M',
  },
  {
    name: 'per-call',
    other: { model_price: 0.25 },
    expected: 'Per-call · $0.25',
  },
  {
    name: 'standard',
    other: { model_ratio: 1, completion_ratio: 2 },
    expected: 'Standard · $2 / $4/M',
  },
  {
    name: 'zero price fallback',
    other: { model_price: 0, group_ratio: 1 },
    expected: 'Group Ratio 1x',
  },
  { name: 'missing price fallback', other: {}, expected: '—' },
])('$name stays visible without a plugin counter', ({ other, expected }) => {
  const preview = renderPreview({
    ...other,
    admin_info: { task_plugin: plugin },
  })
  expect(preview.textContent).toBe(expected)
})

test.each([
  {
    tier: 'standard',
    billingUnit: 'token' as const,
    fixedPrice: undefined,
    expected: 'Dynamic Pricing · standard',
  },
  {
    tier: 'long_context',
    billingUnit: 'token' as const,
    fixedPrice: undefined,
    expected: 'Dynamic Pricing · long_context',
  },
  {
    tier: 'free_small_output',
    billingUnit: 'request' as const,
    fixedPrice: 0,
    expected: 'free_small_output · Per-call $0/request',
  },
])(
  'nested expression preserves the recorded $tier settlement in the list',
  ({ tier, billingUnit, fixedPrice, expected }) => {
    const expression = `len <= 272000
      ? (c > 5
        ? tier("standard", p * 2 + c * 10 + cr * 0.1 + cc * 2.5)
        : tier("free_small_output", fixed(0)))
      : (c > 5
        ? tier("long_context", p * 5 + c * 22.5 + cr * 0.5)
        : tier("free_small_output", fixed(0)))`
    const preview = renderPreview({
      billing_mode: 'tiered_expr',
      billing_unit: billingUnit,
      fixed_price: fixedPrice,
      matched_tier: tier,
      expr_b64: btoa(expression),
    })
    expect(preview.textContent).toBe(expected)
  }
)

test.each([
  { name: 'missing expression', expr: undefined },
  { name: 'invalid encoded expression', expr: '%' },
  {
    name: 'tier absent from the parsed expression',
    expr: btoa('tier("different", p * 99 + c * 99)'),
  },
])(
  'token pricing with $name preserves the saved tier without guessing prices',
  ({ expr }) => {
    const preview = renderPreview({
      billing_mode: 'tiered_expr',
      matched_tier: 'recorded',
      expr_b64: expr,
    })
    expect(preview.textContent).toBe('Dynamic Pricing · recorded')
  }
)

test.each([false, true])(
  'dynamic pricing without a saved tier uses a localized neutral fallback for task=%s',
  async (isTask) => {
    i18n.addResourceBundle('zh', 'translation', zh.translation)
    const preview = renderPreview({
      is_task: isTask,
      billing_mode: 'tiered_expr',
      expr_b64: btoa(
        isTask
          ? 'tier("base", u("clips") * 0.25)'
          : 'tier("base", p * 2 + c * 10)'
      ),
    })
    expect(preview.textContent).toBe(
      'Dynamic Pricing · Special billing expression'
    )
    await act(() => i18n.changeLanguage('zh'))
    expect(
      screen.getByRole('button', { name: '动态计费 · 特殊计费表达式' })
    ).toBeVisible()
  }
)

test('top-up content_parts render in the viewer language', async () => {
  i18n.addResourceBundle('zh', 'translation', {
    'Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}':
      '在线充值成功，充值额度：{{quota}}，支付金额：{{amount}}',
  })
  await i18n.changeLanguage('zh')
  const preview = renderPreview(
    {
      content_parts: [
        {
          key: 'Online top-up succeeded, amount added: {{quota}}, payment amount: {{amount}}',
          params: { quota: '$10.000000', amount: '10.00' },
        },
      ],
    },
    false,
    1
  )
  expect(preview.textContent).toBe(
    '在线充值成功，充值额度：$10.000000，支付金额：10.00'
  )
})

test('quota saturation remains first and only billing adds to the counter', () => {
  const preview = renderPreview({
    model_price: 0.25,
    admin_info: {
      task_plugin: plugin,
      quota_saturation: {
        op: 'round',
        kind: 'overflow',
        original: 3e9,
        clamped: 2147483647,
      },
    },
  })
  expect(preview.textContent).toBe('Quota clamped+1')
})

test.each([true, false])(
  'plugin information in the opened dialog respects admin=%s',
  async (isAdmin) => {
    const preview = renderPreview(
      { model_price: 0.25, admin_info: { task_plugin: plugin } },
      isAdmin
    )
    expect(preview.textContent).toBe('Per-call · $0.25')
    fireEvent.click(preview)
    const dialog = within(await screen.findByRole('dialog'))
    if (isAdmin) {
      expect(dialog.getByText('Incho')).toBeVisible()
      expect(dialog.getByText('1.0.1')).toBeVisible()
      expect(dialog.getByText('Plugin maintainer')).toBeVisible()
    } else {
      expect(dialog.queryByText('Incho')).not.toBeInTheDocument()
      expect(dialog.queryByText('Plugin maintainer')).not.toBeInTheDocument()
    }
  }
)

test.each([
  {
    expression: 'tier("music", u("clips") * 0.25)',
    tier: 'music',
    expected: 'music · clips $0.25/unit',
  },
  {
    expression:
      'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)',
    tier: 'pro',
    expected: 'pro · seconds $0.8/second',
  },
  {
    expression: 'tier("tokens", u("tokens") * 9.8 / 1000000)',
    tier: 'tokens',
    expected: 'tokens · tokens $9.8/1M token',
  },
  {
    expression: 'tier("free", u("clips") * 0)',
    tier: 'free',
    expected: 'free · clips $0/unit',
  },
  {
    expression: 'tier("mixed", 0.1 + u("clips") * 0.25 + u("units") * 0.14)',
    tier: 'mixed',
    expected:
      'mixed · clips $0.25/unit · units $0.14/credit · Additional charge $0.1/request',
  },
])(
  'task expression $tier shows its recorded unit price',
  ({ expression, tier, expected }) => {
    client.setQueryData(['pricing'], {
      data: [
        {
          model_name: 'wan2.5-i2v-preview',
          billing_expr: 'tier("current", u("clips") * 99)',
          billing_usage_schema: {
            clips: { type: 'number', unit: 'count' },
            seconds: { type: 'number', unit: 'second' },
            tokens: { type: 'number', unit: 'token' },
            units: { type: 'number', unit: 'credit' },
            mode: { enum: ['pro', 'std'] },
          },
        },
      ],
      vendors: [],
    })
    const preview = renderPreview({
      is_task: true,
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(expression).toString('base64'),
      matched_tier: tier,
      model_price: 0,
      admin_info: { task_plugin: plugin },
    })
    expect(preview.textContent).toBe(expected)
  }
)

test('task log prices use localized unit labels from pricing metadata', async () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: {
          images: {
            type: 'number',
            unit: 'count',
            unitLabel: { en: 'image', zh: '张' },
          },
        },
      },
    ],
    vendors: [],
  })
  const preview = renderPreview({
    is_task: true,
    billing_mode: 'tiered_expr',
    expr_b64: btoa('tier("images", u("images") * 0.25)'),
    matched_tier: 'images',
  })
  expect(preview).toHaveTextContent('images · images $0.25/image')
  await act(() => i18n.changeLanguage('zh-CN'))
  expect(
    screen.getByRole('button', { name: /images · images/ })
  ).toHaveTextContent('images · images $0.25/张')
})

test('task log prices select the executing provider’s schema', () => {
  client.setQueryData(['pricing'], {
    data: [
      {
        model_name: 'wan2.5-i2v-preview',
        billing_usage_schema: { seconds: { type: 'number', unit: 'second' } },
        billing_plugin_variants: [
          {
            plugin_key: 'beta',
            plugin_name: 'Beta',
            billing_expr: 'tier("images", u("images") * 0.25)',
            billing_usage_schema: {
              images: {
                type: 'number',
                unit: 'count',
                unitLabel: { en: 'image' },
              },
            },
          },
        ],
      },
    ],
    vendors: [],
  })
  const preview = renderPreview({
    is_task: true,
    billing_mode: 'tiered_expr',
    expr_b64: btoa('tier("images", u("images") * 0.25)'),
    matched_tier: 'images',
    admin_info: {
      task_plugin: { key: 'beta', name: 'Beta', version: '1.0.0' },
    },
  })
  expect(preview).toHaveTextContent('images · images $0.25/image')
})

test.each(['missing schema', 'unsupported expression', 'unknown tier'])(
  'task pricing with %s preserves the saved tier without guessing prices',
  (scenario) => {
    if (scenario !== 'missing schema') {
      client.setQueryData(['pricing'], {
        data: [
          {
            model_name: 'wan2.5-i2v-preview',
            billing_usage_schema: { clips: { type: 'number', unit: 'count' } },
          },
        ],
        vendors: [],
      })
    }
    const expression =
      scenario === 'unsupported expression'
        ? 'tier("music", max(u("clips"), 1) * 0.25)'
        : 'tier("music", u("clips") * 0.25)'
    const preview = renderPreview({
      is_task: true,
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(expression).toString('base64'),
      matched_tier: scenario === 'unknown tier' ? 'old' : 'music',
    })
    expect(preview.textContent).toBe(
      `Dynamic Pricing · ${scenario === 'unknown tier' ? 'old' : 'music'}`
    )
  }
)

// Contract fixture from .agent-notes/10-audit-contract.md (quota units:
// 500000 per USD with the default currency config).
const simulatedCacheOther: LogOtherData = {
  cache_tokens: 8000,
  admin_info: {
    simulated_cache: {
      version: 1,
      hit_rate: 80,
      client_format: 'claude',
      upstream_schema: 'claude',
      matched_segments: 5,
      total_segments: 7,
      original: {
        prompt_tokens: 12000,
        completion_tokens: 300,
        cache_read_tokens: 0,
        cache_creation_tokens: 0,
        cache_creation_5m_tokens: 0,
        cache_creation_1h_tokens: 0,
        quota: 4200,
      },
      simulated: {
        prompt_tokens: 12000,
        completion_tokens: 300,
        cache_read_tokens: 8000,
        cache_creation_tokens: 2000,
        cache_creation_5m_tokens: 2000,
        cache_creation_1h_tokens: 0,
        quota: 2600,
      },
      quota_delta: -1600,
    },
  },
}

function comparisonRow(dialog: ReturnType<typeof within>, label: string) {
  const cells = within(
    dialog.getByRole('cell', { name: label }).closest('tr') as HTMLElement
  ).getAllByRole('cell')
  return cells.slice(1).map((cell) => cell.textContent)
}

test('admin log details compare original and simulated cache usage', async () => {
  const preview = renderPreview(simulatedCacheOther)
  fireEvent.click(preview)
  const dialog = within(await screen.findByRole('dialog'))

  expect(dialog.getByText('Simulated cache')).toBeVisible()
  expect(comparisonRow(dialog, 'Input Tokens')).toEqual(['12,000', '12,000'])
  expect(comparisonRow(dialog, 'Cache Read')).toEqual(['0', '8,000'])
  expect(comparisonRow(dialog, 'Cache Write (5m)')).toEqual(['0', '2,000'])
  expect(comparisonRow(dialog, 'Cache Write (1h)')).toEqual(['0', '0'])
  expect(comparisonRow(dialog, 'Cache hit ratio')).toEqual(['0%', '66.7%'])
  expect(comparisonRow(dialog, 'Cost')).toEqual(['$0.0084', '$0.0052'])
  expect(
    dialog.getByText('Cost difference').nextElementSibling
  ).toHaveTextContent('-$0.0032')
  expect(
    dialog.getByText('Configured hit rate').nextElementSibling
  ).toHaveTextContent('80%')
  expect(
    dialog.getByText('Matched segments').nextElementSibling
  ).toHaveTextContent('5 / 7')
  expect(dialog.queryByText('Target hit rate')).not.toBeInTheDocument()
})

test('simulated cache details show the configured range and the drawn target rate', async () => {
  const info = simulatedCacheOther.admin_info?.simulated_cache
  const preview = renderPreview({
    ...simulatedCacheOther,
    admin_info: {
      simulated_cache: {
        ...info,
        hit_rate: 85,
        hit_rate_min: 75,
        hit_rate_max: 85,
        target_rate: 81.25,
      },
    },
  })
  fireEvent.click(preview)
  const dialog = within(await screen.findByRole('dialog'))

  expect(
    dialog.getByText('Configured hit rate').nextElementSibling
  ).toHaveTextContent('75%–85%')
  expect(
    dialog.getByText('Target hit rate').nextElementSibling
  ).toHaveTextContent('81.25%')
})

function renderTokenCell(log: Partial<UsageLog>, other: LogOtherData) {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <DetailPreview
          other={other}
          isAdmin={false}
          columnId='prompt_tokens'
          log={log}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test.each([
  {
    name: 'prompt that already includes cache',
    log: { prompt_tokens: 12000, completion_tokens: 300 },
    other: { cache_tokens: 8000 },
    expected: 'Cache↓ 8,000 (66.7%)',
  },
  {
    name: 'anthropic prompt that excludes cache read and write',
    log: { prompt_tokens: 1000, completion_tokens: 300 },
    other: {
      usage_semantic: 'anthropic',
      cache_tokens: 8000,
      cache_creation_tokens: 9999,
      cache_creation_tokens_5m: 1000,
    },
    expected: 'Cache↓ 8,000 (80%)',
  },
  {
    name: 'explicit input total',
    log: { prompt_tokens: 100, completion_tokens: 1 },
    other: { cache_tokens: 300, input_tokens_total: 400 },
    expected: 'Cache↓ 300 (75%)',
  },
  {
    name: 'cache read above the prompt (clamped)',
    log: { prompt_tokens: 100, completion_tokens: 1 },
    other: { cache_tokens: 500 },
    expected: 'Cache↓ 500 (100%)',
  },
])('the token cell shows the cache hit ratio for a $name', (scenario) => {
  renderTokenCell(scenario.log, scenario.other)
  expect(screen.getByText(/^Cache↓/)).toHaveTextContent(scenario.expected)
})

test('the token cell omits the ratio when nothing was read from cache', () => {
  renderTokenCell(
    { prompt_tokens: 1000, completion_tokens: 10 },
    { cache_creation_tokens: 500 }
  )
  expect(screen.queryByText(/^Cache↓/)).not.toBeInTheDocument()
  expect(screen.queryByText(/%/)).not.toBeInTheDocument()
})

test('non-admin log details list the cache hit ratio in the token breakdown', async () => {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <DetailPreview
          other={{ cache_tokens: 3000 }}
          isAdmin={false}
          log={{ prompt_tokens: 4000, completion_tokens: 20 }}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: /./ }))
  const dialog = within(await screen.findByRole('dialog'))
  expect(
    dialog.getByText('Cache hit ratio').nextElementSibling
  ).toHaveTextContent('75%')
})

test('simulated cache details tolerate a future version with missing and unknown keys', async () => {
  const preview = renderPreview({
    admin_info: {
      simulated_cache: {
        version: 2,
        future_field: { nested: true },
        simulated: { cache_read_tokens: 1024, quota: 1000 },
      } as never,
    },
  })
  fireEvent.click(preview)
  const dialog = within(await screen.findByRole('dialog'))

  expect(comparisonRow(dialog, 'Cache Read')).toEqual(['0', '1,024'])
  expect(comparisonRow(dialog, 'Cost')).toEqual(['-', '$0.002'])
  expect(
    dialog.getByText('Cost difference').nextElementSibling
  ).toHaveTextContent('-')
  expect(
    dialog.getByText('Client format').nextElementSibling
  ).toHaveTextContent('-')
})

test('simulated cache details and row badge stay hidden outside the admin view', async () => {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <DetailPreview
          other={simulatedCacheOther}
          isAdmin={false}
          columnId='prompt_tokens'
          log={{ prompt_tokens: 12000, completion_tokens: 300 }}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(screen.getByText(/12,000/)).toBeVisible()
  expect(screen.queryByText('Simulated cache')).not.toBeInTheDocument()
  cleanup()

  const preview = renderPreview(simulatedCacheOther, false)
  fireEvent.click(preview)
  const dialog = within(await screen.findByRole('dialog'))
  expect(dialog.queryByText('Simulated cache')).not.toBeInTheDocument()
})

test('admin token cell marks rows with a simulated cache', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <DetailPreview
          other={simulatedCacheOther}
          isAdmin
          columnId='prompt_tokens'
          log={{ prompt_tokens: 12000, completion_tokens: 300 }}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(screen.getByText('Simulated cache')).toBeVisible()
})
