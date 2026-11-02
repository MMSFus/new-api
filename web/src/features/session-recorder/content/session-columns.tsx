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

import { DrillCell } from '@/features/simulated-cache/drill-cell'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { SessionSummary } from '../api'
import { formatDecimalBytes, formatUnixTime } from '../lib'

/** Session list columns; the session id opens its record timeline. */
export function useSessionColumns(
  onOpen: (session: SessionSummary) => void
): ColumnDef<SessionSummary>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  return useMemo(
    () => [
      {
        id: 'session_id',
        header: t('Session ID'),
        size: 220,
        cell: ({ row }) => (
          <span className='font-mono'>
            <DrillCell
              label={row.original.session_id}
              title={row.original.session_id}
              onClick={() => onOpen(row.original)}
            />
          </span>
        ),
      },
      {
        id: 'last_at',
        header: t('Last activity'),
        size: 170,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {formatUnixTime(row.original.last_at)}
          </span>
        ),
      },
      {
        id: 'first_at',
        header: t('Started at'),
        size: 170,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {formatUnixTime(row.original.first_at)}
          </span>
        ),
      },
      {
        id: 'records',
        header: t('Records'),
        size: 80,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {formatNumber(row.original.records, locale)}
          </span>
        ),
      },
      {
        id: 'models',
        header: t('Models'),
        size: 200,
        cell: ({ row }) => (
          <span className='line-clamp-2 break-all'>
            {(row.original.models ?? []).join(', ') || '-'}
          </span>
        ),
      },
      {
        id: 'user_ids',
        header: t('User ID'),
        size: 100,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {(row.original.user_ids ?? []).join(', ') || '-'}
          </span>
        ),
      },
      {
        id: 'raw_bytes',
        header: t('Raw size'),
        size: 100,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {formatDecimalBytes(row.original.raw_bytes, locale)}
          </span>
        ),
      },
    ],
    [t, locale, onOpen]
  )
}
