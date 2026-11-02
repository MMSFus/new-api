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
import type { ColumnDef, Table } from '@tanstack/react-table'
import { Fragment, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, DataTableRow } from '@/components/data-table'
import { TableCell, TableRow } from '@/components/ui/table'
import { SimulatedCacheComparison } from '@/features/usage-logs/components/simulated-cache-comparison'

import type { SimulatedCacheRow } from './types'

export function SimulatedCacheLogsTable(props: {
  table: Table<SimulatedCacheRow>
  columns: ColumnDef<SimulatedCacheRow>[]
  isLoading: boolean
  isFetching: boolean
  isError: boolean
  toolbar?: ReactNode
}) {
  const { t } = useTranslation()
  const table = props.table
  return (
    <DataTablePage
      table={table}
      columns={props.columns}
      isLoading={props.isLoading}
      isFetching={props.isFetching}
      emptyTitle={
        props.isError
          ? t('Failed to load simulated cache logs')
          : t('No records')
      }
      hideMobile
      fixedHeight={false}
      paginationInFooter={false}
      applyHeaderSize
      toolbar={props.toolbar}
      renderRow={(row) => (
        <Fragment key={row.id}>
          {/* className varies with expansion so the memoized row re-renders
              its toggle; the row object itself stays stable. */}
          <DataTableRow
            row={row}
            className={row.getIsExpanded() ? 'bg-muted/30' : undefined}
            cellRenderColumns={table.options.columns}
          />
          {row.getIsExpanded() && row.original.audit && (
            <TableRow className='hover:bg-transparent'>
              <TableCell
                colSpan={row.getVisibleCells().length}
                className='bg-muted/30 whitespace-normal'
              >
                <SimulatedCacheComparison audit={row.original.audit} />
              </TableCell>
            </TableRow>
          )}
        </Fragment>
      )}
    />
  )
}
