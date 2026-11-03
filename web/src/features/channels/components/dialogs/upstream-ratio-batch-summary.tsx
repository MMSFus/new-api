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

import type { UpstreamRatioBatchSummary } from '../../api'
import { UPSTREAM_RATIO_STATUS_LABELS } from '../../lib'

const SUMMARY_LABELS: Record<keyof UpstreamRatioBatchSummary, string> = {
  enabled: 'Auto sync enabled',
  ineligible: 'Not applicable',
  ...UPSTREAM_RATIO_STATUS_LABELS,
}

const SUMMARY_ORDER: (keyof UpstreamRatioBatchSummary)[] = [
  'enabled',
  'success',
  'unchanged',
  'out_of_range',
  'unsupported',
  'ineligible',
  'failed',
]

type UpstreamRatioBatchSummaryListProps = {
  summary: UpstreamRatioBatchSummary
}

/** Lists the non-zero counts of a batch ratio operation. */
export function UpstreamRatioBatchSummaryList(
  props: UpstreamRatioBatchSummaryListProps
) {
  const { t } = useTranslation()
  const entries = SUMMARY_ORDER.filter(
    (key) => (props.summary[key] ?? 0) > 0
  )
  if (entries.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>{t('Nothing changed')}</p>
    )
  }
  return (
    <dl
      aria-label={t('Result summary')}
      className='grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-3'
    >
      {entries.map((key) => (
        <div key={key} className='flex justify-between gap-2'>
          <dt className='text-muted-foreground'>{t(SUMMARY_LABELS[key])}</dt>
          <dd className='font-mono tabular-nums'>{props.summary[key]}</dd>
        </div>
      ))}
    </dl>
  )
}
