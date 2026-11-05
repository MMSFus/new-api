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
import {
  asArray,
  asString,
  errorMessage,
  flattenUsage,
  formatJsonText,
  isObject,
  parseSse,
  pickParams,
  splitMarkdownImages,
  type JsonObject,
} from './parse-common'
import type { Conversation, ImageSource, Part, Turn } from './types'

const PARAMS = [
  'model',
  'stream',
  'max_tokens',
  'temperature',
  'top_p',
  'top_k',
]

function blockImage(block: JsonObject): ImageSource | null {
  const source = isObject(block.source) ? block.source : {}
  if (source.type === 'base64') {
    return {
      kind: 'data',
      mediaType: asString(source.media_type) || 'image/png',
      base64: asString(source.data),
    }
  }
  if (source.type === 'url') return { kind: 'url', url: asString(source.url) }
  return null
}

/** Message content: a string or a list of content blocks. */
function blocksToParts(content: unknown): Part[] {
  if (typeof content === 'string') return splitMarkdownImages(content)
  const parts: Part[] = []
  for (const block of asArray(content)) {
    if (!isObject(block)) continue
    if (block.type === 'text') {
      parts.push(...splitMarkdownImages(asString(block.text)))
    } else if (block.type === 'thinking') {
      parts.push({ type: 'reasoning', text: asString(block.thinking) })
    } else if (block.type === 'redacted_thinking') {
      parts.push({ type: 'reasoning', text: '', redacted: true })
    } else if (block.type === 'image') {
      const source = blockImage(block)
      if (source) parts.push({ type: 'image', source })
    } else if (block.type === 'document') {
      parts.push({
        type: 'file',
        name: asString(block.title) || 'document',
        mediaType: isObject(block.source)
          ? asString(block.source.media_type)
          : undefined,
      })
    } else if (block.type === 'tool_use' || block.type === 'server_tool_use') {
      parts.push({
        type: 'tool_call',
        id: asString(block.id),
        name: asString(block.name),
        input: formatJsonText(block.input),
      })
    } else if (block.type === 'tool_result') {
      const nested = asArray(block.content)
      const text =
        typeof block.content === 'string'
          ? block.content
          : nested
              .filter(isObject)
              .map((b) => asString(b.text))
              .join('\n')
      parts.push({
        type: 'tool_result',
        id: asString(block.tool_use_id),
        output: formatJsonText(text),
        isError: block.is_error === true,
      })
      parts.push(
        ...blocksToParts(
          nested.filter((b) => isObject(b) && b.type === 'image')
        )
      )
    } else {
      parts.push({
        type: 'tool_call',
        name: asString(block.type),
        input: formatJsonText(block),
      })
    }
  }
  return parts
}

/** Rebuilds the assistant message from message_start / content_block events. */
function streamMessage(text: string, out: Conversation): JsonObject {
  const sse = parseSse(text)
  out.partial ||= sse.broken
  let message: JsonObject = {}
  const blocks: JsonObject[] = []
  const json: string[] = []
  for (const { data } of sse.events) {
    if (!isObject(data)) continue
    const index = typeof data.index === 'number' ? data.index : blocks.length
    if (data.type === 'message_start' && isObject(data.message)) {
      message = { ...data.message }
    } else if (
      data.type === 'content_block_start' &&
      isObject(data.content_block)
    ) {
      blocks[index] = { ...data.content_block }
      json[index] = ''
    } else if (
      data.type === 'content_block_delta' &&
      isObject(data.delta) &&
      blocks[index]
    ) {
      const delta = data.delta
      const block = blocks[index]
      if (delta.type === 'text_delta') {
        block.text = asString(block.text) + asString(delta.text)
      } else if (delta.type === 'thinking_delta') {
        block.thinking = asString(block.thinking) + asString(delta.thinking)
      } else if (delta.type === 'input_json_delta') {
        json[index] += asString(delta.partial_json)
      }
    } else if (data.type === 'message_delta') {
      if (isObject(data.delta)) message.stop_reason = data.delta.stop_reason
      if (isObject(data.usage)) {
        message.usage = {
          ...(isObject(message.usage) ? message.usage : {}),
          ...data.usage,
        }
      }
    } else if (data.type === 'error') {
      out.error ??= errorMessage(data)
    }
  }
  blocks.forEach((block, i) => {
    if (json[i]) block.input = json[i]
  })
  return { ...message, content: blocks.filter(Boolean) }
}

export function parseAnthropic(
  request: JsonObject,
  response: unknown,
  responseText: string,
  stream: boolean
): Conversation {
  const out: Conversation = {
    format: 'anthropic',
    request: [],
    response: [],
    params: pickParams(request, PARAMS),
  }
  if (isObject(request.thinking)) {
    out.params.thinking = asString(request.thinking.type) || 'on'
  }
  if (request.system) {
    out.request.push({ role: 'system', parts: blocksToParts(request.system) })
  }
  for (const message of asArray(request.messages).filter(isObject)) {
    const parts = blocksToParts(message.content)
    // A user message that only carries tool results is the client's tool turn.
    let role: Turn['role'] = 'user'
    if (message.role === 'assistant') role = 'assistant'
    else if (
      parts.length > 0 &&
      parts.every((p) => p.type === 'tool_result' || p.type === 'image')
    ) {
      role = 'tool'
    }
    out.request.push({ role, parts })
  }
  const result = stream ? streamMessage(responseText, out) : response
  out.error ??= errorMessage(result)
  if (!isObject(result) || out.error) return out
  const turn: Turn = { role: 'assistant', parts: blocksToParts(result.content) }
  if (turn.parts.length > 0) out.response = [turn]
  out.usage = flattenUsage(result.usage)
  out.finishReason = asString(result.stop_reason) || undefined
  return out
}
