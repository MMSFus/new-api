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

import { StaticDataTable } from '@/components/data-table'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { RevenueByModel, RevenueSummary } from '../api'
import type { RevenueBreakdown as Breakdown } from '../lib'
import type { RevenueFormat } from '../use-revenue-format'
import {
  useModelColumns,
  useRuleColumns,
  type RuleRow,
} from './breakdown-columns'
import { ruleRows } from './rule-rows'

export function RevenueBreakdownSection(props: {
  summary: RevenueSummary | undefined
  format: RevenueFormat
  /** Rule id to name, for the model table. */
  ruleNames: Map<string, string>
  loading: boolean
  breakdown: Breakdown
  onBreakdownChange: (breakdown: Breakdown) => void
  onDrillRule: (row: RuleRow) => void
  onDrillModel: (row: RevenueByModel) => void
  onFilterModel: (row: RevenueByModel) => void
}) {
  const { t } = useTranslation()
  const rules = ruleRows(props.summary, t('Unmatched'))
  const models = props.summary?.by_model ?? []
  const ruleColumns = useRuleColumns({
    format: props.format,
    onDrill: props.onDrillRule,
  })
  const modelColumns = useModelColumns({
    format: props.format,
    ruleNames: props.ruleNames,
    onDrill: props.onDrillModel,
    onFilter: props.onFilterModel,
  })

  let table = (
    <StaticDataTable
      className='rounded-none border-0'
      data={rules}
      getRowKey={(row) => row.rule_id ?? 'unmatched'}
      empty={rules.length === 0}
      emptyContent={t('No data available')}
      columns={ruleColumns}
    />
  )
  if (props.loading) {
    table = (
      <div className='p-4 sm:p-5'>
        <Skeleton className='h-48 w-full' />
      </div>
    )
  } else if (props.breakdown === 'model') {
    table = (
      <StaticDataTable
        className='rounded-none border-0'
        data={models}
        getRowKey={(row) => row.model_name}
        empty={models.length === 0}
        emptyContent={t('No data available')}
        columns={modelColumns}
      />
    )
  }

  return (
    <section
      aria-label={t('Breakdown')}
      className='bg-card overflow-hidden rounded-2xl border shadow-xs'
    >
      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 sm:px-5'>
        <div className='flex min-w-0 flex-col gap-1'>
          <h2 className='text-sm font-semibold'>{t('Breakdown')}</h2>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Each record counts toward the first enabled rule that matches its model.'
            )}
          </p>
        </div>
        <Tabs
          value={props.breakdown}
          onValueChange={(value) => props.onBreakdownChange(value as Breakdown)}
        >
          <TabsList aria-label={t('Breakdown dimension')}>
            <TabsTrigger value='rule'>{t('By rule')}</TabsTrigger>
            <TabsTrigger value='model'>{t('By model')}</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      <div className='min-w-0'>{table}</div>
    </section>
  )
}
