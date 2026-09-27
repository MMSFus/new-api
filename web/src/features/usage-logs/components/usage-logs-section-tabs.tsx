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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionNavTabs } from '@/components/layout/components/section-nav-tabs'

type UsageLogsTabValue = 'common' | 'drawing' | 'task' | 'audit'

/**
 * Switches between the log pages folded into the Usage Logs sidebar entry.
 */
export function UsageLogsSectionTabs(props: { value: UsageLogsTabValue }) {
  const { t } = useTranslation()
  const tabs = useMemo(
    () => [
      {
        value: 'common',
        label: t('Common Logs'),
        url: '/usage-logs/common',
      },
      {
        value: 'drawing',
        label: t('Drawing Logs'),
        url: '/usage-logs/drawing',
      },
      { value: 'task', label: t('Task Logs'), url: '/usage-logs/task' },
      { value: 'audit', label: t('Audit Logs'), url: '/usage-logs/audit' },
    ],
    [t]
  )
  return <SectionNavTabs tabs={tabs} value={props.value} />
}
