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
import type { RecordBody } from '../../api'
import { parseAnthropic } from './parse-anthropic'
import {
  asString,
  isEventStream,
  isObject,
  tryParseJson,
  type JsonObject,
} from './parse-common'
import { parseGemini } from './parse-gemini'
import { parseImageGeneration, type ImageRequest } from './parse-image'
import { parseOpenAIChat, parseOpenAIResponses } from './parse-openai'
import type { Conversation, ConversationFormat } from './types'

/** A recorded body as text, plus its bytes when it was not UTF-8. */
export interface BodyInput {
  contentType: string
  text: string
  bytes: Uint8Array<ArrayBuffer> | null
  /** Only part of the stored body is present. */
  partial: boolean
}

export function bodyInput(body: RecordBody | null): BodyInput | null {
  if (!body) return null
  if (body.encoding === 'utf8') {
    return {
      contentType: body.content_type,
      text: body.text,
      bytes: null,
      partial: body.truncated,
    }
  }
  const binary = atob(body.text)
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0))
  return {
    contentType: body.content_type,
    text: new TextDecoder().decode(bytes),
    bytes,
    partial: body.truncated,
  }
}

/**
 * The protocol of a record, from its path and request body shape; null when
 * it is not one the conversation view knows.
 */
export function detectFormat(
  path: string,
  requestText: string | null
): ConversationFormat | null {
  const p = path.toLowerCase()
  if (p.includes('/images/')) return 'image-generation'
  if (p.includes('/chat/completions')) return 'openai-chat'
  if (p.includes('/responses')) return 'openai-responses'
  if (p.endsWith('/messages')) return 'anthropic'
  if (p.includes(':generatecontent') || p.includes(':streamgeneratecontent')) {
    return 'gemini'
  }
  if (!requestText) return null
  const body = tryParseJson(requestText)
  if (!isObject(body)) return null
  if (Array.isArray(body.contents)) return 'gemini'
  if (body.input !== undefined) return 'openai-responses'
  if (Array.isArray(body.messages)) return 'openai-chat'
  return null
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  }
  return btoa(binary)
}

/** Fields and image files of an image request (JSON or multipart edit). */
async function imageRequest(body: BodyInput | null): Promise<ImageRequest> {
  const out: ImageRequest = { fields: {}, images: [] }
  if (!body) return out
  if (!body.contentType.toLowerCase().startsWith('multipart/form-data')) {
    const parsed = tryParseJson(body.text)
    if (isObject(parsed)) out.fields = parsed
    return out
  }
  const raw = body.bytes ?? new TextEncoder().encode(body.text)
  try {
    const form = await new Response(raw, {
      headers: { 'Content-Type': body.contentType },
    }).formData()
    for (const [field, value] of form.entries()) {
      if (typeof value === 'string') {
        out.fields[field] = value
        continue
      }
      if (!value.type.startsWith('image/')) continue
      const base64 = bytesToBase64(new Uint8Array(await value.arrayBuffer()))
      out.images.push({
        field,
        source: { kind: 'data', mediaType: value.type, base64 },
      })
    }
  } catch {
    // A truncated form cannot be parsed; keep its leading text fields.
    for (const match of body.text.matchAll(
      /name="([^"]+)"\r?\n\r?\n([^\r\n]*)/g
    )) {
      out.fields[match[1]] = match[2]
    }
  }
  return out
}

export async function parseConversation(input: {
  path: string
  model: string
  request: BodyInput | null
  response: BodyInput | null
}): Promise<Conversation | null> {
  const format = detectFormat(input.path, input.request?.text ?? null)
  if (!format) return null
  const responseText = input.response?.text ?? ''
  const stream = isEventStream(input.response?.contentType ?? '', responseText)
  const response = stream ? undefined : tryParseJson(responseText)
  // A body that is not JSON and not SSE was cut off while recording.
  const unparsed = Boolean(responseText) && !stream && response === undefined

  let conversation: Conversation
  if (format === 'image-generation') {
    const request = await imageRequest(input.request)
    conversation = parseImageGeneration(
      request,
      response,
      responseText,
      input.response?.contentType ?? ''
    )
  } else {
    const parsed = tryParseJson(input.request?.text ?? '')
    const request: JsonObject = isObject(parsed) ? parsed : {}
    if (format === 'openai-chat') {
      conversation = parseOpenAIChat(request, response, responseText, stream)
    } else if (format === 'openai-responses') {
      conversation = parseOpenAIResponses(
        request,
        response,
        responseText,
        stream
      )
    } else if (format === 'anthropic') {
      conversation = parseAnthropic(request, response, responseText, stream)
    } else {
      conversation = parseGemini(
        request,
        response,
        responseText,
        input.model || asString(request.model)
      )
    }
    if (unparsed) conversation.partial = true
  }
  if (input.request?.partial || input.response?.partial) {
    conversation.partial = true
  }
  return conversation
}
