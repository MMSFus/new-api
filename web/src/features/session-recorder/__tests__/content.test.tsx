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
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { RecordBody, RecordContent } from '../api'
import { bodyView } from '../content/body-view'
import {
  fail,
  mockApi,
  NOW,
  ok,
  record,
  recordList,
  renderPage,
  sessions,
  setUser,
  type Handlers,
} from './harness'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const BASE = '/api/session_recorder'
const RECORDS = `GET ${BASE}/sessions/records`
const CONTENT = `GET ${BASE}/records/rec-1/content`
const savedAuth = useAuthStore.getState().auth

function recordedBody(patch: Partial<RecordBody> = {}): RecordBody {
  return {
    content_type: 'application/json',
    size: 40,
    truncated: false,
    encoding: 'utf8',
    text: '{"model":"claude-opus-4","stream":true}',
    ...patch,
  }
}

function recordContent(patch: Partial<RecordContent> = {}): RecordContent {
  return {
    record: { request_id: 'rec-1', model: 'claude-opus-4' },
    request: recordedBody(),
    response: recordedBody({
      content_type: 'text/event-stream',
      size: 64,
      text: 'event: message_start\ndata: {"type":"message_start"}\n\n',
    }),
    source: 'local',
    ...patch,
  }
}

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  // Content access only: the tab must not depend on any revenue grant.
  setUser(ROLE.ADMIN, { session_recorder_content: { read: true } })
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})

/** Opens session → record and returns the sheet. */
async function openRecord(overrides: Handlers = {}) {
  mockApi({
    [RECORDS]: recordList([record()]),
    [CONTENT]: ok(recordContent()),
    ...overrides,
  })
  await renderPage('/session-recorder?tab=content')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'sess-1' }))
  const sheet = await screen.findByRole('dialog')
  await user.click(
    await within(sheet).findByRole('button', { name: 'Open record rec-1' })
  )
  return { user, sheet: await screen.findByRole('dialog') }
}

describe('session → records → content drill-down', () => {
  test('a session opens its record timeline in request order', async () => {
    mockApi({
      [RECORDS]: recordList([
        record(),
        record({ record_id: 'rec-2', created_at: NOW - 300, status_code: 500 }),
      ]),
    })
    await renderPage('/session-recorder?tab=content')

    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'sess-1' }))

    const timeline = await screen.findByRole('list', {
      name: 'Record timeline',
    })
    const items = within(timeline).getAllByRole('button')
    expect(items.map((item) => item.getAttribute('aria-label'))).toEqual([
      'Open record rec-1',
      'Open record rec-2',
    ])
    expect(within(items[1]).getByText('500')).toBeVisible()
  })

  test('a record shows request and response with copy and download', async () => {
    const { sheet } = await openRecord()

    const request = await within(sheet).findByRole('region', {
      name: 'Request',
    })
    expect(within(request).getByText('application/json · 40 B')).toBeVisible()
    expect(
      within(request).getByRole('button', { name: 'Download' })
    ).toBeVisible()
    expect(
      within(request).getByRole('button', { name: 'Copy code' })
    ).toBeVisible()
    const response = within(sheet).getByRole('region', { name: 'Response' })
    expect(within(response).getByText(/text\/event-stream/)).toBeVisible()
  })

  test('Back returns from the content to the record timeline', async () => {
    const { user, sheet } = await openRecord()
    await within(sheet).findByRole('region', { name: 'Request' })

    await user.click(within(sheet).getByRole('button', { name: 'Back' }))

    expect(
      await screen.findByRole('list', { name: 'Record timeline' })
    ).toBeVisible()
  })

  test('a truncated body warns that only the first 2 MiB is shown', async () => {
    const { sheet } = await openRecord({
      [CONTENT]: ok(
        recordContent({
          response: recordedBody({
            content_type: 'text/event-stream',
            size: 5_000_000,
            truncated: true,
            text: 'data: partial',
          }),
        })
      ),
    })

    const response = await within(sheet).findByRole('region', {
      name: 'Response',
    })
    expect(
      within(response).getByText(
        'Truncated: only the first 2 MiB of 5 MB is shown. Copy and download contain the shown part only.'
      )
    ).toBeVisible()
    expect(
      within(sheet).getByRole('region', { name: 'Request' })
    ).not.toHaveTextContent('Truncated')
  })

  test('content read from the archive says the first read is slower', async () => {
    const { sheet } = await openRecord({
      [CONTENT]: ok(recordContent({ source: 'archive' })),
    })

    expect(
      await within(sheet).findByText(
        'Read from the archive because the local copy was cleaned up. The first read is slower; repeat reads are cached for a while.'
      )
    ).toBeVisible()
  })

  test('local content carries no archive notice', async () => {
    const { sheet } = await openRecord()

    await within(sheet).findByRole('region', { name: 'Request' })
    expect(within(sheet).queryByText(/Read from the archive/)).toBeNull()
  })

  test('an archived record explains the wait while its content loads', async () => {
    const { sheet } = await openRecord({
      [CONTENT]: () => new Promise(() => {}),
    })

    expect(
      await within(sheet).findByText(
        'Reading from the archive. The first read can take a while.'
      )
    ).toBeVisible()
  })

  test('a missing body is reported instead of an empty viewer', async () => {
    const { sheet } = await openRecord({
      [CONTENT]: ok(recordContent({ response: null })),
    })

    const response = await within(sheet).findByRole('region', {
      name: 'Response',
    })
    expect(within(response).getByText('No body was recorded.')).toBeVisible()
  })

  test('a failed content read shows the server message and retries', async () => {
    let calls = 0
    const { user, sheet } = await openRecord({
      [CONTENT]: async () => {
        calls++
        return { data: { success: false, message: 'archive object not found' } }
      },
    })

    expect(
      await within(sheet).findByText('archive object not found')
    ).toBeVisible()
    await user.click(within(sheet).getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(calls).toBe(2))
  })
})

describe('body formatting', () => {
  test('valid JSON is displayed without changing its numeric literals', () => {
    const text = '{"id":9007199254740993,"a":1}'
    expect(bodyView(recordedBody({ text }))).toEqual({
      code: text,
      language: 'json',
      extension: 'json',
    })
  })

  test('SSE, truncated JSON and base64 bodies stay verbatim', () => {
    expect(
      bodyView(
        recordedBody({ content_type: 'text/event-stream', text: 'data: x' })
      )
    ).toEqual({ code: 'data: x', language: 'plaintext', extension: 'sse' })
    expect(
      bodyView(recordedBody({ truncated: true, text: '{"a":' })).code
    ).toBe('{"a":')
    expect(
      bodyView(recordedBody({ encoding: 'base64', text: 'AAEC' }))
    ).toEqual({ code: 'AAEC', language: 'plaintext', extension: 'b64' })
  })
})

describe('session list states', () => {
  test('no sessions in the range shows the empty state', async () => {
    mockApi({ [`GET ${BASE}/sessions`]: ok({ items: [], total: 0 }) })
    await renderPage('/session-recorder?tab=content')

    expect(await screen.findByText('No sessions')).toBeVisible()
  })

  test('a failed session list shows the server message', async () => {
    mockApi({ [`GET ${BASE}/sessions`]: fail('page_size must be at most 100') })
    await renderPage('/session-recorder?tab=content')

    expect(
      await screen.findByText('page_size must be at most 100')
    ).toBeVisible()
  })

  test('a session without records shows the empty timeline', async () => {
    mockApi({ [RECORDS]: recordList([]) })
    await renderPage('/session-recorder?tab=content')

    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'sess-1' }))

    expect(await screen.findByText('No records in this session')).toBeVisible()
  })

  test('the records request passes the session id as a query parameter', async () => {
    const api = mockApi({
      [RECORDS]: recordList([record({ session_id: 'agent/42' })]),
      [`GET ${BASE}/sessions`]: ok({
        items: [{ ...sessions.items[0], session_id: 'agent/42' }],
        total: 1,
      }),
    })
    await renderPage('/session-recorder?tab=content')

    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'agent/42' }))

    await screen.findByRole('button', { name: 'Open record rec-1' })
    expect(api.params(`${BASE}/sessions/records`)).toEqual([
      { session_id: 'agent/42' },
    ])
  })

  test('a session over the record limit says only the first records are shown', async () => {
    mockApi({ [RECORDS]: recordList([record()], 1500) })
    await renderPage('/session-recorder?tab=content')

    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'sess-1' }))

    expect(
      await screen.findByText(
        'This session has 1,500 records; only the first 1,000 are shown.'
      )
    ).toBeVisible()
  })

  test('a session within the limit shows no limit notice', async () => {
    mockApi({ [RECORDS]: recordList([record()]) })
    await renderPage('/session-recorder?tab=content')

    await userEvent
      .setup()
      .click(await screen.findByRole('button', { name: 'sess-1' }))

    await screen.findByRole('button', { name: 'Open record rec-1' })
    expect(screen.queryByText(/only the first/)).toBeNull()
  })
})
