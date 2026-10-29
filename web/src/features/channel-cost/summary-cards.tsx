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
import { Activity, Coins, PiggyBank, Receipt } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatCard } from '@/features/dashboard/components/ui/stat-card'
import { formatQuotaDelta } from '@/features/usage-logs/lib/simulated-cache'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'

import type { ChannelCostAmount } from './api'
import { formatCostQuota, formatPercent, profitMargin } from './lib'

export function ChannelCostSummaryCards(props: {
  total: ChannelCostAmount | undefined
  loading: boolean
  error: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const total = props.total
  const cards = [
    {
      key: 'revenue',
      title: t('Revenue'),
      value: formatLogQuota(total?.revenue ?? 0),
      description: t('Charged to users, net of refunds'),
      icon: Coins,
      tone: 'accent-1' as const,
      details: [
        {
          label: t('Costed revenue'),
          value: formatLogQuota(total?.costed_revenue ?? 0),
        },
        {
          label: t('Simulated cache difference'),
          value: formatQuotaDelta(total?.simulated_cache_delta ?? 0),
        },
      ],
    },
    {
      key: 'cost',
      title: t('Upstream cost'),
      value: formatCostQuota(total?.cost ?? 0),
      description: t('Official price times the channel upstream cost ratio'),
      icon: Receipt,
      tone: 'accent-2' as const,
      details: [
        {
          label: t('Uncosted requests'),
          value: formatNumber(total?.uncosted_requests ?? 0, locale),
          tone:
            (total?.uncosted_requests ?? 0) > 0
              ? ('warning' as const)
              : ('muted' as const),
        },
      ],
    },
    {
      key: 'profit',
      title: t('Gross profit'),
      value: formatCostQuota(total?.profit ?? 0),
      description: t('Costed revenue minus upstream cost'),
      icon: PiggyBank,
      tone: 'accent-3' as const,
      details: [
        {
          label: t('Margin'),
          value: formatPercent(total ? profitMargin(total) : null, locale),
          tone:
            (total?.profit ?? 0) < 0
              ? ('destructive' as const)
              : ('success' as const),
        },
      ],
    },
    {
      key: 'requests',
      title: t('Requests'),
      value: formatNumber(total?.requests ?? 0, locale),
      description: t('Consume logs in the range'),
      icon: Activity,
      tone: 'accent-1' as const,
      details: [
        {
          label: t('Refunds'),
          value: formatNumber(total?.refunds ?? 0, locale),
        },
        {
          label: t('Tokens'),
          value: formatNumber(
            (total?.prompt_tokens ?? 0) + (total?.completion_tokens ?? 0),
            locale
          ),
        },
      ],
    },
  ]

  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
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
