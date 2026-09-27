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
import { cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { checkIsActive } from '@/components/layout/lib/url-utils'
import {
  parseSidebarModulesAdmin,
  serializeSidebarModulesAdmin,
} from '@/features/system-settings/maintenance/config'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarConfig } from '../use-sidebar-config'
import { useSidebarData } from '../use-sidebar-data'

beforeEach(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  useAuthStore.getState().auth.reset()
})

function sidebarFor(admin?: object, user?: object, canConfigure = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], {
    SidebarModulesAdmin: admin ? JSON.stringify(admin) : '',
  })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'alice',
    role: 1,
    permissions: { sidebar_settings: canConfigure },
    sidebar_modules: user ? JSON.stringify(user) : '',
  })
  function Wrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
  }
  const result = renderHook(
    () => useSidebarConfig(useSidebarData().navGroups),
    { wrapper: Wrapper }
  )
  return result
}

function allItems(groups: ReturnType<typeof sidebarFor>['result']['current']) {
  return groups.flatMap((group) => group.items)
}

function itemTitled(
  groups: ReturnType<typeof sidebarFor>['result']['current'],
  title: string
) {
  return allItems(groups).find((item) => item.title === title)
}

describe('console sidebar layout', () => {
  it('regular users see one Console group with at most six fixed entries', () => {
    const { result } = sidebarFor()
    const general = result.current.filter((group) => group.id !== 'admin')
    expect(general.map((group) => group.id)).toEqual(['general'])
    const titles = general[0].items
      .filter((item) => !('type' in item && item.type === 'chat-presets'))
      .map((item) => item.title)
    expect(titles).toEqual([
      'Overview',
      'API Keys',
      'Usage Logs',
      'Wallet',
      'Playground',
      'Personal Settings',
    ])
  })

  it('admin features stay in their own group after the console entries', () => {
    const { result } = sidebarFor()
    expect(result.current.map((group) => group.id)).toEqual([
      'general',
      'admin',
    ])
    expect(
      result.current
        .find((group) => group.id === 'admin')
        ?.items.map((item) => item.title)
    ).toContain('Channels')
  })

  it.each([
    ['/dashboard', 'Overview'],
    ['/dashboard/models', 'Overview'],
    ['/usage-logs/common', 'Usage Logs'],
    ['/usage-logs/task', 'Usage Logs'],
    ['/usage-logs/drawing', 'Usage Logs'],
    ['/usage-logs/audit', 'Usage Logs'],
    ['/profile', 'Personal Settings'],
    ['/security', 'Personal Settings'],
  ])('visiting %s highlights only %s', (href, title) => {
    const { result } = sidebarFor()
    const selected = allItems(result.current).filter((item) =>
      checkIsActive(href, item)
    )
    expect(selected.map((item) => item.title)).toEqual([title])
  })
})

describe('security sidebar visibility', () => {
  it('old configurations keep Personal Settings and API Keys visible', () => {
    const { result } = sidebarFor(
      { personal: { enabled: true, personal: true, topup: true } },
      { personal: { enabled: true, personal: true } }
    )
    expect(itemTitled(result.current, 'Personal Settings')?.url).toBe(
      '/profile'
    )
    expect(itemTitled(result.current, 'API Keys')).toBeDefined()
  })

  it('disabling Profile but keeping Security opens Security & Access', () => {
    const { result } = sidebarFor({
      personal: { enabled: true, personal: false, security: true },
    })
    expect(itemTitled(result.current, 'Personal Settings')?.url).toBe(
      '/security'
    )
  })

  it.each([
    [{ personal: { enabled: false } }, undefined],
    [undefined, { personal: { enabled: false } }],
    [
      { personal: { enabled: true, personal: false, security: false } },
      undefined,
    ],
  ])(
    'admin or user disablement of both pages hides Personal Settings (%j, %j)',
    (admin, user) => {
      const { result } = sidebarFor(admin, user)
      expect(itemTitled(result.current, 'Personal Settings')).toBeUndefined()
    }
  )

  it('users without sidebar configuration permission retain the admin view', () => {
    const { result } = sidebarFor(
      undefined,
      { personal: { enabled: false } },
      false
    )
    expect(itemTitled(result.current, 'Personal Settings')).toBeDefined()
  })
})

describe('usage log sidebar entry', () => {
  it('admin settings default Audit Logs to visible and preserve its independent toggle when saved', () => {
    const config = parseSidebarModulesAdmin(
      '{"console":{"enabled":true,"log":true}}'
    )
    expect(config.console.audit).toBe(true)
    config.console.audit = false
    const saved = parseSidebarModulesAdmin(serializeSidebarModulesAdmin(config))
    expect(saved.console.audit).toBe(false)
  })

  it('hiding Usage Logs keeps the entry and opens the next enabled log page', () => {
    const { result } = sidebarFor({
      console: {
        enabled: true,
        log: false,
        task: false,
        midjourney: false,
        audit: true,
      },
    })
    expect(itemTitled(result.current, 'Usage Logs')?.url).toBe(
      '/usage-logs/audit'
    )
  })

  it.each([
    [{ console: { enabled: false } }, undefined],
    [undefined, { console: { enabled: false } }],
    [
      {
        console: {
          enabled: true,
          log: false,
          task: false,
          midjourney: false,
          audit: false,
        },
      },
      undefined,
    ],
  ])(
    'disabling every log page hides the Usage Logs entry (%j, %j)',
    (admin, user) => {
      const { result } = sidebarFor(admin, user)
      expect(itemTitled(result.current, 'Usage Logs')).toBeUndefined()
    }
  )
})
