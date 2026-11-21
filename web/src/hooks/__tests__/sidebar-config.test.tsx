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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { checkIsActive } from '@/components/layout/lib/url-utils'
import { invalidateModelPricing } from '@/features/model-pricing/api'
import {
  parseSidebarModulesAdmin,
  serializeSidebarModulesAdmin,
} from '@/features/system-settings/maintenance/config'
import {
  MODEL_PRICING_ISSUES_QUERY_KEY,
  type ModelPricingIssue,
} from '@/features/system-settings/models/pricing-issues'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarConfig } from '../use-sidebar-config'
import { useSidebarData } from '../use-sidebar-data'
import { useSidebarView } from '../use-sidebar-view'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useLocation: () => '/dashboard/overview',
}))

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

describe('permission-gated admin entries', () => {
  function adminSidebarTitles(
    role: number,
    adminPermissions?: Record<string, Record<string, boolean>>
  ) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(['status'], { SidebarModulesAdmin: '' })
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'bob',
      role,
      permissions: { admin_permissions: adminPermissions },
      sidebar_modules: '',
    })
    function Wrapper(props: { children: ReactNode }) {
      return (
        <QueryClientProvider client={client}>
          {props.children}
        </QueryClientProvider>
      )
    }
    const { result } = renderHook(() => useSidebarView(), { wrapper: Wrapper })
    return result.current.navGroups
      .flatMap((group) => group.items)
      .map((item) => item.title)
  }

  it('admins without the grants do not see Cost & Billing or Simulated cache', () => {
    const titles = adminSidebarTitles(10)
    expect(titles).toContain('Channels')
    expect(titles).not.toContain('Cost & Billing')
    expect(titles).not.toContain('Simulated cache')
  })

  it('each grant reveals only its own entry', () => {
    const titles = adminSidebarTitles(10, { channel_cost: { read: true } })
    expect(titles).toContain('Cost & Billing')
    expect(titles).not.toContain('Simulated cache')
  })

  it('root sees both entries without explicit grants', () => {
    const titles = adminSidebarTitles(100)
    expect(titles).toContain('Cost & Billing')
    expect(titles).toContain('Simulated cache')
  })
})

describe('model pricing badge', () => {
  function issue(modelName: string, pluginKey: string): ModelPricingIssue {
    return {
      model_name: modelName,
      plugin_key: pluginKey,
      plugin_name: pluginKey,
      reason: 'unconfigured',
    }
  }

  /** Lists the sidebar entries of the current view that carry a badge. */
  function BadgedEntries() {
    const { navGroups } = useSidebarView()
    const entries = navGroups
      .flatMap((group) => group.items)
      .flatMap((item) => [item, ...(item.items ?? [])])
      .filter((item) => item.badge)
    return (
      <ul aria-label='Badged entries'>
        {entries.map((item) => (
          <li key={item.title}>{`${item.title}: ${item.badge}`}</li>
        ))}
      </ul>
    )
  }

  async function renderSidebar(
    path: string,
    role: number,
    issues: () => ModelPricingIssue[]
  ) {
    const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
      data: {
        success: true,
        data: url === '/api/option/model_pricing/issues' ? issues() : {},
      },
    }))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(['status'], { SidebarModulesAdmin: '' })
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role })
    const router = createRouter({
      routeTree: createRootRoute({ component: BadgedEntries }),
      history: createMemoryHistory({ initialEntries: [path] }),
    })
    await router.load()
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    return { client, get }
  }

  function badgedEntries() {
    return screen
      .getByRole('list', { name: 'Badged entries' })
      .querySelectorAll('li')
  }

  it('counts each task model once on the System Settings entry of the main sidebar', async () => {
    await renderSidebar('/dashboard/overview', ROLE.SUPER_ADMIN, () => [
      issue('kling-v3', 'kling'),
      issue('shared-video', 'alpha'),
      issue('shared-video', 'beta'),
    ])

    expect(await screen.findByText('System Settings: 2')).toBeInTheDocument()
    expect(badgedEntries()).toHaveLength(1)
  })

  it('inside System Settings shows the count on Billing & Payment and Model Pricing', async () => {
    await renderSidebar('/system-settings/site', ROLE.SUPER_ADMIN, () => [
      issue('kling-v3', 'kling'),
    ])

    expect(await screen.findByText('Model Pricing: 1')).toBeInTheDocument()
    expect(screen.getByText('Billing & Payment: 1')).toBeInTheDocument()
    expect(badgedEntries()).toHaveLength(2)
  })

  it('follows plugin changes and saved prices without a page reload', async () => {
    let issues = [issue('kling-v3', 'kling')]
    const { client } = await renderSidebar(
      '/dashboard/overview',
      ROLE.SUPER_ADMIN,
      () => issues
    )
    await screen.findByText('System Settings: 1')

    // A plugin action refreshes the count it may have changed.
    issues = [issue('kling-v3', 'kling'), issue('vidu-q3', 'vidu')]
    await act(() =>
      client.invalidateQueries({ queryKey: MODEL_PRICING_ISSUES_QUERY_KEY })
    )
    expect(await screen.findByText('System Settings: 2')).toBeInTheDocument()

    // Every pricing save runs this invalidation.
    issues = []
    await act(() => invalidateModelPricing(client))
    await waitFor(() => expect(badgedEntries()).toHaveLength(0))
  })

  it('never asks for the count below the super administrator role', async () => {
    const { get } = await renderSidebar(
      '/dashboard/overview',
      ROLE.ADMIN,
      () => [issue('kling-v3', 'kling')]
    )

    expect(
      await screen.findByRole('list', { name: 'Badged entries' })
    ).toBeInTheDocument()
    expect(get).not.toHaveBeenCalledWith('/api/option/model_pricing/issues')
    expect(badgedEntries()).toHaveLength(0)
  })
})
