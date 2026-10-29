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
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import {
  formatQuotaDelta,
  type SimulatedCacheAudit,
} from '@/features/usage-logs/lib/simulated-cache'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatLogQuota, formatNumber } from '@/lib/format'

export type SimulatedCacheRow = {
  log: UsageLog
  /** null if the row lacks a readable marker (should not happen per API). */
  audit: SimulatedCacheAudit | null
}

function quotaText(value: number | null | undefined): string {
  return value == null ? '-' : formatLogQuota(value)
}

export function useSimulatedCacheColumns(): ColumnDef<SimulatedCacheRow>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return useMemo(() => {
    const tokens = (value: number | undefined) =>
      value === undefined ? '-' : formatNumber(value, locale)
    const columns: ColumnDef<SimulatedCacheRow>[] = [
      {
        id: 'expand',
        header: () => <span className='sr-only'>{t('Details')}</span>,
        size: 40,
        cell: ({ row }) => {
          const expanded = row.getIsExpanded()
          return (
            <Button
              type='button'
              variant='ghost'
              size='icon-xs'
              aria-expanded={expanded}
              aria-label={expanded ? t('Hide details') : t('Show details')}
              disabled={!row.original.audit}
              onClick={() => row.toggleExpanded()}
            >
              {expanded ? (
                <ChevronDown aria-hidden='true' />
              ) : (
                <ChevronRight aria-hidden='true' />
              )}
            </Button>
          )
        },
      },
      {
        id: 'created_at',
        header: t('Time'),
        size: 170,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {dayjs
              .unix(row.original.log.created_at)
              .format('YYYY-MM-DD HH:mm:ss')}
          </span>
        ),
      },
      {
        id: 'username',
        header: t('Username'),
        size: 110,
        accessorFn: (entry) => entry.log.username,
      },
      {
        id: 'channel',
        header: t('Channel'),
        size: 140,
        accessorFn: (entry) =>
          [entry.log.channel_name, `#${entry.log.channel}`]
            .filter(Boolean)
            .join(' '),
      },
      {
        id: 'model_name',
        header: t('Model'),
        size: 180,
        accessorFn: (entry) => entry.log.model_name,
      },
      {
        id: 'cache_read',
        header: t('Cache Read'),
        size: 160,
        cell: ({ row }) => {
          const audit = row.original.audit
          return (
            <span className='font-mono tabular-nums'>
              {tokens(audit?.original.cacheReadTokens)} →{' '}
              {tokens(audit?.simulated.cacheReadTokens)}
            </span>
          )
        },
      },
      {
        id: 'quota',
        header: t('Cost'),
        size: 180,
        cell: ({ row }) => {
          const audit = row.original.audit
          return (
            <span className='font-mono tabular-nums'>
              {quotaText(audit?.original.quota)} →{' '}
              {quotaText(audit?.simulated.quota ?? row.original.log.quota)}
            </span>
          )
        },
      },
      {
        id: 'quota_delta',
        header: t('Cost difference'),
        size: 120,
        cell: ({ row }) => {
          const delta = row.original.audit?.quotaDelta ?? null
          return (
            <span className='font-mono tabular-nums'>
              {formatQuotaDelta(delta)}
            </span>
          )
        },
      },
    ]
    return columns
  }, [t, locale])
}
