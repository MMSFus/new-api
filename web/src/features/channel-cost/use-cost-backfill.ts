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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getCostBackfillStatus } from './api'

export const COST_BACKFILL_QUERY_KEY = ['channel-cost-backfill'] as const

/**
 * Backfill task status, polled faster while a task runs. Only the root user
 * may backfill. With `trackCompletion`, a task reaching a terminal state
 * refreshes the cost report once.
 */
export function useCostBackfillStatus(options: { trackCompletion: boolean }) {
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const queryClient = useQueryClient()
  const lastTerminalTask = useRef('')
  const statusQuery = useQuery({
    queryKey: COST_BACKFILL_QUERY_KEY,
    queryFn: getCostBackfillStatus,
    enabled: isRoot,
    retry: false,
    refetchInterval: (query) =>
      ['pending', 'running'].includes(query.state.data?.task?.status ?? '')
        ? 3000
        : 15000,
    refetchIntervalInBackground: false,
    meta: { errorToast: false },
  })
  const task = statusQuery.data?.task
  const active = task?.status === 'pending' || task?.status === 'running'

  useEffect(() => {
    if (!options.trackCompletion) return
    if (!task || active || lastTerminalTask.current === task.task_id) return
    lastTerminalTask.current = task.task_id
    void queryClient.invalidateQueries({ queryKey: ['channel-cost'] })
  }, [options.trackCompletion, task, active, queryClient])

  return { isRoot, statusQuery, task, active }
}
