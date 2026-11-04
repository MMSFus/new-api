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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import {
  REVENUE_MAX_RANGE_DAYS,
  revenueGranularity,
  type RevenueBreakdown,
} from '../lib'
import { useRevenueFormat } from '../use-revenue-format'
import {
  useRevenueRules,
  useRevenueSummary,
} from '../use-session-recorder-data'
import { RevenueBreakdownSection } from './breakdown'
import { RevenueDrillSheet, type RevenueDrillTarget } from './drill-sheet'
import { ruleRows } from './rule-rows'
import { RevenueRulesDialog } from './rules-dialog'
import { RevenueSummaryCards } from './summary-cards'
import { RevenueTrendChart } from './trend-chart'

export const REVENUE_MAX_RANGE_SECONDS = REVENUE_MAX_RANGE_DAYS * 24 * 3600

/**
 * Revenue from archived recordings: compressed bytes priced by the first
 * matching rule. KPIs, trend and breakdowns follow the page filters.
 */
export function RevenueTab(props: {
  range: { start: number; end: number }
  model: string | undefined
  /** False while the page range is invalid for this tab. */
  canQuery: boolean
  canWrite: boolean
  canViewContent: boolean
  breakdown: RevenueBreakdown
  onBreakdownChange: (breakdown: RevenueBreakdown) => void
  onFilter: (patch: {
    model_name?: string
    start_timestamp?: number
    end_timestamp?: number
  }) => void
  onViewSessions: (model: string) => void
}) {
  const { t } = useTranslation()
  const [target, setTarget] = useState<RevenueDrillTarget | null>(null)
  const range = props.range
  const params = {
    start_timestamp: range.start,
    end_timestamp: range.end,
    ...(props.model ? { model: props.model } : {}),
    granularity: revenueGranularity(range),
  }
  const summaryQuery = useRevenueSummary(params, props.canQuery)
  const rulesQuery = useRevenueRules(true)
  const format = useRevenueFormat(rulesQuery.data?.currency_label)
  const siteFormat = useRevenueFormat('')
  const summary = props.canQuery ? summaryQuery.data : undefined
  const loading = props.canQuery && summaryQuery.isPending
  const ruleNames = new Map<string, string>()
  for (const rule of rulesQuery.data?.rules ?? []) {
    ruleNames.set(rule.id, rule.name)
  }
  for (const row of ruleRows(summary, '')) {
    if (row.rule_id) ruleNames.set(row.rule_id, row.rule_name)
  }
  const totals = summary?.totals
  const onlyUnarchived =
    totals !== undefined &&
    totals.records === 0 &&
    totals.unarchived_records > 0

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-start justify-between gap-2'>
        <p className='text-muted-foreground max-w-3xl text-sm'>
          {t(
            'Revenue counts archived records only, by their compressed size uploaded to R2 (1 TB = 10^12 bytes). Rules apply as currently configured.'
          )}
        </p>
        {props.canWrite && <RevenueRulesDialog siteSymbol={siteFormat.unit} />}
      </div>
      {props.canQuery && summaryQuery.isError && (
        <Alert variant='destructive'>
          <AlertDescription className='flex items-center justify-between gap-2'>
            <span>
              {summaryQuery.error.message ||
                t('Failed to load revenue summary')}
            </span>
            <Button
              size='sm'
              variant='outline'
              onClick={() => void summaryQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {onlyUnarchived && (
        <Alert>
          <AlertDescription>
            {t(
              'No records in this range are archived yet. Revenue appears after their batches are uploaded.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <RevenueSummaryCards
        totals={totals}
        format={format}
        loading={loading}
        error={!props.canQuery || summaryQuery.isError}
      />
      <RevenueTrendChart
        trend={summary?.trend ?? []}
        range={range}
        format={format}
        loading={loading}
        enabled={props.canQuery}
        onSelectPoint={(point) =>
          props.onFilter({
            start_timestamp: point.start,
            end_timestamp: point.end,
          })
        }
      />
      <RevenueBreakdownSection
        summary={summary}
        format={format}
        ruleNames={ruleNames}
        loading={loading}
        breakdown={props.breakdown}
        onBreakdownChange={props.onBreakdownChange}
        onDrillRule={(row) => setTarget({ kind: 'rule', row })}
        onDrillModel={(row) => setTarget({ kind: 'model', row })}
        onFilterModel={(row) => props.onFilter({ model_name: row.model_name })}
      />
      <RevenueDrillSheet
        target={target}
        summary={summary}
        range={range}
        format={format}
        ruleNames={ruleNames}
        canViewContent={props.canViewContent}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        onDrill={setTarget}
        onFilterModel={(model) => {
          setTarget(null)
          props.onFilter({ model_name: model })
        }}
        onViewSessions={(model) => {
          setTarget(null)
          props.onViewSessions(model)
        }}
      />
    </div>
  )
}
