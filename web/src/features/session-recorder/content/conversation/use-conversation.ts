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
import { useQuery } from '@tanstack/react-query'

import {
  getRecordBody,
  type RecordBodyKind,
  type RecordContent,
  type SessionRecord,
} from '../../api'
import { SESSION_RECORDER_QUERY_KEY } from '../../use-session-recorder-data'
import { bodyInput, parseConversation, type BodyInput } from './parse'
import type { Conversation } from './types'

export interface ConversationResult {
  conversation: Conversation | null
  /** The full body of a truncated view could not be downloaded. */
  fullBodyFailed: boolean
}

/**
 * The content API returns at most 2 MiB per body. A truncated body is
 * downloaded in full (still bounded by the recorder's own size limit) so
 * multi-image responses and long histories parse completely; when the
 * download fails the 2 MiB view is parsed instead.
 */
async function resolveBody(
  recordId: string,
  kind: RecordBodyKind,
  content: RecordContent
): Promise<{ body: BodyInput | null; failed: boolean }> {
  const shown = content[kind]
  if (!shown?.truncated) return { body: bodyInput(shown), failed: false }
  try {
    const full = await getRecordBody(recordId, kind)
    return {
      body: {
        contentType: shown.content_type || full.contentType,
        text: new TextDecoder().decode(full.bytes),
        bytes: full.bytes,
        partial: full.truncated,
      },
      failed: false,
    }
  } catch {
    return { body: bodyInput(shown), failed: true }
  }
}

export function useRecordConversation(
  record: SessionRecord,
  content: RecordContent
) {
  return useQuery({
    queryKey: [...SESSION_RECORDER_QUERY_KEY, 'conversation', record.record_id],
    retry: false,
    staleTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
    queryFn: async (): Promise<ConversationResult> => {
      const [request, response] = await Promise.all([
        resolveBody(record.record_id, 'request', content),
        resolveBody(record.record_id, 'response', content),
      ])
      const conversation = await parseConversation({
        path: record.path,
        model: record.model_name,
        request: request.body,
        response: response.body,
      })
      return { conversation, fullBodyFailed: request.failed || response.failed }
    },
  })
}
