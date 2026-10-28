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

import {
  StaticDataTable,
  staticDataTableClassNames,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'

import { formatCacheHitRatio } from '../lib/cache-hit-ratio'
import {
  formatQuotaDelta,
  getCacheHitRatio,
  type SimulatedCacheAudit,
  type SimulatedCacheSide,
} from '../lib/simulated-cache'
import { DetailRow } from './dialogs/log-detail-layout'

type ComparisonRow = {
  label: string
  original: string
  simulated: string
}

/** Row marker for logs whose upstream cache usage was simulated. */
export function SimulatedCacheBadge() {
  const { t } = useTranslation()
  return (
    <StatusBadge
      label={t('Simulated cache')}
      variant='violet'
      size='sm'
      copyable={false}
      className='w-fit'
    />
  )
}

/**
 * Original (upstream) vs simulated (client + billing) usage of one request
 * whose prompt cache was simulated. Shared by the usage-log details dialog and
 * the simulated cache audit table.
 */
export function SimulatedCacheComparison(props: {
  audit: SimulatedCacheAudit
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const audit = props.audit
  const tokens = (value: number) => formatNumber(value, locale)
  const ratio = (side: SimulatedCacheSide) =>
    formatCacheHitRatio(getCacheHitRatio(side), locale)
  const percent = (value: number | null) =>
    value === null ? '-' : `${formatNumber(value, locale)}%`
  const configuredRate =
    audit.hitRateMin !== null && audit.hitRateMax !== null
      ? `${percent(audit.hitRateMin)}–${percent(audit.hitRateMax)}`
      : percent(audit.hitRate)
  const quota = (value: number | null) =>
    value === null ? '-' : formatLogQuota(value)

  const rows: ComparisonRow[] = [
    {
      label: t('Input Tokens'),
      original: tokens(audit.original.promptTokens),
      simulated: tokens(audit.simulated.promptTokens),
    },
    {
      label: t('Cache Read'),
      original: tokens(audit.original.cacheReadTokens),
      simulated: tokens(audit.simulated.cacheReadTokens),
    },
    {
      label: t('Cache Write'),
      original: tokens(audit.original.cacheCreationTokens),
      simulated: tokens(audit.simulated.cacheCreationTokens),
    },
    {
      label: t('Cache Write (5m)'),
      original: tokens(audit.original.cacheCreation5mTokens),
      simulated: tokens(audit.simulated.cacheCreation5mTokens),
    },
    {
      label: t('Cache Write (1h)'),
      original: tokens(audit.original.cacheCreation1hTokens),
      simulated: tokens(audit.simulated.cacheCreation1hTokens),
    },
    {
      label: t('Output Tokens'),
      original: tokens(audit.original.completionTokens),
      simulated: tokens(audit.simulated.completionTokens),
    },
    {
      label: t('Cache hit ratio'),
      original: ratio(audit.original),
      simulated: ratio(audit.simulated),
    },
    {
      label: t('Cost'),
      original: quota(audit.original.quota),
      simulated: quota(audit.simulated.quota),
    },
  ]
  const columns: StaticDataTableColumn<ComparisonRow>[] = [
    {
      id: 'metric',
      header: t('Metric'),
      className: staticDataTableClassNames.compactHeaderCell,
      cellClassName: staticDataTableClassNames.compactMutedCell,
      cell: (row) => row.label,
    },
    {
      id: 'original',
      header: t('Original'),
      className: staticDataTableClassNames.compactHeaderCellRight,
      cellClassName: staticDataTableClassNames.compactNumericCell,
      cell: (row) => row.original,
    },
    {
      id: 'simulated',
      header: t('Simulated'),
      className: staticDataTableClassNames.compactHeaderCellRight,
      cellClassName: staticDataTableClassNames.compactNumericCell,
      cell: (row) => row.simulated,
    },
  ]
  const segments =
    audit.matchedSegments === null && audit.totalSegments === null
      ? '-'
      : `${audit.matchedSegments ?? '-'} / ${audit.totalSegments ?? '-'}`

  return (
    // Container query: side by side only when the host gives enough width
    // (the audit table expansion), stacked in the narrow details dialog.
    <div className='@container/simulated-cache min-w-0'>
      <div
        data-slot='simulated-cache-comparison'
        className='flex min-w-0 flex-col gap-3 @3xl/simulated-cache:flex-row @3xl/simulated-cache:items-start'
      >
        <div className='min-w-0 flex-1'>
          <StaticDataTable
            columns={columns}
            data={rows}
            getRowKey={(row) => row.label}
            headerRowClassName={staticDataTableClassNames.compactHeaderRow}
            tableClassName={staticDataTableClassNames.compactTable}
            className='bg-background'
          />
        </div>
        <div
          data-slot='simulated-cache-meta'
          className='bg-background min-w-0 space-y-1.5 rounded-md border p-3 @3xl/simulated-cache:w-80 @3xl/simulated-cache:shrink-0'
        >
          <DetailRow
            label={t('Cost difference')}
            value={formatQuotaDelta(audit.quotaDelta)}
            mono
          />
          <DetailRow
            label={t('Configured hit rate')}
            value={configuredRate}
            mono
          />
          {audit.targetRate !== null && (
            <DetailRow
              label={t('Target hit rate')}
              value={percent(audit.targetRate)}
              mono
            />
          )}
          <DetailRow label={t('Matched segments')} value={segments} mono />
          <DetailRow
            label={t('Client format')}
            value={audit.clientFormat || '-'}
            mono
          />
          <DetailRow
            label={t('Upstream schema')}
            value={audit.upstreamSchema || '-'}
            mono
          />
        </div>
      </div>
    </div>
  )
}
