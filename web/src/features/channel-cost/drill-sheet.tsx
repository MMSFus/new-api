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
import type { PaginationState } from '@tanstack/react-table'
import { ArrowLeft, Filter } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import dayjs from '@/lib/dayjs'

import type { ChannelCostFilters, ChannelCostLog } from './api'
import { useChannelCostColumns } from './columns'
import type { ChannelCostScope } from './lib'
import { ChannelCostLogDetail } from './log-detail'
import { ChannelCostLogsTable } from './logs-table'
import { ChannelCostScopeSummary } from './scope-summary'
import { useChannelCostLogs } from './use-channel-cost-data'
import { useChannelCostTable } from './use-channel-cost-table'

export type ChannelCostDrillTarget =
  | { kind: 'scope'; scope: ChannelCostScope }
  | { kind: 'log'; log: ChannelCostLog }

export function ChannelCostDrillSheet(props: {
  target: ChannelCostDrillTarget | null
  filters: ChannelCostFilters
  onOpenChange: (open: boolean) => void
  onApplyFilter: (patch: Partial<ChannelCostFilters>) => void
}) {
  const target = props.target
  let key = ''
  if (target?.kind === 'scope') key = `scope:${target.scope.title}`
  else if (target?.kind === 'log') key = `log:${target.log.id}`
  return (
    <Sheet open={target !== null} onOpenChange={props.onOpenChange}>
      <SheetContent
        className={sideDrawerContentClassName('sm:max-w-3xl xl:max-w-5xl')}
      >
        {target?.kind === 'scope' && (
          <ScopeContent
            key={key}
            scope={target.scope}
            filters={props.filters}
            onApplyFilter={props.onApplyFilter}
          />
        )}
        {target?.kind === 'log' && <LogContent key={key} log={target.log} />}
      </SheetContent>
    </Sheet>
  )
}

function LogContent(props: { log: ChannelCostLog; onBack?: () => void }) {
  const { t } = useTranslation()
  return (
    <>
      <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
        <SheetTitle>{t('Cost details')}</SheetTitle>
        <SheetDescription className='font-mono tabular-nums'>
          {props.log.model_name} ·{' '}
          {dayjs.unix(props.log.created_at).format('YYYY-MM-DD HH:mm:ss')}
        </SheetDescription>
      </SheetHeader>
      <div className={sideDrawerFormClassName('gap-4')}>
        {props.onBack && (
          <div>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={props.onBack}
            >
              <ArrowLeft aria-hidden='true' />
              {t('Back')}
            </Button>
          </div>
        )}
        <ChannelCostLogDetail log={props.log} />
      </div>
    </>
  )
}

function ScopeContent(props: {
  scope: ChannelCostScope
  filters: ChannelCostFilters
  onApplyFilter: (patch: Partial<ChannelCostFilters>) => void
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<ChannelCostLog | null>(null)
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  })
  const filters = { ...props.filters, ...props.scope.patch }
  const logs = useChannelCostLogs(filters, pagination, true)
  const columns = useChannelCostColumns(setSelected)
  const table = useChannelCostTable({
    columns,
    logs: logs.logs,
    total: logs.total,
    pagination,
    onPaginationChange: setPagination,
    locked: logs.query.isFetching,
  })
  const hasPatch = Object.keys(props.scope.patch).length > 0

  if (selected) {
    return <LogContent log={selected} onBack={() => setSelected(null)} />
  }
  return (
    <>
      <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
        <SheetTitle>{props.scope.title}</SheetTitle>
        <SheetDescription className='tabular-nums'>
          {dayjs.unix(filters.start_timestamp ?? 0).format('YYYY-MM-DD HH:mm')}
          {' ~ '}
          {dayjs.unix(filters.end_timestamp ?? 0).format('YYYY-MM-DD HH:mm')}
        </SheetDescription>
      </SheetHeader>
      <div className={sideDrawerFormClassName('gap-4')}>
        <ChannelCostScopeSummary
          amount={props.scope.amount}
          upstreamRatio={props.scope.upstreamRatio}
        />
        {hasPatch && (
          <div className='flex justify-end'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => props.onApplyFilter(props.scope.patch)}
            >
              <Filter aria-hidden='true' />
              {t('Apply as filter')}
            </Button>
          </div>
        )}
        {logs.query.isError ? (
          <ErrorState
            title={t('Failed to load cost logs')}
            description={logs.query.error.message}
            onRetry={() => void logs.query.refetch()}
          />
        ) : (
          <ChannelCostLogsTable
            table={table}
            columns={columns}
            isLoading={logs.query.isPending}
            isFetching={logs.query.isFetching}
            isError={false}
          />
        )}
      </div>
    </>
  )
}
