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
import { useTranslation } from 'react-i18next'

import { formatQuotaDelta } from '@/features/usage-logs/lib/simulated-cache'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ChannelCostAmount } from './api'
import { formatCostQuota, formatPercent, profitMargin, profitTone } from './lib'

/** Totals of a drilled slice as a compact metric grid. */
export function ChannelCostScopeSummary(props: {
  amount: ChannelCostAmount
  upstreamRatio?: number | null
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const amount = props.amount
  const metrics: { label: string; value: string; className?: string }[] = [
    { label: t('Requests'), value: formatNumber(amount.requests, locale) },
    { label: t('Refunds'), value: formatNumber(amount.refunds, locale) },
    {
      label: t('Uncosted requests'),
      value: formatNumber(amount.uncosted_requests, locale),
      className: amount.uncosted_requests > 0 ? 'text-warning' : undefined,
    },
    { label: t('Revenue'), value: formatLogQuota(amount.revenue) },
    {
      label: t('Costed revenue'),
      value: formatLogQuota(amount.costed_revenue),
    },
    { label: t('Upstream cost'), value: formatCostQuota(amount.cost) },
    {
      label: t('Gross profit'),
      value: formatCostQuota(amount.profit),
      className: profitTone(amount.profit),
    },
    {
      label: t('Margin'),
      value: formatPercent(profitMargin(amount), locale),
    },
    {
      label: t('Input / Output'),
      value: `${formatNumber(amount.prompt_tokens, locale)} / ${formatNumber(amount.completion_tokens, locale)}`,
    },
    {
      label: t('Simulated cache difference'),
      value: formatQuotaDelta(amount.simulated_cache_delta),
    },
  ]
  if (props.upstreamRatio !== undefined) {
    metrics.push({
      label: t('Upstream cost ratio'),
      value:
        props.upstreamRatio == null
          ? t('Not set')
          : String(props.upstreamRatio),
      className: props.upstreamRatio == null ? 'text-warning' : undefined,
    })
  }

  return (
    <dl className='grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4'>
      {metrics.map((metric) => (
        <div
          key={metric.label}
          className='bg-muted/40 min-w-0 rounded-lg px-3 py-2'
        >
          <dt className='text-muted-foreground truncate text-xs'>
            {metric.label}
          </dt>
          <dd
            className={cn(
              'mt-1 truncate font-mono text-sm font-semibold tabular-nums',
              metric.className
            )}
            title={metric.value}
          >
            {metric.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}
