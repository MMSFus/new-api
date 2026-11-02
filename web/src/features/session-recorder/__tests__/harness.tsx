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
// Shared fixtures of the session recorder page tests: the page mounted on its
// route, the api layer answered by URL, and users with given grants.
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { vi } from 'vitest'

import { ThemeProvider } from '@/context/theme-provider'
import en from '@/i18n/locales/en.json'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SessionRecorder } from '..'
import type {
  BackfillStatus,
  RevenueRuleSet,
  RevenueSummary,
  SessionPage,
  SessionRecord,
} from '../api'
import { sessionRecorderSearchSchema } from '../lib'

export const NOW = 1_800_000_000

type Method = 'get' | 'post' | 'put' | 'delete'
export type Handler = (body?: unknown, config?: unknown) => Promise<unknown>
export type Handlers = Partial<Record<string, Handler>>

export function ok(data?: unknown) {
  return async () => ({ data: { success: true, data } })
}

export function fail(message: string) {
  return async () => ({ data: { success: false, message } })
}

export const rules: RevenueRuleSet = {
  rules: [
    {
      id: 'r1',
      name: 'Claude premium',
      match_type: 'glob',
      pattern: 'claude-opus-*',
      price_per_tb: 1200,
      enabled: true,
    },
    {
      id: 'r2',
      name: 'GPT',
      match_type: 'keyword',
      pattern: 'gpt',
      price_per_tb: 600,
      enabled: true,
    },
  ],
  currency_label: '',
}

export const summary: RevenueSummary = {
  totals: {
    records: 30,
    raw_bytes: 9_000_000_000_000,
    archive_bytes: 2_500_000_000_000,
    revenue: 2700,
    unarchived_records: 4,
    unarchived_raw_bytes: 3_000_000,
  },
  by_rule: [
    {
      rule_id: 'r1',
      rule_name: 'Claude premium',
      price_per_tb: 1200,
      records: 20,
      archive_bytes: 2_000_000_000_000,
      revenue: 2400,
    },
    {
      rule_id: 'r2',
      rule_name: 'GPT',
      price_per_tb: 600,
      records: 8,
      archive_bytes: 500_000_000_000,
      revenue: 300,
    },
  ],
  unmatched: { records: 2, archive_bytes: 1_000_000 },
  by_model: [
    {
      model_name: 'claude-opus-4',
      rule_id: 'r1',
      records: 20,
      raw_bytes: 7_000_000_000_000,
      archive_bytes: 2_000_000_000_000,
      revenue: 2400,
    },
    {
      model_name: 'gpt-5',
      rule_id: 'r2',
      records: 8,
      raw_bytes: 2_000_000_000_000,
      archive_bytes: 500_000_000_000,
      revenue: 300,
    },
  ],
  trend: [
    { bucket: NOW - 86_400, archive_bytes: 2_500_000_000_000, revenue: 2700 },
  ],
}

export const sessions: SessionPage = {
  items: [
    {
      session_id: 'sess-1',
      first_at: NOW - 600,
      last_at: NOW - 60,
      records: 2,
      models: ['claude-opus-4'],
      user_ids: [7],
      raw_bytes: 4000,
      archive_bytes: 1000,
    },
  ],
  total: 1,
}

/** The records endpoint body for `items`, all of the session shown. */
export function recordList(items: SessionRecord[], total = items.length) {
  return ok({ items, total, limit: 1000 })
}

export function record(patch: Partial<SessionRecord> = {}): SessionRecord {
  return {
    record_id: 'rec-1',
    session_id: 'sess-1',
    created_at: NOW - 600,
    duration_ms: 1200,
    model_name: 'claude-opus-4',
    group: 'default',
    channel_id: 3,
    user_id: 7,
    token_id: 9,
    status_code: 200,
    stream: true,
    path: '/v1/messages',
    raw_bytes: 2000,
    archive_bytes: 500,
    batch_id: 'b1',
    object_key: 'recordings/b1.tar.gz',
    root_id: 'root-1',
    record_path: 'records/rec-1.jsonl',
    archived_at: NOW - 300,
    ...patch,
  }
}

export const idleBackfill: BackfillStatus = {
  state: 'idle',
  task_id: '',
  progress: 0,
  total_batches: 0,
  processed_batches: 0,
  indexed_records: 0,
  skipped_batches: 0,
  failed_batches: 0,
  last_error: '',
  started_at: 0,
  finished_at: 0,
}

/**
 * Answers `METHOD url` (e.g. `GET /api/session_recorder/sessions`) with a
 * handler; defaults cover the page's queries. Unknown requests reject.
 */
export function mockApi(overrides: Handlers = {}) {
  const base = '/api/session_recorder'
  const defaults: Handlers = {
    [`GET ${base}/revenue/rules`]: ok(rules),
    [`GET ${base}/revenue/summary`]: ok(summary),
    [`GET ${base}/sessions`]: ok(sessions),
    [`GET ${base}/backfill`]: ok(idleBackfill),
  }
  const calls: {
    method: Method
    url: string
    body?: unknown
    config?: unknown
  }[] = []
  const spies = {} as Record<Method, ReturnType<typeof vi.spyOn>>
  for (const method of ['get', 'post', 'put', 'delete'] as const) {
    spies[method] = vi.spyOn(api, method).mockImplementation(((
      url: string,
      second?: unknown,
      third?: unknown
    ) => {
      // get/delete take (url, config); post/put take (url, body, config).
      const withBody = method === 'post' || method === 'put'
      const body = withBody ? second : undefined
      const config = withBody ? third : second
      calls.push({ method, url, body, config })
      const key = `${method.toUpperCase()} ${url}`
      const respond = overrides[key] ?? defaults[key]
      if (!respond) return Promise.reject(new Error(`unexpected ${key}`))
      return respond(body, config)
    }) as never)
  }
  return {
    calls,
    params: (url: string) =>
      calls
        .filter((call) => call.method === 'get' && call.url === url)
        .map(
          (call) =>
            (call.config as { params?: Record<string, unknown> })?.params
        ),
    bodies: (method: Method, url: string) =>
      calls
        .filter((call) => call.method === method && call.url === url)
        .map((call) => call.body),
  }
}

export function setUser(
  role: number,
  adminPermissions?: Record<string, Record<string, boolean>>
) {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: {
        id: role === ROLE.SUPER_ADMIN ? 1 : 2,
        username: role === ROLE.SUPER_ADMIN ? 'root' : 'admin',
        role,
        permissions: { admin_permissions: adminPermissions },
      },
    },
  }))
}

export const FULL_GRANTS = {
  session_recorder_revenue: { read: true, write: true },
  session_recorder_content: { read: true },
}

export async function renderPage(path = '/session-recorder') {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const root = createRootRoute()
  const layout = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => layout,
    path: '/session-recorder/',
    validateSearch: sessionRecorderSearchSchema,
    component: SessionRecorder,
  })
  const router = createRouter({
    routeTree: root.addChildren([layout.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await router.load()
  const view = render(
    <I18nextProvider i18n={i18n}>
      <ThemeProvider>
        <QueryClientProvider client={client}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ThemeProvider>
    </I18nextProvider>
  )
  return { router, client, view }
}
