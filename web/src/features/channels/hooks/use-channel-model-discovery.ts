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
import { useCallback, useEffect, useRef, useState } from 'react'

import { createServerError } from '@/lib/server-error-message'

import { fetchModels, fetchUpstreamModels, stageKimiCredentials } from '../api'
import { CHANNEL_TYPE_KIMI } from '../constants'

type DiscoveryState = {
  status: 'idle' | 'loading' | 'success' | 'error' | 'stale'
  models: string[]
  error?: unknown
}

export type ChannelModelDiscoveryRequest =
  | { kind: 'saved'; channelId: number }
  | { kind: 'preview'; data: Parameters<typeof fetchModels>[0] }

type ChannelModelDiscoveryProps = {
  enabled: boolean
  request: ChannelModelDiscoveryRequest
  onCredentialDraft?: (key: string) => void
}

export function useChannelModelDiscovery(props: ChannelModelDiscoveryProps) {
  const onCredentialDraft = props.onCredentialDraft
  const [state, setState] = useState<DiscoveryState>({
    status: 'idle',
    models: [],
  })
  const sequence = useRef(0)
  const preparedRequest = useRef<ChannelModelDiscoveryRequest | null>(null)

  useEffect(() => {
    if (
      props.enabled &&
      preparedRequest.current &&
      JSON.stringify(preparedRequest.current) === JSON.stringify(props.request)
    ) {
      preparedRequest.current = null
      return
    }
    preparedRequest.current = null
    sequence.current += 1
    setState((previous) => {
      if (previous.status === 'idle') return previous
      if (!props.enabled) return { status: 'idle', models: [] }
      return { status: 'stale', models: previous.models }
    })
  }, [props.enabled, props.request])

  useEffect(
    () => () => {
      sequence.current += 1
    },
    []
  )

  const fetch = useCallback(async () => {
    if (!props.enabled) return
    const requestSequence = ++sequence.current
    setState((previous) => ({ status: 'loading', models: previous.models }))
    try {
      let request = props.request
      if (
        request.kind === 'preview' &&
        request.data.type === CHANNEL_TYPE_KIMI &&
        !request.data.key?.trim().startsWith('kimi-oauth:')
      ) {
        const key = await stageKimiCredentials(request.data.key ?? '')
        if (requestSequence !== sequence.current) return
        request = { ...request, data: { ...request.data, key } }
        preparedRequest.current = request
        onCredentialDraft?.(key)
      }
      const response =
        request.kind === 'saved'
          ? await fetchUpstreamModels(request.channelId)
          : await fetchModels(request.data)
      if (requestSequence !== sequence.current) return
      if (!response.success) {
        throw createServerError(response, 'Failed to fetch models')
      }
      setState({ status: 'success', models: response.data ?? [] })
    } catch (error) {
      if (requestSequence !== sequence.current) return
      setState((previous) => ({
        status: 'error',
        models: previous.models,
        error,
      }))
    }
  }, [props.enabled, props.request, onCredentialDraft])

  return { ...state, fetch }
}
