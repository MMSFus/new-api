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

import type { RevenueRuleSet } from '../api'
import { reorderMove } from '../revenue/rules-schema'
import {
  fail,
  FULL_GRANTS,
  mockApi,
  NOW,
  ok,
  renderPage,
  rules,
  setUser,
} from './harness'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const BASE = '/api/session_recorder/revenue'
const savedAuth = useAuthStore.getState().auth

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  setUser(ROLE.ADMIN, FULL_GRANTS)
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})

async function openEditor() {
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Edit rules' }))
  const dialog = await screen.findByRole('dialog', { name: 'Revenue rules' })
  const list = await within(dialog).findByRole('list', { name: 'Rules' })
  return { user, dialog, list }
}

function ruleRow(list: HTMLElement, index: number) {
  return within(list).getAllByRole('listitem')[index]
}

function savedRules(api: ReturnType<typeof mockApi>) {
  return api.bodies('put', `${BASE}/rules`) as RevenueRuleSet[]
}

describe('ordering', () => {
  test('moving a rule up saves the new priority order', async () => {
    const api = mockApi({ [`PUT ${BASE}/rules`]: ok() })
    await renderPage()
    const { user, list } = await openEditor()

    await user.click(within(list).getByRole('button', { name: 'Move GPT up' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(savedRules(api)).toHaveLength(1))
    expect(savedRules(api)[0].rules.map((rule) => rule.id)).toEqual([
      'r2',
      'r1',
    ])
  })

  test('the drag handle reorders with the arrow keys', async () => {
    const api = mockApi({ [`PUT ${BASE}/rules`]: ok() })
    await renderPage()
    const { user, list } = await openEditor()

    within(list)
      .getByRole('button', { name: 'Drag Claude premium to reorder' })
      .focus()
    await user.keyboard('{ArrowDown}')

    expect(
      within(ruleRow(list, 0)).getByRole('textbox', { name: 'Rule name' })
    ).toHaveValue('GPT')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(savedRules(api)[0]?.rules.map((rule) => rule.id)).toEqual([
        'r2',
        'r1',
      ])
    )
  })

  test('a drag step is reduced to a single move in either direction', () => {
    expect(reorderMove(['a', 'b', 'c'], ['b', 'c', 'a'])).toEqual([0, 2])
    expect(reorderMove(['a', 'b', 'c'], ['c', 'a', 'b'])).toEqual([2, 0])
    expect(reorderMove(['a', 'b'], ['a', 'b'])).toBeNull()
  })
})

describe('editing and validation', () => {
  test('an edited price, type and switch are sent as a typed rule', async () => {
    const api = mockApi({ [`PUT ${BASE}/rules`]: ok() })
    await renderPage()
    const { user, list } = await openEditor()
    const row = ruleRow(list, 1)

    const price = within(row).getByRole('textbox', { name: 'Price per TB ($)' })
    await user.clear(price)
    await user.type(price, '650.5')
    await user.click(within(row).getByRole('switch', { name: 'Enabled' }))
    await user.click(within(row).getByRole('combobox', { name: 'Match type' }))
    await user.click(
      await screen.findByRole('option', { name: 'Regular expression (RE2)' })
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(savedRules(api)).toHaveLength(1))
    expect(savedRules(api)[0].rules[1]).toEqual({
      ...rules.rules[1],
      match_type: 'regex',
      price_per_tb: 650.5,
      enabled: false,
    })
  })

  test('a currency label over 16 characters is rejected before saving', async () => {
    const api = mockApi({ [`PUT ${BASE}/rules`]: ok() })
    await renderPage()
    const { user, dialog } = await openEditor()

    await user.type(
      within(dialog).getByRole('textbox', { name: 'Currency label' }),
      'A'.repeat(17)
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await within(dialog).findByText(
        'The currency label can be at most 16 characters'
      )
    ).toBeVisible()
    expect(savedRules(api)).toHaveLength(0)
  })

  test('empty fields and a negative price are rejected before saving', async () => {
    const api = mockApi({ [`PUT ${BASE}/rules`]: ok() })
    await renderPage()
    const { user, list } = await openEditor()
    const row = ruleRow(list, 0)

    await user.clear(within(row).getByRole('textbox', { name: 'Rule name' }))
    await user.clear(within(row).getByRole('textbox', { name: 'Expression' }))
    const price = within(row).getByRole('textbox', { name: 'Price per TB ($)' })
    await user.clear(price)
    await user.type(price, '-1')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await within(row).findByText('Rule name is required')).toBeVisible()
    expect(within(row).getByText('Expression is required')).toBeVisible()
    expect(within(row).getByText('Enter a price of 0 or more')).toBeVisible()
    expect(price).toHaveAttribute('aria-invalid', 'true')
    expect(savedRules(api)).toHaveLength(0)
  })

  test('a server validation error is shown and keeps the dialog open', async () => {
    const api = mockApi({
      [`PUT ${BASE}/rules`]: fail('rule "GPT": invalid regex pattern'),
    })
    await renderPage()
    const { user, dialog } = await openEditor()

    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await within(dialog).findByText('rule "GPT": invalid regex pattern')
    ).toBeVisible()
    expect(screen.getByRole('dialog', { name: 'Revenue rules' })).toBeVisible()
    expect(savedRules(api)).toHaveLength(1)
  })

  test('a rejected save request is shown instead of escaping as an unhandled rejection', async () => {
    mockApi({
      [`PUT ${BASE}/rules`]: () => Promise.reject(new Error('Network Error')),
    })
    await renderPage()
    const { user, dialog } = await openEditor()

    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await within(dialog).findByText('Network Error')).toBeVisible()
  })

  test('a successful save closes the dialog and reloads the summary', async () => {
    const api = mockApi({ [`PUT ${BASE}/rules`]: ok() })
    await renderPage()
    const before = api.params(`${BASE}/summary`).length
    const { user } = await openEditor()

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Revenue rules' })
      ).not.toBeInTheDocument()
    )
    await waitFor(() =>
      expect(api.params(`${BASE}/summary`).length).toBeGreaterThan(before)
    )
  })
})

describe('test model name', () => {
  test('the unsaved draft rules are tested and the matching rule is named', async () => {
    const api = mockApi({
      [`POST ${BASE}/rules/test`]: ok({
        matched_rule_id: 'r2',
        matched_rule_name: 'GPT',
      }),
    })
    await renderPage()
    const { user, list } = await openEditor()
    await user.click(within(list).getByRole('button', { name: 'Move GPT up' }))

    await user.type(
      screen.getByRole('textbox', { name: 'Test model name' }),
      'gpt-5'
    )
    await user.click(screen.getByRole('button', { name: 'Test' }))

    expect(await screen.findByText('Matches rule: GPT')).toBeVisible()
    const body = api.bodies('post', `${BASE}/rules/test`)[0] as {
      rules: { id: string }[]
      model: string
    }
    expect(body.model).toBe('gpt-5')
    expect(body.rules.map((rule) => rule.id)).toEqual(['r2', 'r1'])
  })

  test('a model no rule matches is reported as unmatched', async () => {
    mockApi({
      [`POST ${BASE}/rules/test`]: ok({
        matched_rule_id: null,
        matched_rule_name: null,
      }),
    })
    await renderPage()
    const { user } = await openEditor()

    await user.type(
      screen.getByRole('textbox', { name: 'Test model name' }),
      'llama{Enter}'
    )

    expect(
      await screen.findByText(
        'No rule matches; the record counts as unmatched.'
      )
    ).toBeVisible()
  })

  test('a failed test shows the server message', async () => {
    mockApi({ [`POST ${BASE}/rules/test`]: fail('invalid regex pattern') })
    await renderPage()
    const { user } = await openEditor()

    await user.type(
      screen.getByRole('textbox', { name: 'Test model name' }),
      'x{Enter}'
    )

    expect(await screen.findByText('invalid regex pattern')).toBeVisible()
  })
})
