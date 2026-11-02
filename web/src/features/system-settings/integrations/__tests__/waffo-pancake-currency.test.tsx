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
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  type WaffoPancakeBinding,
  type WaffoPancakeSettingsValues,
  WaffoPancakeSettingsSection,
} from '../waffo-pancake-settings-section'

const binding: WaffoPancakeBinding = { storeID: '', productID: '' }

function PancakeFixture(props: { saved: WaffoPancakeSettingsValues }) {
  const [values, setValues] = useState(props.saved)
  return (
    <WaffoPancakeSettingsSection
      defaultValues={props.saved}
      values={values}
      onValueChange={(key, value) =>
        setValues((prev) => ({ ...prev, [key]: value }))
      }
      selectedBinding={binding}
      savedBinding={binding}
      onSelectedBindingChange={vi.fn()}
    />
  )
}

function renderPancake(savedCurrency: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <PancakeFixture
        saved={{
          // Blank merchant ID keeps the catalog probe from firing.
          WaffoPancakeMerchantID: '',
          WaffoPancakePrivateKey: '',
          WaffoPancakeReturnURL: '',
          WaffoPancakeCurrency: savedCurrency,
        }}
      />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

function mockAdminPlans() {
  return vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        { plan: { id: 1, waffo_pancake_product_id: 'PROD_a' } },
        { plan: { id: 2, waffo_pancake_product_id: 'PROD_b' } },
        { plan: { id: 3, waffo_pancake_product_id: '' } },
      ],
    },
  })
}

describe('Waffo Pancake currency setting', () => {
  it('shows no currency warnings while the saved USD currency is unchanged', () => {
    const request = mockAdminPlans()
    renderPancake('USD')

    expect(screen.getByRole('combobox', { name: 'Currency' })).toHaveValue(
      'USD'
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(request).not.toHaveBeenCalled()
  })

  it('warns with the affected plan count and the subscription pricing caveat when switching to JPY', async () => {
    const request = mockAdminPlans()
    const user = renderPancake('USD')

    const input = screen.getByRole('combobox', { name: 'Currency' })
    await user.clear(input)
    await user.type(input, 'jpy{Enter}')

    expect(input).toHaveValue('JPY')
    expect(
      await screen.findByText(
        'Affected subscription plans with a Pancake product: 2'
      )
    ).toBeVisible()
    expect(
      screen.getByText('Settlement currency will change from USD to JPY')
    ).toBeVisible()
    expect(
      screen.getByText('Subscription prices are charged as JPY')
    ).toBeVisible()
    expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans')
  })

  it('keeps the subscription pricing caveat but drops the change warning when the saved currency is already non-USD', () => {
    const request = mockAdminPlans()
    renderPancake('EUR')

    expect(
      screen.getByText('Subscription prices are charged as EUR')
    ).toBeVisible()
    expect(
      screen.queryByText(/Settlement currency will change/)
    ).not.toBeInTheDocument()
    expect(request).not.toHaveBeenCalled()
  })
})
