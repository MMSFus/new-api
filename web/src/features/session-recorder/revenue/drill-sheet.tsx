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
import { Filter, MessagesSquare } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { MetricGrid, type MetricGridItem } from '@/components/metric-grid'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import dayjs from '@/lib/dayjs'

import type { RevenueByModel, RevenueSummary } from '../api'
import type { RevenueFormat } from '../use-revenue-format'
import { useModelColumns, type RuleRow } from './breakdown-columns'

export type RevenueDrillTarget =
  | { kind: 'rule'; row: RuleRow }
  | { kind: 'model'; row: RevenueByModel }

export function RevenueDrillSheet(props: {
  target: RevenueDrillTarget | null
  summary: RevenueSummary | undefined
  range: { start: number; end: number }
  format: RevenueFormat
  ruleNames: Map<string, string>
  /** Shows "View sessions" for a model; needs content access. */
  canViewContent: boolean
  onOpenChange: (open: boolean) => void
  onDrill: (target: RevenueDrillTarget) => void
  onFilterModel: (model: string) => void
  onViewSessions: (model: string) => void
}) {
  const { t } = useTranslation()
  const target = props.target
  const format = props.format
  const modelColumns = useModelColumns({
    format,
    ruleNames: props.ruleNames,
    onDrill: (row) => props.onDrill({ kind: 'model', row }),
    onFilter: (row) => props.onFilterModel(row.model_name),
  })

  let title = ''
  let metrics: MetricGridItem[] = []
  let body = null
  if (target?.kind === 'rule') {
    const row = target.row
    title = row.rule_name
    metrics = [
      {
        label: t('Price per TB'),
        value:
          row.price_per_tb === null ? '-' : format.revenue(row.price_per_tb),
      },
      { label: t('Records'), value: format.count(row.records) },
      {
        label: t('Compressed size'),
        value: format.terabytes(row.archive_bytes),
      },
      { label: t('Revenue'), value: format.revenue(row.revenue) },
    ]
    const models = (props.summary?.by_model ?? []).filter(
      (model) => model.rule_id === row.rule_id
    )
    body = (
      <section aria-label={t('Models')} className='space-y-2'>
        <h3 className='text-sm font-medium'>{t('Models')}</h3>
        <StaticDataTable
          data={models}
          getRowKey={(model) => model.model_name}
          empty={models.length === 0}
          emptyContent={t('No data available')}
          columns={modelColumns}
        />
      </section>
    )
  } else if (target?.kind === 'model') {
    const row = target.row
    title = row.model_name || '-'
    metrics = [
      {
        label: t('Matched rule'),
        value: row.rule_id
          ? (props.ruleNames.get(row.rule_id) ?? row.rule_id)
          : t('Unmatched'),
      },
      { label: t('Records'), value: format.count(row.records) },
      { label: t('Raw size'), value: format.bytes(row.raw_bytes) },
      {
        label: t('Compressed size'),
        value: format.terabytes(row.archive_bytes),
      },
      { label: t('Revenue'), value: format.revenue(row.revenue) },
    ]
    body = row.model_name ? (
      <div className='flex flex-wrap justify-end gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => props.onFilterModel(row.model_name)}
        >
          <Filter aria-hidden='true' />
          {t('Apply as filter')}
        </Button>
        {props.canViewContent && (
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => props.onViewSessions(row.model_name)}
          >
            <MessagesSquare aria-hidden='true' />
            {t('View sessions')}
          </Button>
        )}
      </div>
    ) : null
  }

  return (
    <Sheet open={target !== null} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
        <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
          <SheetTitle className='break-all'>{title}</SheetTitle>
          <SheetDescription className='tabular-nums'>
            {dayjs.unix(props.range.start).format('YYYY-MM-DD HH:mm')}
            {' ~ '}
            {dayjs.unix(props.range.end).format('YYYY-MM-DD HH:mm')}
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName('gap-4')}>
          <MetricGrid metrics={metrics} />
          {body}
        </div>
      </SheetContent>
    </Sheet>
  )
}
