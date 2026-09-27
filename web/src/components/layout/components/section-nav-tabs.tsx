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
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSidebarConfig } from '@/hooks/use-sidebar-config'
import { cn } from '@/lib/utils'

import type { NavGroup, NavLink } from '../types'

export type SectionNavTab = {
  value: string
  label: string
  url: NavLink['url']
}

type SectionNavTabsProps = {
  tabs: SectionNavTab[]
  value: string
  className?: string
}

/**
 * Tabs linking pages that share one sidebar entry. Tabs follow the same
 * admin × user sidebar module rules as the sidebar and the row is hidden
 * when fewer than two pages remain.
 */
export function SectionNavTabs(props: SectionNavTabsProps) {
  const navigate = useNavigate()
  const navGroups = useMemo<NavGroup[]>(
    () => [
      {
        title: 'Sections',
        items: props.tabs.map((tab) => ({ title: tab.value, url: tab.url })),
      },
    ],
    [props.tabs]
  )
  const filteredGroups = useSidebarConfig(navGroups)
  const visibleValues = new Set(
    (filteredGroups[0]?.items ?? []).map((item) => item.title)
  )
  const visibleTabs = props.tabs.filter((tab) => visibleValues.has(tab.value))

  if (visibleTabs.length < 2) return null

  return (
    <Tabs
      value={props.value}
      onValueChange={(value) => {
        const tab = visibleTabs.find((item) => item.value === value)
        if (tab) void navigate({ to: tab.url })
      }}
      className={cn('max-w-full', props.className)}
    >
      <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
        {visibleTabs.map((tab) => (
          <TabsTrigger key={tab.value} value={tab.value}>
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
