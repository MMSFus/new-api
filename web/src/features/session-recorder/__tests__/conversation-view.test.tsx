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

import type { RecordBody, RecordContent, SessionRecord } from '../api'
import {
  mockApi,
  NOW,
  ok,
  record,
  recordList,
  renderPage,
  setUser,
  type Handlers,
} from './harness'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const BASE = '/api/session_recorder'
const CONTENT = `GET ${BASE}/records/rec-1/content`
const BODY = `GET ${BASE}/records/rec-1/body`
const savedAuth = useAuthStore.getState().auth

function body(text: string, patch: Partial<RecordBody> = {}): RecordBody {
  return {
    content_type: 'application/json',
    size: text.length,
    truncated: false,
    encoding: 'utf8',
    text,
    ...patch,
  }
}

function content(request: RecordBody, response: RecordBody): RecordContent {
  return { record: {}, request, response, source: 'local' }
}

const IMAGE_REQUEST = body(
  '{"model":"gpt-image-2","prompt":"three cats","n":3}'
)

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  setUser(ROLE.ADMIN, { session_recorder_content: { read: true } })
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})

async function openRecord(row: Partial<SessionRecord>, handlers: Handlers) {
  const api = mockApi({
    [`GET ${BASE}/sessions/records`]: recordList([record(row)]),
    ...handlers,
  })
  await renderPage('/session-recorder?tab=content')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'sess-1' }))
  await user.click(
    await screen.findByRole('button', { name: 'Open record rec-1' })
  )
  return { api, user, sheet: await screen.findByRole('dialog') }
}

describe('conversation tab', () => {
  test('a chat record opens on the conversation with each role labelled', async () => {
    const { sheet } = await openRecord(
      { path: '/v1/chat/completions' },
      {
        [CONTENT]: ok(
          content(
            body(
              '{"messages":[{"role":"system","content":"Be brief."},{"role":"user","content":"Hi there"}]}'
            ),
            body(
              '{"choices":[{"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}]}'
            )
          )
        ),
      }
    )

    expect(
      await within(sheet).findByRole('tab', { name: 'Conversation' })
    ).toHaveAttribute('aria-selected', 'true')
    const request = await within(sheet).findByRole('region', {
      name: 'Request',
    })
    expect(
      within(request).getByRole('listitem', { name: 'System' })
    ).toHaveTextContent('Be brief.')
    expect(
      within(request).getByRole('listitem', { name: 'User' })
    ).toHaveTextContent('Hi there')
    const response = within(sheet).getByRole('region', { name: 'Response' })
    expect(
      within(response).getByRole('listitem', { name: 'Assistant' })
    ).toHaveTextContent('Hello!')
    expect(
      within(response).getByRole('list', { name: 'Usage' })
    ).toHaveTextContent('finish_reason: stop')
  })

  test('the Raw tab still shows the stored bodies', async () => {
    const { user, sheet } = await openRecord(
      { path: '/v1/chat/completions' },
      {
        [CONTENT]: ok(content(body('{"messages":[]}'), body('{"choices":[]}'))),
      }
    )

    await user.click(await within(sheet).findByRole('tab', { name: 'Raw' }))

    const request = await within(sheet).findByRole('region', {
      name: 'Request',
    })
    expect(within(request).getByText(/application\/json/)).toBeVisible()
    expect(
      within(sheet).getByRole('region', { name: 'Record metadata' })
    ).toBeVisible()
  })

  test('a record of an unknown protocol has no conversation tab and opens raw', async () => {
    const { sheet } = await openRecord(
      { path: '/v1/audio/speech' },
      { [CONTENT]: ok(content(body('plain'), body('audio'))) }
    )

    expect(
      await within(sheet).findByRole('tab', { name: 'Raw' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(
      within(sheet).queryByRole('tab', { name: 'Conversation' })
    ).toBeNull()
  })

  test('long histories show the latest turns until earlier ones are requested', async () => {
    const messages = Array.from({ length: 9 }, (_, i) => ({
      role: i % 2 ? 'assistant' : 'user',
      content: `turn ${i + 1}`,
    }))
    const { user, sheet } = await openRecord(
      { path: '/v1/chat/completions' },
      {
        [CONTENT]: ok(
          content(body(JSON.stringify({ messages })), body('{"choices":[]}'))
        ),
      }
    )

    const request = await within(sheet).findByRole('region', {
      name: 'Request',
    })
    expect(within(request).queryByText('turn 3')).toBeNull()
    expect(within(request).getByText('turn 9')).toBeVisible()

    await user.click(
      within(request).getByRole('button', { name: 'Show 3 earlier messages' })
    )

    expect(within(request).getByText('turn 1')).toBeVisible()
  })
})

describe('generated images', () => {
  test('every image of a multi-image response is shown in the gallery', async () => {
    const { sheet } = await openRecord(
      { path: '/v1/images/generations' },
      {
        [CONTENT]: ok(
          content(
            IMAGE_REQUEST,
            body(
              '{"data":[{"b64_json":"AAAA","revised_prompt":"cat one"},{"b64_json":"BBBB"},{"b64_json":"CCCC"}]}'
            )
          )
        ),
      }
    )

    const gallery = await within(sheet).findByRole('list', {
      name: 'Images (3)',
    })
    const images = within(gallery).getAllByRole('img')
    expect(images.map((image) => image.getAttribute('src'))).toEqual([
      'data:image/png;base64,AAAA',
      'data:image/png;base64,BBBB',
      'data:image/png;base64,CCCC',
    ])
    expect(within(gallery).getByText('cat one')).toBeVisible()
  })

  test('external images load only after the reviewer asks, without a referrer', async () => {
    const { user, sheet } = await openRecord(
      { path: '/v1/images/generations' },
      {
        [CONTENT]: ok(
          content(
            IMAGE_REQUEST,
            body(
              '{"data":[{"url":"https://img.example/1.png"},{"url":"https://img.example/2.png"}]}'
            )
          )
        ),
      }
    )

    const gallery = await within(sheet).findByRole('list', {
      name: 'Images (2)',
    })
    expect(within(gallery).queryAllByRole('img')).toHaveLength(0)
    expect(within(gallery).getAllByText('img.example')).toHaveLength(2)

    await user.click(
      within(sheet).getByRole('button', { name: 'Load external images' })
    )

    const images = within(gallery).getAllByRole('img')
    expect(images).toHaveLength(2)
    expect(images[0]).toHaveAttribute('referrerpolicy', 'no-referrer')
  })

  test('opening an image shows it large with a download link', async () => {
    const { user, sheet } = await openRecord(
      { path: '/v1/images/generations' },
      {
        [CONTENT]: ok(
          content(IMAGE_REQUEST, body('{"data":[{"b64_json":"AAAA"}]}'))
        ),
      }
    )

    await user.click(
      await within(sheet).findByRole('button', { name: 'Open Image 1' })
    )

    const preview = await screen.findByRole('dialog', { name: 'Image 1' })
    expect(
      within(preview).getByRole('link', { name: 'Download' })
    ).toHaveAttribute('download', expect.stringMatching(/\.png$/))
  })

  test('a truncated response is completed from the full body download', async () => {
    const full = '{"data":[{"b64_json":"AAAA"},{"b64_json":"BBBB"}]}'
    const { api, sheet } = await openRecord(
      { path: '/v1/images/generations' },
      {
        [CONTENT]: ok(
          content(
            IMAGE_REQUEST,
            body('{"data":[{"b64_json":"AA', {
              truncated: true,
              size: full.length,
            })
          )
        ),
        [BODY]: async () => ({
          data: new Blob([full]),
          headers: {
            'x-recorder-truncated': 'false',
            'content-type': 'application/json',
          },
        }),
      }
    )

    expect(
      await within(sheet).findByRole('list', { name: 'Images (2)' })
    ).toBeVisible()
    expect(api.params(`${BASE}/records/rec-1/body`)).toEqual([
      { kind: 'response' },
    ])
    expect(within(sheet).queryByText(/may be missing content/)).toBeNull()
  })

  test('a failed full body download falls back to the shown part with a warning', async () => {
    const { sheet } = await openRecord(
      { path: '/v1/images/generations' },
      {
        [CONTENT]: ok(
          content(
            IMAGE_REQUEST,
            body('{"data":[{"b64_json":"AAAA"},{"b64_json":"BB', {
              truncated: true,
              size: 9_000_000,
            })
          )
        ),
        [BODY]: async () => {
          throw new Error('network down')
        },
      }
    )

    expect(
      await within(sheet).findByText(/only the first 2 MiB is shown/)
    ).toBeVisible()
    await waitFor(() =>
      expect(
        within(sheet).getByRole('list', { name: 'Images (1)' })
      ).toBeVisible()
    )
  })
})
