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

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { SessionRecord } from '../api'
import { formatDecimalBytes, formatUnixTime } from '../lib'

/** Records of a session in request order; each opens its content. */
export function RecordTimeline(props: {
  records: SessionRecord[]
  onOpen: (record: SessionRecord) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  return (
    <ol aria-label={t('Record timeline')} className='relative space-y-2'>
      {props.records.map((record) => {
        const failed = record.status_code >= 400
        return (
          <li key={record.record_id} className='relative ps-5'>
            <span
              aria-hidden='true'
              className='bg-border absolute start-1.5 top-0 bottom-[-0.5rem] w-px'
            />
            <span
              aria-hidden='true'
              className='bg-primary absolute start-0.5 top-4 size-2.5 rounded-full'
            />
            <Button
              type='button'
              variant='outline'
              className='h-auto w-full flex-col items-start gap-1 px-3 py-2 text-start font-normal whitespace-normal'
              aria-label={t('Open record {{id}}', { id: record.record_id })}
              onClick={() => props.onOpen(record)}
            >
              <span className='flex w-full flex-wrap items-center gap-2'>
                <span className='font-mono text-xs tabular-nums'>
                  {formatUnixTime(record.created_at)}
                </span>
                <span className='min-w-0 truncate font-medium'>
                  {record.model_name || '-'}
                </span>
                <StatusBadge
                  variant={failed ? 'danger' : 'success'}
                  copyable={false}
                >
                  {String(record.status_code)}
                </StatusBadge>
                {record.stream && (
                  <StatusBadge variant='info' copyable={false}>
                    {t('Stream')}
                  </StatusBadge>
                )}
                {record.archived_at === 0 && (
                  <StatusBadge variant='warning' copyable={false}>
                    {t('Not archived')}
                  </StatusBadge>
                )}
              </span>
              <span className='text-muted-foreground flex w-full flex-wrap gap-x-4 font-mono text-xs tabular-nums'>
                <span>{record.path || '-'}</span>
                <span>{formatNumber(record.duration_ms, locale)} ms</span>
                <span>{formatDecimalBytes(record.raw_bytes, locale)}</span>
                <span>
                  {t('User ID')}: {record.user_id}
                </span>
              </span>
            </Button>
          </li>
        )
      })}
    </ol>
  )
}
