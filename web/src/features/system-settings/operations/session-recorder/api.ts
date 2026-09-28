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
import i18next from 'i18next'
import { toast } from 'sonner'

import { getChannels, getEnabledModels } from '@/features/channels/api'
import { getGroups } from '@/features/users/api'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  ApiEnvelope,
  SessionRecorderCheckResult,
  SessionRecorderSettings,
  SessionRecorderStatus,
} from './types'

const BASE = '/api/session_recorder'

export const sessionRecorderKeys = {
  settings: ['session-recorder', 'settings'] as const,
  status: ['session-recorder', 'status'] as const,
  channels: ['session-recorder', 'channels'] as const,
  groups: ['session-recorder', 'groups'] as const,
  models: ['session-recorder', 'models'] as const,
}

export type RecorderChannelOption = {
  id: number
  name: string
  models: string[]
}

async function fetchSettings(): Promise<SessionRecorderSettings> {
  const res = await api.get<ApiEnvelope<SessionRecorderSettings>>(
    `${BASE}/setting`
  )
  return requireServerSuccess(res.data).data as SessionRecorderSettings
}

async function fetchStatus(): Promise<SessionRecorderStatus> {
  const res = await api.get<ApiEnvelope<SessionRecorderStatus>>(
    `${BASE}/status`,
    { disableDuplicate: true } as Record<string, unknown>
  )
  return requireServerSuccess(res.data).data as SessionRecorderStatus
}

// Channels are paged (max 100 per page); collect up to 20 pages.
async function fetchAllChannels(): Promise<RecorderChannelOption[]> {
  const out: RecorderChannelOption[] = []
  for (let page = 1; page <= 20; page++) {
    const res = requireServerSuccess(
      await getChannels({ p: page, page_size: 100, id_sort: true })
    )
    const items = res.data?.items ?? []
    for (const item of items) {
      out.push({
        id: item.id,
        name: item.name,
        models: (item.models ?? '')
          .split(',')
          .map((m) => m.trim())
          .filter(Boolean),
      })
    }
    if (items.length < 100 || out.length >= (res.data?.total ?? 0)) break
  }
  return out
}

export function useSessionRecorderSettings() {
  return useQuery({
    queryKey: sessionRecorderKeys.settings,
    queryFn: fetchSettings,
  })
}

export function useSessionRecorderStatus(enabled: boolean) {
  return useQuery({
    queryKey: sessionRecorderKeys.status,
    queryFn: fetchStatus,
    refetchInterval: enabled ? 10_000 : false,
  })
}

export function useRecorderChannels() {
  return useQuery({
    queryKey: sessionRecorderKeys.channels,
    queryFn: fetchAllChannels,
    staleTime: 60_000,
  })
}

export function useRecorderGroups() {
  return useQuery({
    queryKey: sessionRecorderKeys.groups,
    queryFn: async () => requireServerSuccess(await getGroups()).data ?? [],
    staleTime: 60_000,
  })
}

export function useRecorderModels() {
  return useQuery({
    queryKey: sessionRecorderKeys.models,
    queryFn: async () =>
      requireServerSuccess(await getEnabledModels()).data ?? [],
    staleTime: 60_000,
  })
}

export function useUpdateSessionRecorderSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (settings: SessionRecorderSettings) => {
      const res = await api.put<ApiEnvelope<SessionRecorderSettings>>(
        `${BASE}/setting`,
        settings
      )
      return requireServerSuccess(res.data).data as SessionRecorderSettings
    },
    onSuccess: (data) => {
      queryClient.setQueryData(sessionRecorderKeys.settings, data)
      queryClient.invalidateQueries({ queryKey: sessionRecorderKeys.status })
      toast.success(i18next.t('Setting updated successfully'))
    },
    onError: (error: Error) =>
      handleServerError(error, i18next.t('Failed to update setting')),
  })
}

export function useCheckSessionRecorder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await api.post<ApiEnvelope<SessionRecorderCheckResult>>(
        `${BASE}/check`
      )
      return requireServerSuccess(res.data).data as SessionRecorderCheckResult
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: sessionRecorderKeys.status })
      if (data.state === 'ok') {
        toast.success(i18next.t('R2 connectivity check passed'))
      } else {
        toast.error(
          i18next.t('R2 connectivity check failed: {{error}}', {
            error: data.error ?? data.credential_state,
          })
        )
      }
    },
    onError: (error: Error) =>
      handleServerError(error, i18next.t('R2 connectivity check failed')),
  })
}
