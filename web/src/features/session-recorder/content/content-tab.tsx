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
import type { OnChangeFn, PaginationState } from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage } from '@/components/data-table'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useServerPagedTable } from '@/hooks/use-server-paged-table'

import type { SessionFilters, SessionSummary } from '../api'
import { useSessions } from '../use-session-recorder-data'
import { useSessionColumns } from './session-columns'
import { SessionSheet } from './session-sheet'

/**
 * Conversation review: sessions matching the page filters, newest activity
 * first. A session opens its record timeline, a record its request and
 * response. Every content read is audited by the server.
 */
export function ContentTab(props: {
  filters: SessionFilters
  canQuery: boolean
  pagination: PaginationState
  onPaginationChange: OnChangeFn<PaginationState>
}) {
  const { t } = useTranslation()
  const [session, setSession] = useState<SessionSummary | null>(null)
  const sessions = useSessions(props.filters, props.pagination, props.canQuery)
  const columns = useSessionColumns(setSession)
  const table = useServerPagedTable({
    columns,
    rows: sessions.sessions,
    getRowId: (row) => row.session_id,
    total: sessions.total,
    pagination: props.pagination,
    onPaginationChange: props.onPaginationChange,
    locked: sessions.query.isFetching || !props.canQuery,
  })
  const query = sessions.query

  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Recorded sessions, newest activity first. Viewing record content is written to the audit log.'
        )}
      </p>
      {props.canQuery && query.isError && (
        <Alert variant='destructive'>
          <AlertDescription className='flex items-center justify-between gap-2'>
            <span>{query.error.message || t('Failed to load sessions')}</span>
            <Button
              size='sm'
              variant='outline'
              onClick={() => void query.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={props.canQuery && query.isPending}
        isFetching={query.isFetching}
        emptyTitle={
          query.isError ? t('Failed to load sessions') : t('No sessions')
        }
        hideMobile
        fixedHeight={false}
        paginationInFooter={false}
        applyHeaderSize
      />
      <SessionSheet
        session={session}
        onOpenChange={(open) => {
          if (!open) setSession(null)
        }}
      />
    </div>
  )
}
