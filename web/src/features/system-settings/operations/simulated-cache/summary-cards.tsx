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
import { Activity, Coins, Database } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatCard } from '@/features/dashboard/components/ui/stat-card'
import { formatQuotaDelta } from '@/features/usage-logs/lib/simulated-cache'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'

import type { SimulatedCacheStat } from './api'

export function SimulatedCacheSummaryCards(props: {
  stat: SimulatedCacheStat | undefined
  loading: boolean
  error: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const stat = props.stat
  const cards = [
    {
      key: 'count',
      title: t('Simulated requests'),
      value: formatNumber(stat?.count ?? 0, locale),
      description: t('Consume logs with a simulated cache in the range'),
      icon: Activity,
      tone: 'accent-1' as const,
      details: [],
    },
    {
      key: 'quota',
      title: t('Simulated cost'),
      value: formatLogQuota(stat?.simulated_quota ?? 0),
      description: t('Charged with the simulated cache usage'),
      icon: Coins,
      tone: 'accent-2' as const,
      details: [
        {
          label: t('Original cost'),
          value: formatLogQuota(stat?.original_quota ?? 0),
        },
        {
          label: t('Cost difference'),
          value: formatQuotaDelta(stat?.quota_delta ?? 0),
        },
      ],
    },
    {
      key: 'cache',
      title: t('Simulated cache read'),
      value: formatNumber(stat?.simulated_cache_read_tokens ?? 0, locale),
      description: t('Tokens reported to clients as cache reads'),
      icon: Database,
      tone: 'accent-3' as const,
      details: [
        {
          label: t('Original cache read'),
          value: formatNumber(stat?.original_cache_read_tokens ?? 0, locale),
        },
        {
          label: t('Simulated cache write'),
          value: formatNumber(
            stat?.simulated_cache_creation_tokens ?? 0,
            locale
          ),
        },
      ],
    },
  ]

  return (
    <div className='grid gap-3 md:grid-cols-3'>
      {cards.map((card) => (
        <div key={card.key} className='bg-card rounded-xl border p-3'>
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
