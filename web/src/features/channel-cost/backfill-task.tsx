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

import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { CostBackfillTask } from './api'
import { formatBackfillRange } from './lib'

const taskTitles = {
  pending: 'Historical cost backfill submitted',
  running: 'Backfilling historical costs',
  succeeded: 'Historical cost backfill complete',
  failed: 'Historical cost backfill incomplete',
} as const
const taskStatuses = {
  pending: 'Pending',
  running: 'Running',
  succeeded: 'Completed',
  failed: 'Failed',
} as const

/** Range, filters, progress and skip reasons of the latest backfill task. */
export function CostBackfillTaskStatus(props: { task: CostBackfillTask }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const task = props.task
  const active = task.status === 'pending' || task.status === 'running'
  const scope = task.payload?.filter
  const state = task.result ?? task.state
  const taskFilters = scope
    ? [
        scope.Channel ? `${t('Channel')} #${scope.Channel}` : '',
        scope.ModelName ? `${t('Model')}: ${scope.ModelName}` : '',
        scope.Username ? `${t('Username')}: ${scope.Username}` : '',
        scope.Group ? `${t('Group')}: ${scope.Group}` : '',
      ]
        .filter(Boolean)
        .join(' · ')
    : ''

  return (
    <section aria-live='polite' className='space-y-4 rounded-xl border p-5'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h2 className='text-sm font-medium'>{t(taskTitles[task.status])}</h2>
          {scope && (
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Task range')}:{' '}
              {formatBackfillRange(scope.StartTimestamp, scope.EndTimestamp)}
            </p>
          )}
          {taskFilters && (
            <p className='text-muted-foreground mt-1 text-xs break-all'>
              {taskFilters}
            </p>
          )}
        </div>
        <Badge variant='secondary'>{t(taskStatuses[task.status])}</Badge>
      </div>
      {state && (
        <>
          <div className='flex flex-wrap gap-x-5 gap-y-2 text-sm'>
            <span>
              {t('Scanned')}: {formatNumber(state.processed, locale)}
            </span>
            <span>
              {t('Backfilled')}: {formatNumber(state.updated, locale)}
            </span>
            <span>
              {t('Skipped')}: {formatNumber(state.skipped, locale)}
            </span>
          </div>
          {active && (
            <Progress aria-label={t('Progress')} value={state.progress ?? 0} />
          )}
        </>
      )}
      {active && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'The task runs in batches in the background and continues after you close this page.'
          )}
        </p>
      )}
      {task.status === 'succeeded' && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Backfilled costs are estimates, marked as historical backfill in the details. The report has been refreshed.'
          )}
        </p>
      )}
      {task.status === 'failed' && (
        <>
          <p className='text-destructive text-sm'>
            {t('Historical cost backfill failed. Please retry later.')}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Completed estimates are preserved. Retry processes records that are still uncosted.'
            )}
          </p>
        </>
      )}
      {!!state?.skipped && (
        <ul className='text-muted-foreground space-y-1 text-sm'>
          {Object.entries(state.reasons ?? {}).map(([reason, count]) => (
            <li key={reason}>
              {t(reason)}: {formatNumber(count, locale)}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
