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
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/http-client'
import { createAppQueryClient } from '@/lib/query-client'
import { useAuthStore } from '@/stores/auth-store'
import { usePricingPreferencesStore } from '@/stores/pricing-preferences-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
  type CurrencyDisplayType,
} from '@/stores/system-config-store'

import type { ModelPricingConfig } from '../api'
import { ModelPricingPanel } from '../model-pricing-panel'
import { pricingOptions, type PricingValues } from '../pricing'

const originalAdapter = api.defaults.adapter
let client: QueryClient | undefined
const expression = 'tier("base", p * 8 + c * 16)'
let siteDisplayType: CurrencyDisplayType = 'CNY'

function setSite(quotaDisplayType: CurrencyDisplayType) {
  siteDisplayType = quotaDisplayType
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType,
      usdExchangeRate: 8,
    },
  })
}

function renderPanel(
  configured: PricingValues,
  patch: () => { success: boolean; message?: string } = () => ({
    success: true,
  })
) {
  const snapshot: ModelPricingConfig = {
    entries: [
      {
        model_name: 'example',
        version: 'v1',
        configured,
        effective: configured,
      },
    ],
    options: pricingOptions({}),
    empty_version: 'empty',
  }
  const saved: Array<{ changes: Array<{ pricing: PricingValues }> }> = []
  const adapter: AxiosAdapter = async (config) => {
    const respond = (data: unknown) => ({
      data,
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
    if (config.method === 'get' && config.url === '/api/status') {
      // The status query refreshes the currency config in the store.
      return respond({
        success: true,
        data: { quota_display_type: siteDisplayType, usd_exchange_rate: 8 },
      })
    }
    if (config.method === 'get' && config.url === '/api/pricing') {
      return respond({ success: true, data: [], vendors: [] })
    }
    if (config.url === '/api/option/model_pricing') {
      if (config.method === 'get') {
        return respond({ success: true, data: snapshot })
      }
      saved.push(JSON.parse(config.data))
      return respond(patch())
    }
    throw new Error(`Unexpected request: ${config.method} ${config.url}`)
  }
  api.defaults.adapter = adapter
  client = createAppQueryClient()
  render(
    <QueryClientProvider client={client}>
      <ModelPricingPanel modelName='example' />
    </QueryClientProvider>
  )
  return saved
}

async function selectPriceCurrency(option: string) {
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('combobox', { name: 'Price currency' })
  )
  await user.click(await screen.findByRole('option', { name: option }))
}

async function save() {
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Save model prices' }))
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'administrator', role: 100 })
  usePricingPreferencesStore.setState({ currency: 'USD' })
  setSite('CNY')
})

afterEach(() => {
  cleanup()
  client?.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.setUser(null)
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  localStorage.clear()
  vi.restoreAllMocks()
})

const expressionPricing: PricingValues = {
  'billing_setting.billing_mode': 'tiered_expr',
  'billing_setting.billing_expr': expression,
}

describe('model price currency', () => {
  it('saves a newly selected currency with the pricing draft', async () => {
    const saved = renderPanel(expressionPricing)
    await selectPriceCurrency('Chinese yuan (CNY)')
    await save()
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(saved[0].changes[0].pricing).toMatchObject({
      ModelPriceCurrency: 'CNY',
      'billing_setting.billing_expr': expression,
    })
  })

  it('switching back to USD omits the mark so the backend clears it', async () => {
    const saved = renderPanel({
      ...expressionPricing,
      ModelPriceCurrency: 'CNY',
    })
    await selectPriceCurrency('US dollar (USD)')
    await save()
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(saved[0].changes[0].pricing).not.toHaveProperty('ModelPriceCurrency')
  })

  it('edits a CNY price as stored, with the CNY symbol and a USD reference', async () => {
    renderPanel({ ...expressionPricing, ModelPriceCurrency: 'CNY' })
    const input = await screen.findByRole('textbox', { name: 'Input price' })
    expect(input).toHaveValue('8')
    expect(input.parentElement).toHaveTextContent('≈ $1')
    // Raw coefficients are labelled with the currency they are written in.
    expect(
      screen.getByRole('complementary', { name: 'Preview' })
    ).toHaveTextContent('Expression (CNY)')
    expect(
      screen.queryByRole('combobox', { name: 'Pricing currency' })
    ).not.toBeInTheDocument()
  })

  it('flags a CNY mark that no longer matches a USD display type', async () => {
    setSite('USD')
    renderPanel({ ...expressionPricing, ModelPriceCurrency: 'CNY' })
    expect(
      await screen.findByText(/which is not the current display currency/)
    ).toBeInTheDocument()
  })

  it('shows the backend currency validation error and keeps the draft', async () => {
    const notify = vi.spyOn(toast, 'error').mockReturnValue('pricing-error')
    renderPanel(expressionPricing, () => ({
      success: false,
      message: 'The selected currency has no valid exchange rate',
    }))
    await selectPriceCurrency('Chinese yuan (CNY)')
    await save()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'The selected currency has no valid exchange rate'
    )
    expect(notify).toHaveBeenCalledWith(
      'The selected currency has no valid exchange rate'
    )
    expect(
      screen.getByRole('combobox', { name: 'Price currency' })
    ).toHaveTextContent('Chinese yuan (CNY)')
  })
})
