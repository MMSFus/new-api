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
import type {
  ColumnDef,
  OnChangeFn,
  PaginationState,
  Table,
} from '@tanstack/react-table'

import { useDataTable } from '@/components/data-table'

/**
 * Table state of an admin report whose rows are paged by the server: no
 * selection, sorting or client filtering.
 */
export function useServerPagedTable<TData>(options: {
  columns: ColumnDef<TData>[]
  rows: TData[]
  getRowId: (row: TData) => string
  total: number
  pagination: PaginationState
  onPaginationChange: OnChangeFn<PaginationState>
  /** Paging is ignored while a page loads or the search is invalid. */
  locked: boolean
}): Table<TData> {
  const { table } = useDataTable({
    columns: options.columns,
    data: options.rows,
    getRowId: options.getRowId,
    totalCount: options.total,
    pagination: options.pagination,
    onPaginationChange: (updater) => {
      if (options.locked) return
      options.onPaginationChange(updater)
    },
    enableRowSelection: false,
    enableSorting: false,
    manualFiltering: true,
    manualPagination: true,
  })
  return table
}
