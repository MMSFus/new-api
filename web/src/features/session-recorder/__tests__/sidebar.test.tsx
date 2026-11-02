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
import { renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useSidebarView } from '@/hooks/use-sidebar-view'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

function setUser(
  role: number,
  adminPermissions: Record<string, Record<string, boolean>>
) {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'admin',
    role,
    permissions: { admin_permissions: adminPermissions },
    sidebar_modules: '',
  })
}

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useLocation: () => '/dashboard/overview',
}))

const savedAuth = useAuthStore.getState().auth

beforeEach(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  useAuthStore.setState({ auth: savedAuth })
})

describe('sidebar entry', () => {
  function sidebarTitles() {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(['status'], { SidebarModulesAdmin: '' })
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

  test('either read grant shows Session Recorder right after Simulated cache', () => {
    setUser(ROLE.ADMIN, {
      simulated_cache: { read: true },
      session_recorder_content: { read: true },
    })
    const titles = sidebarTitles()
    expect(titles.indexOf('Session Recorder')).toBe(
      titles.indexOf('Simulated cache') + 1
    )
  })

  test('an admin without either grant does not see Session Recorder', () => {
    setUser(ROLE.ADMIN, { simulated_cache: { read: true } })
    expect(sidebarTitles()).not.toContain('Session Recorder')
  })
})
