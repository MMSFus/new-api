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
import { Archive, Coins, FileClock, Rows3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatCard } from '@/features/dashboard/components/ui/stat-card'

import type { RevenueTotals } from '../api'
import type { RevenueFormat } from '../use-revenue-format'

export function RevenueSummaryCards(props: {
  totals: RevenueTotals | undefined
  format: RevenueFormat
  loading: boolean
  error: boolean
}) {
  const { t } = useTranslation()
  const totals = props.totals
  const format = props.format
  const unarchived = totals?.unarchived_records ?? 0
  const cards = [
    {
      key: 'archive',
      title: t('Compressed data'),
      value: format.terabytes(totals?.archive_bytes ?? 0),
      description: t('Compressed size uploaded to R2; 1 TB = 10^12 bytes'),
      icon: Archive,
      tone: 'accent-1' as const,
      details: [
        {
          label: t('Raw size'),
          value: format.bytes(totals?.raw_bytes ?? 0),
        },
      ],
    },
    {
      key: 'revenue',
      title: t('Revenue'),
      value: format.revenue(totals?.revenue ?? 0),
      description: t(
        'Compressed TB times the price of the first matching rule'
      ),
      icon: Coins,
      tone: 'accent-2' as const,
      details: [{ label: t('Unit'), value: format.unit }],
    },
    {
      key: 'records',
      title: t('Records'),
      value: format.count(totals?.records ?? 0),
      description: t('Archived records in the range'),
      icon: Rows3,
      tone: 'accent-3' as const,
    },
    {
      key: 'unarchived',
      title: t('Unarchived records'),
      value: format.count(unarchived),
      description: t('Not uploaded yet; excluded from revenue'),
      icon: FileClock,
      tone: 'accent-1' as const,
      details: [
        {
          label: t('Raw size'),
          value: format.bytes(totals?.unarchived_raw_bytes ?? 0),
          tone: unarchived > 0 ? ('warning' as const) : ('muted' as const),
        },
      ],
    },
  ]

  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
      {cards.map((card) => (
        <div
          key={card.key}
          role='group'
          aria-label={card.title}
          className='bg-card rounded-xl border p-3'
        >
          <StatCard
            title={card.title}
            value={card.value}
            description={card.description}
            icon={card.icon}
            tone={card.tone}
            details={card.details}
            loading={props.loading}
            error={props.error}
          />
        </div>
      ))}
    </div>
  )
}
