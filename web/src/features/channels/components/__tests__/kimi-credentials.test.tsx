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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { KimiCredentials } from '../kimi-credentials'

function mount(disabled = false, value = '{}', channelId?: number) {
  const onImport = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const props = { disabled, value, channelId, proxy: '', onImport }
  const view = render(
    <QueryClientProvider client={client}>
      <KimiCredentials {...props} />
    </QueryClientProvider>
  )
  return {
    onImport,
    user: userEvent.setup(
      vi.isFakeTimers() ? { advanceTimers: vi.advanceTimersByTime } : {}
    ),
    ...view,
    rerenderProps: (next: Partial<typeof props>) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <KimiCredentials {...props} {...next} />
        </QueryClientProvider>
      ),
  }
}
const device = {
  handle: 'poll-first',
  user_code: 'TEST-CODE',
  verification_uri: 'https://auth.kimi.ai/device',
  interval: 5,
  expires_at: Math.floor(Date.now() / 1000) + 600,
}
const success = (data: unknown) => ({ data: { success: true, data } })
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

test('international selection starts official authorization and disables competing imports', async () => {
  vi.spyOn(api, 'post').mockResolvedValue(success(device))
  const { user, unmount } = mount()
  await user.selectOptions(screen.getByLabelText('Kimi region'), 'global')
  await user.click(
    screen.getByRole('button', { name: 'Sign in with Kimi Code' })
  )
  expect(
    await screen.findByRole('link', { name: 'Open Kimi authorization' })
  ).toHaveAttribute('href', device.verification_uri)
  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/kimi/oauth/start',
    { region: 'global', proxy: '' },
    expect.anything()
  )
  expect(
    screen.getByRole('button', { name: 'Preview pasted Kimi JSON' })
  ).toBeDisabled()
  expect(
    screen.getByLabelText('Import sub2api or CPA JSON files')
  ).toBeDisabled()
  unmount()
  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/kimi/oauth/cancel',
    { handle: device.handle },
    expect.anything()
  )
})

test('polling honors slow-down interval and returns only the opaque ready key to the form', async () => {
  vi.useFakeTimers()
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url.endsWith('/start')) return success(device)
    return success({ status: 'pending', handle: 'poll-next', interval: 10 })
  })
  const { onImport } = mount()
  fireEvent.click(
    screen.getByRole('button', { name: 'Sign in with Kimi Code' })
  )
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(
    screen.getByRole('link', { name: 'Open Kimi authorization' })
  ).toBeInTheDocument()
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000)
  })
  expect(post).toHaveBeenLastCalledWith(
    '/api/channel/kimi/oauth/poll',
    { handle: 'poll-first' },
    expect.anything()
  )
  const requests = post.mock.calls.length
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000)
  })
  expect(post.mock.calls.length).toBe(requests)
  post.mockResolvedValue(
    success({ status: 'ready', key: 'kimi-oauth:test-ready', count: 1 })
  )
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000)
  })
  expect(onImport).toHaveBeenCalledWith({
    key: 'kimi-oauth:test-ready',
    count: 1,
  })
  expect(screen.getByRole('status')).toHaveTextContent(
    'Save the channel to apply'
  )
  expect(
    screen.queryByRole('link', { name: 'Open Kimi authorization' })
  ).not.toBeInTheDocument()
})

test('changing region cancels a late start result without replacing credentials', async () => {
  let finish!: (value: unknown) => void
  const post = vi.spyOn(api, 'post').mockImplementation((url) =>
    url.endsWith('/start')
      ? new Promise((resolve) => {
          finish = resolve
        })
      : Promise.resolve(success(null))
  )
  const { user, onImport } = mount()
  await user.click(
    screen.getByRole('button', { name: 'Sign in with Kimi Code' })
  )
  await user.selectOptions(screen.getByLabelText('Kimi region'), 'global')
  await act(async () => {
    finish(success(device))
  })
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/channel/kimi/oauth/cancel',
      { handle: device.handle },
      expect.anything()
    )
  )
  expect(onImport).not.toHaveBeenCalled()
  expect(
    screen.queryByRole('link', { name: 'Open Kimi authorization' })
  ).not.toBeInTheDocument()
})

test('mixed import previews errors and applies only selected valid accounts', async () => {
  vi.spyOn(api, 'post').mockResolvedValue(
    success({
      key: 'first\nsecond',
      count: 2,
      issues: [{ name: 'bad', error: 'Invalid Kimi token expiry' }],
      accounts: [
        {
          key: 'first',
          name: 'First',
          region: 'mainland-cn',
          auth_type: 'oauth',
          expires_at: 0,
          renewable: true,
        },
        {
          key: 'second',
          name: 'Second',
          region: 'global',
          auth_type: 'api_key',
          expires_at: 0,
          renewable: false,
        },
      ],
    })
  )
  const { user, onImport } = mount()
  await user.click(
    screen.getByRole('button', { name: 'Preview pasted Kimi JSON' })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Invalid Kimi token expiry'
  )
  await user.click(screen.getByRole('checkbox', { name: /Second/ }))
  await user.click(
    screen.getByRole('button', { name: 'Apply selected Kimi accounts' })
  )
  expect(onImport).toHaveBeenCalledWith({ key: 'first', count: 1 })
})

test('multiple JSONL files are previewed together without applying until confirmed', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue(
    success({
      key: 'first',
      count: 1,
      issues: [],
      accounts: [
        {
          key: 'first',
          name: 'First',
          region: 'global',
          auth_type: 'oauth',
          expires_at: 0,
          renewable: true,
        },
      ],
    })
  )
  const { user, onImport } = mount()
  const raw =
    '{"type":"kimi-ai","access_token":"test-a"}\n{"type":"kimi-ai","access_token":"test-b"}'
  const file = new File([raw], 'kimi.jsonl', { type: 'application/json' })
  Object.defineProperty(file, 'text', { value: async () => raw })
  await user.upload(screen.getByLabelText('Import sub2api or CPA JSON files'), [
    file,
  ])
  await screen.findByRole('button', { name: 'Apply selected Kimi accounts' })
  expect(post).toHaveBeenCalledWith(
    '/api/channel/kimi/import',
    {
      json: JSON.stringify([
        { type: 'kimi-ai', access_token: 'test-a' },
        { type: 'kimi-ai', access_token: 'test-b' },
      ]),
      region: 'mainland-cn',
    },
    expect.anything()
  )
  expect(onImport).not.toHaveBeenCalled()
})

test('disabled and failed operations never replace channel credentials', async () => {
  const { rerenderProps, user, onImport } = mount(true)
  expect(
    screen.getByRole('button', { name: 'Sign in with Kimi Code' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Preview pasted Kimi JSON' })
  ).toBeDisabled()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, message: 'Invalid Kimi credential JSON' },
  })
  rerenderProps({ disabled: false })
  await user.click(
    screen.getByRole('button', { name: 'Preview pasted Kimi JSON' })
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Preview pasted Kimi JSON' })
    ).toBeEnabled()
  )
  expect(onImport).not.toHaveBeenCalled()
})

test('subscription ratios are shown as percentages independently of credential import', async () => {
  vi.spyOn(api, 'get').mockResolvedValue(
    success([{ name: 'limit_5h', used_ratio: 0.25, reset_time: '' }])
  )
  const { user, onImport } = mount(false, '', 9)
  await user.click(
    screen.getByRole('button', { name: 'Check Kimi subscription usage' })
  )
  expect(await screen.findByText(/Five-hour usage: 25%/)).toBeInTheDocument()
  expect(onImport).not.toHaveBeenCalled()
})
