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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { AntigravityCredentials } from '../antigravity-credentials'

function mount(disabled = false, value = '') {
  const onImport = vi.fn()
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AntigravityCredentials
        disabled={disabled}
        value={value}
        proxy=''
        onImport={onImport}
      />
    </QueryClientProvider>
  )
  return { onImport, user: userEvent.setup() }
}

afterEach(() => vi.restoreAllMocks())

test('Chinese locale renders credential actions from the translation namespace', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'zh',
    resources: { zh },
    interpolation: { escapeValue: false },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient()}>
        <AntigravityCredentials
          disabled={false}
          proxy=''
          value=''
          onImport={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(
    screen.getByRole('button', { name: '使用 Google 登录' })
  ).toBeInTheDocument()
  expect(
    screen.getByLabelText('导入 sub2api 或 CPA JSON 文件')
  ).toBeInTheDocument()
})

test('Google authorization exchanges the complete callback and places credentials in the channel form', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url.endsWith('/start')) {
      return {
        data: {
          success: true,
          data: {
            authorization_url: 'https://accounts.google.com/test',
            expires_at: 100,
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: { key: '{"refresh_token":"test"}', count: 1 },
      },
    }
  })
  const { user, onImport } = mount()
  await user.click(screen.getByRole('button', { name: 'Sign in with Google' }))
  expect(
    await screen.findByRole('link', { name: 'Open Google authorization' })
  ).toHaveAttribute('href', 'https://accounts.google.com/test')
  const callback = 'http://localhost:8085/callback?code=test&state=test-state'
  await user.type(screen.getByLabelText('Complete callback URL'), callback)
  await user.click(
    screen.getByRole('button', { name: 'Complete authorization' })
  )
  await waitFor(() =>
    expect(onImport).toHaveBeenCalledWith({
      key: '{"refresh_token":"test"}',
      count: 1,
    })
  )
  expect(post).toHaveBeenLastCalledWith(
    '/api/channel/antigravity/oauth/exchange',
    { callback_url: callback },
    expect.anything()
  )
  expect(
    screen.queryByLabelText('Complete callback URL')
  ).not.toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent(
    'Save the channel to apply'
  )
})

test('pasted JSON is validated by the backend and multi-account results are passed to the form', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { key: 'first\nsecond', count: 2 } },
  })
  const { user, onImport } = mount(false, '{"accounts":[]}')
  await user.click(screen.getByRole('button', { name: 'Import pasted JSON' }))
  await waitFor(() =>
    expect(onImport).toHaveBeenCalledWith({ key: 'first\nsecond', count: 2 })
  )
  expect(post).toHaveBeenCalledWith(
    '/api/channel/antigravity/import',
    { json: '{"accounts":[]}' },
    expect.anything()
  )
})

test('multiple JSON files use one atomic import and the file control can be used again', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { key: 'first\nsecond', count: 2 } },
  })
  const { user, onImport } = mount()
  const files = ['first', 'second'].map((name) => {
    const content = JSON.stringify({ type: 'antigravity', refresh_token: name })
    const file = new File([content], `${name}.json`, {
      type: 'application/json',
    })
    Object.defineProperty(file, 'text', { value: async () => content })
    return file
  })
  const input = screen.getByLabelText('Import sub2api or CPA JSON files')
  await user.upload(input, files)
  await waitFor(() =>
    expect(onImport).toHaveBeenCalledWith({ key: 'first\nsecond', count: 2 })
  )
  expect(post).toHaveBeenCalledTimes(1)
  expect((input as HTMLInputElement).value).toBe('')
})

test('disabled credentials cannot initiate OAuth or import files or pasted JSON', () => {
  mount(true, '{}')
  expect(
    screen.getByRole('button', { name: 'Sign in with Google' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Import pasted JSON' })
  ).toBeDisabled()
  expect(
    screen.getByLabelText('Import sub2api or CPA JSON files')
  ).toBeDisabled()
})

test('failed JSON import does not replace channel credentials', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, message: 'Invalid Antigravity credential JSON' },
  })
  const { user, onImport } = mount(false, 'invalid')
  await user.click(screen.getByRole('button', { name: 'Import pasted JSON' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Import pasted JSON' })
    ).toBeEnabled()
  )
  expect(onImport).not.toHaveBeenCalled()
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})

test('pending OAuth start disables competing credential actions', async () => {
  let finish!: (value: unknown) => void
  vi.spyOn(api, 'post').mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve
      })
  )
  const { user } = mount(false, '{}')
  await user.click(screen.getByRole('button', { name: 'Sign in with Google' }))
  expect(
    screen.getByRole('button', { name: 'Import pasted JSON' })
  ).toBeDisabled()
  expect(
    screen.getByLabelText('Import sub2api or CPA JSON files')
  ).toBeDisabled()
  finish({
    data: {
      success: true,
      data: {
        authorization_url: 'https://accounts.google.com/test',
        expires_at: 100,
      },
    },
  })
  await screen.findByLabelText('Complete callback URL')
})
