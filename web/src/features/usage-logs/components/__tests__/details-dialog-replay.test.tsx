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
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import type { UsageLog } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({
    models: [],
    priceRate: 1,
    usdExchangeRate: 1,
  }),
}))

function makeLog(other: Record<string, unknown>): UsageLog {
  return {
    id: 1,
    user_id: 2,
    created_at: 1,
    type: 2,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'claude-test',
    quota: 100,
    prompt_tokens: 1,
    completion_tokens: 1,
    use_time: 1,
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

async function renderDetails(
  isAdmin: boolean,
  other: Record<string, unknown> = { replay_hit: true }
) {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <DetailsDialog
          log={makeLog(other)}
          isAdmin={isAdmin}
          isRoot={false}
          open
          onOpenChange={() => undefined}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => vi.clearAllMocks())

test('replayed log shows the replay marker to administrators', async () => {
  await renderDetails(true)
  expect(await screen.findByText('Replayed response')).toBeInTheDocument()
})

test('replayed log shows the replay marker to non-admin users', async () => {
  await renderDetails(false)
  expect(await screen.findByText('Replayed response')).toBeInTheDocument()
})

test('log without replay_hit does not show the replay marker', async () => {
  await renderDetails(false, {})
  await screen.findByRole('dialog')
  expect(screen.queryByText('Replayed response')).not.toBeInTheDocument()
})
