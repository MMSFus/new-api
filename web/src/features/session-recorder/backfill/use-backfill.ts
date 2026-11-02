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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import {
  cancelBackfill,
  getBackfillStatus,
  startBackfill,
  type BackfillStatus,
} from '../api'
import { SESSION_RECORDER_QUERY_KEY } from '../use-session-recorder-data'

export const BACKFILL_QUERY_KEY = [
  ...SESSION_RECORDER_QUERY_KEY,
  'backfill',
] as const

/**
 * Index backfill status, polled faster while it runs, with its start and
 * cancel actions. A run that stops refreshes the page data once, since its
 * indexed records change the revenue and the session list.
 */
export function useBackfill(enabled: boolean) {
  const queryClient = useQueryClient()
  const statusQuery = useQuery({
    queryKey: BACKFILL_QUERY_KEY,
    queryFn: getBackfillStatus,
    enabled,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.state === 'running' ? 3000 : 30000,
    refetchIntervalInBackground: false,
    meta: { errorToast: false },
  })
  const status = statusQuery.data
  const running = status?.state === 'running'
  // The server keeps a cancelled run `running` until its current batch ends;
  // remember which run was asked to stop so the panel can say so.
  const [cancelledTask, setCancelledTask] = useState<string | null>(null)
  const cancelling =
    running && cancelledTask !== null && cancelledTask === status?.task_id
  const wasRunning = useRef(false)

  useEffect(() => {
    if (running) {
      wasRunning.current = true
      return
    }
    if (!wasRunning.current || !status) return
    wasRunning.current = false
    void queryClient.invalidateQueries({
      queryKey: SESSION_RECORDER_QUERY_KEY,
      predicate: (query) => query.queryKey[1] !== BACKFILL_QUERY_KEY[1],
    })
  }, [running, status, queryClient])

  const start = useMutation({
    mutationFn: startBackfill,
    meta: { errorToast: false },
    onSuccess: (next) => {
      queryClient.setQueryData<BackfillStatus>(BACKFILL_QUERY_KEY, next)
    },
  })
  const cancel = useMutation({
    mutationFn: cancelBackfill,
    meta: { errorToast: false },
    onSuccess: (next) => {
      setCancelledTask(next.task_id || status?.task_id || null)
      queryClient.setQueryData<BackfillStatus>(BACKFILL_QUERY_KEY, next)
    },
  })

  return { statusQuery, status, running, cancelling, start, cancel }
}
