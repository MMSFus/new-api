/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { ConfigGroupsSection } from '../../billing/config-groups-section'
import { BalanceBucketsSettingsSection } from '../../general/balance-buckets-settings-section'
import { DefaultThemeSection } from '../../site/default-theme-section'
import { SettingsPageProvider } from '../settings-page-context'

const clients: QueryClient[] = []

function Fixture(props: { children: ReactNode }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        {props.children}
      </SettingsPageProvider>
    </>
  )
}

function renderSettings(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <Fixture>{children}</Fixture>
    </QueryClientProvider>
  )
  vi.spyOn(toast, 'error')
  return vi
    .spyOn(api, 'put')
    .mockRejectedValueOnce(new Error('Cannot save settings'))
    .mockResolvedValue({ data: { success: true } })
}

function formOf(element: HTMLElement): HTMLFormElement {
  const form = element.closest('form')
  if (!(form instanceof HTMLFormElement)) throw new Error('form is missing')
  return form
}

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

test('failed config group save retains the edited group for a successful retry', async () => {
  const put = renderSettings(
    <ConfigGroupsSection
      defaultValue='[{"key":"best","name":"Best","groups":["vip"]}]'
      groupRatio='{"vip":1}'
      userUsableGroups='{}'
    />
  )
  const user = userEvent.setup()
  const name = screen.getByRole('textbox', { name: 'Display name' })
  await user.clear(name)
  await user.type(name, 'Updated')
  await user.click(screen.getByRole('button', { name: 'Save config groups' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
  expect(name).toHaveValue('Updated')
  await user.click(screen.getByRole('button', { name: 'Save config groups' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
  expect(put.mock.calls[1][1]).toEqual(put.mock.calls[0][1])
})

test.each(['button', 'submit'] as const)(
  'failed balance type save through %s keeps the draft and allows retry',
  async (event) => {
    const put = renderSettings(
      <BalanceBucketsSettingsSection defaultValue='{}' />
    )
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Fill Template' }))
    const save = screen.getByRole('button', {
      name: 'Save balance type settings',
    })
    if (event === 'button') {
      await user.click(save)
    } else {
      fireEvent.submit(
        formOf(screen.getByRole('button', { name: 'JSON Mode' }))
      )
    }
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    expect(save).toBeEnabled()
    await user.click(save)
    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    expect(put.mock.calls[1][1]).toEqual(put.mock.calls[0][1])
  }
)

test.each(['button', 'submit'] as const)(
  'failed default theme save through %s retains the selected preset for retry',
  async (event) => {
    const put = renderSettings(
      <DefaultThemeSection defaultValues={{ light: '', dark: '' }} />
    )
    const user = userEvent.setup()
    const light = screen.getByRole('combobox', {
      name: 'When the system is light',
    })
    await user.click(light)
    const preset = screen.getAllByRole('option')[1]
    const label = preset.textContent
    await user.click(preset)
    if (event === 'button') {
      await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    } else {
      fireEvent.submit(formOf(light))
    }
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    expect(light).toHaveTextContent(label ?? '')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    expect(put.mock.calls[1][1]).toEqual(put.mock.calls[0][1])
  }
)
