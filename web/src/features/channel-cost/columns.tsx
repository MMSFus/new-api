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
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { formatQuotaDelta } from '@/features/usage-logs/lib/simulated-cache'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { LOG_TYPE_REFUND, type ChannelCostLog } from './api'
import { formatCostQuota, profitTone } from './lib'

export function useChannelCostColumns(): ColumnDef<ChannelCostLog>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  return useMemo(
    () => [
      {
        id: 'created_at',
        header: t('Time'),
        size: 170,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {dayjs.unix(row.original.created_at).format('YYYY-MM-DD HH:mm:ss')}
          </span>
        ),
      },
      {
        id: 'type',
        header: t('Type'),
        size: 80,
        cell: ({ row }) =>
          row.original.type === LOG_TYPE_REFUND ? (
            <StatusBadge variant='warning' copyable={false}>
              {t('Refund')}
            </StatusBadge>
          ) : (
            <StatusBadge variant='neutral' copyable={false}>
              {t('Consume')}
            </StatusBadge>
          ),
      },
      {
        id: 'username',
        header: t('Username'),
        size: 110,
        accessorFn: (log) => log.username,
      },
      {
        id: 'channel',
        header: t('Channel'),
        size: 140,
        accessorFn: (log) =>
          [log.channel_name, `#${log.channel_id}`].filter(Boolean).join(' '),
      },
      {
        id: 'model_name',
        header: t('Model'),
        size: 180,
        accessorFn: (log) => log.model_name,
      },
      {
        id: 'group',
        header: t('Group'),
        size: 100,
        accessorFn: (log) => log.group || '-',
      },
      {
        id: 'tokens',
        header: t('Input / Output'),
        size: 130,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {formatNumber(row.original.prompt_tokens, locale)} /{' '}
            {formatNumber(row.original.completion_tokens, locale)}
          </span>
        ),
      },
      {
        id: 'ratios',
        header: t('Group / upstream ratio'),
        size: 130,
        cell: ({ row }) => {
          const cost = row.original.cost
          if (!cost) {
            return <span className='text-warning text-xs'>{t('Uncosted')}</span>
          }
          return (
            <span className='font-mono tabular-nums'>
              {cost.group_ratio} / {cost.upstream_ratio}
            </span>
          )
        },
      },
      {
        id: 'revenue',
        header: t('Revenue'),
        size: 110,
        cell: ({ row }) => {
          const sign = row.original.type === LOG_TYPE_REFUND ? -1 : 1
          return (
            <span className='font-mono tabular-nums'>
              {formatLogQuota(sign * row.original.quota)}
            </span>
          )
        },
      },
      {
        id: 'cost',
        header: t('Upstream cost'),
        size: 110,
        cell: ({ row }) => {
          const cost = row.original.cost
          if (!cost) return '-'
          const sign = row.original.type === LOG_TYPE_REFUND ? -1 : 1
          return (
            <span
              className='font-mono tabular-nums'
              title={t('Official price {{base}} × {{ratio}}', {
                base: formatCostQuota(cost.base_quota),
                ratio: cost.upstream_ratio,
              })}
            >
              {formatCostQuota(sign * cost.cost_quota)}
            </span>
          )
        },
      },
      {
        id: 'cost_source',
        header: t('Cost source'),
        size: 150,
        cell: ({ row }) => {
          if (!row.original.cost) return t('Uncosted')
          if (row.original.cost.source === 'historical_backfill') {
            return (
              <StatusBadge variant='warning' copyable={false}>
                {t('Historical backfill · Estimate')}
              </StatusBadge>
            )
          }
          return t('Recorded at request time')
        },
      },
      {
        id: 'profit',
        header: t('Gross profit'),
        size: 110,
        cell: ({ row }) => {
          const cost = row.original.cost
          if (!cost) return '-'
          const sign = row.original.type === LOG_TYPE_REFUND ? -1 : 1
          const profit = sign * (row.original.quota - cost.cost_quota)
          return (
            <span className={cn('font-mono tabular-nums', profitTone(profit))}>
              {formatCostQuota(profit)}
            </span>
          )
        },
      },
      {
        id: 'simulated_cache',
        header: t('Simulated cache difference'),
        size: 120,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {formatQuotaDelta(row.original.simulated_cache_delta)}
          </span>
        ),
      },
    ],
    [t, locale]
  )
}
