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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { BackfillStatus } from '../api'
import {
  fail,
  idleBackfill,
  mockApi,
  NOW,
  ok,
  renderPage,
  setUser,
  type Handlers,
} from './harness'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const BACKFILL = '/api/session_recorder/backfill'
const savedAuth = useAuthStore.getState().auth

const running: BackfillStatus = {
  state: 'running',
  task_id: 'task-1',
  progress: 25,
  total_batches: 40,
  processed_batches: 10,
  indexed_records: 1234,
  skipped_batches: 1,
  failed_batches: 0,
  last_error: '',
  started_at: NOW - 600,
  finished_at: 0,
}

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  setUser(ROLE.SUPER_ADMIN)
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})

async function openPanel(overrides: Handlers = {}) {
  const api = mockApi(overrides)
  await renderPage()
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: /Historical index backfill/ })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Historical index backfill',
  })
  return { api, user, dialog }
}

test('starting asks for confirmation, then posts and shows the progress', async () => {
  // The bar follows the server's progress, not processed / total batches.
  const { api, user, dialog } = await openPanel({
    [`POST ${BACKFILL}`]: ok({ ...running, progress: 30 }),
  })

  await user.click(
    await within(dialog).findByRole('button', { name: 'Start backfill' })
  )
  expect(api.bodies('post', BACKFILL)).toHaveLength(0)
  const confirm = await screen.findByRole('alertdialog')
  await user.click(
    within(confirm).getByRole('button', { name: 'Start background backfill' })
  )

  await waitFor(() =>
    expect(api.calls.filter((call) => call.method === 'post')).toHaveLength(1)
  )
  expect(
    await within(dialog).findByRole('progressbar', { name: 'Progress' })
  ).toHaveAttribute('aria-valuenow', '30')
  expect(within(dialog).getByText('10 / 40')).toBeVisible()
  expect(within(dialog).getByText('1,234')).toBeVisible()
})

test('a running backfill marks the header entry', async () => {
  mockApi({ [`GET ${BACKFILL}`]: ok(running) })
  await renderPage()

  const entry = await screen.findByRole('button', {
    name: /Historical index backfill/,
  })
  await waitFor(() => expect(entry).toHaveTextContent('Running'))
})

test('a cancel request shows Cancelling while the server still reports running', async () => {
  const { api, user, dialog } = await openPanel({
    [`GET ${BACKFILL}`]: ok(running),
    // The run stops only after its current batch.
    [`DELETE ${BACKFILL}`]: ok(running),
  })

  await user.click(
    await within(dialog).findByRole('button', { name: 'Cancel backfill' })
  )

  expect(
    await within(dialog).findByRole('button', { name: 'Cancelling' })
  ).toBeDisabled()
  expect(
    within(dialog).getByText(
      'Cancelling: the backfill stops after the current batch. Indexed batches are kept.'
    )
  ).toBeVisible()
  expect(api.calls.filter((call) => call.method === 'delete')).toHaveLength(1)
})

test('a running backfill that was not cancelled shows the plain cancel action', async () => {
  const { dialog } = await openPanel({ [`GET ${BACKFILL}`]: ok(running) })

  expect(
    await within(dialog).findByRole('button', { name: 'Cancel backfill' })
  ).toBeEnabled()
  expect(within(dialog).queryByText(/Cancelling/)).toBeNull()
})

test('a finished run with failed batches shows them and offers a retry run', async () => {
  const { api, user, dialog } = await openPanel({
    [`GET ${BACKFILL}`]: ok({
      ...running,
      state: 'done',
      progress: 100,
      processed_batches: 40,
      failed_batches: 3,
      last_error: 'Recorded content could not be read',
      finished_at: NOW - 60,
    }),
    [`POST ${BACKFILL}`]: ok(running),
  })

  expect(
    await within(dialog).findByText(
      '3 batches failed. Run the backfill again to retry them.'
    )
  ).toBeVisible()
  expect(within(dialog).getByText('Completed with failures')).toBeVisible()
  expect(
    within(dialog).getByText('Last error: Recorded content could not be read')
  ).toBeVisible()

  await user.click(
    within(dialog).getByRole('button', { name: 'Retry failed batches' })
  )
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Start background backfill',
    })
  )
  await waitFor(() =>
    expect(api.calls.filter((call) => call.method === 'post')).toHaveLength(1)
  )
})

test('a clean finished run offers a fresh start', async () => {
  const { dialog } = await openPanel({
    [`GET ${BACKFILL}`]: ok({
      ...running,
      state: 'done',
      progress: 100,
      processed_batches: 40,
    }),
  })

  expect(
    await within(dialog).findByRole('button', { name: 'Start backfill' })
  ).toBeVisible()
  expect(within(dialog).queryByText(/batches failed/)).toBeNull()
})

test('a cancelled run with progress offers to resume', async () => {
  const { dialog } = await openPanel({
    [`GET ${BACKFILL}`]: ok({ ...running, state: 'cancelled' }),
  })

  expect(
    await within(dialog).findByRole('button', { name: 'Resume backfill' })
  ).toBeVisible()
  expect(within(dialog).getByText('Cancelled')).toBeVisible()
})

test('a failed run shows its last error', async () => {
  const { dialog } = await openPanel({
    [`GET ${BACKFILL}`]: ok({
      ...idleBackfill,
      state: 'failed',
      last_error: 'r2: access denied',
    }),
  })

  expect(
    await within(dialog).findByText('Last error: r2: access denied')
  ).toBeVisible()
})

test('a rejected start is shown in the panel instead of an unhandled rejection', async () => {
  const { user, dialog } = await openPanel({
    [`POST ${BACKFILL}`]: fail('backfill runs only on the master node'),
  })

  await user.click(
    await within(dialog).findByRole('button', { name: 'Start backfill' })
  )
  await user.click(
    within(await screen.findByRole('alertdialog')).getByRole('button', {
      name: 'Start background backfill',
    })
  )

  expect(
    await within(dialog).findByText('backfill runs only on the master node')
  ).toBeVisible()
})

test('a failed status request offers a retry', async () => {
  let calls = 0
  const { user, dialog } = await openPanel({
    [`GET ${BACKFILL}`]: async () => {
      calls++
      return { data: { success: false, message: 'forbidden' } }
    },
  })

  expect(
    await within(dialog).findByText('Failed to load backfill status')
  ).toBeVisible()
  const before = calls
  await user.click(within(dialog).getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(calls).toBeGreaterThan(before))
})
