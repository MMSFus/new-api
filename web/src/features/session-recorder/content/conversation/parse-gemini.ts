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
  appendText,
  asArray,
  asString,
  errorMessage,
  flattenUsage,
  formatJsonText,
  isEventStream,
  isObject,
  parseSse,
  pickParams,
  splitMarkdownImages,
  type JsonObject,
} from './parse-common'
import type { Conversation, Part, Turn } from './types'

/** Gemini parts; streamed text chunks of the same kind are merged. */
function geminiParts(parts: unknown, into: Part[] = []): Part[] {
  for (const part of asArray(parts)) {
    if (!isObject(part)) continue
    const inlineValue = part.inlineData ?? part.inline_data
    const inline = isObject(inlineValue) ? inlineValue : null
    if (typeof part.text === 'string') {
      appendText(into, part.thought === true ? 'reasoning' : 'text', part.text)
    } else if (inline) {
      const mediaType = asString(inline.mimeType) || asString(inline.mime_type)
      if (mediaType.startsWith('image/')) {
        into.push({
          type: 'image',
          source: { kind: 'data', mediaType, base64: asString(inline.data) },
        })
      } else into.push({ type: 'file', mediaType })
    } else if (isObject(part.fileData)) {
      into.push({
        type: 'file',
        name: asString(part.fileData.fileUri),
        mediaType: asString(part.fileData.mimeType),
      })
    } else if (isObject(part.functionCall)) {
      into.push({
        type: 'tool_call',
        name: asString(part.functionCall.name),
        input: formatJsonText(part.functionCall.args),
      })
    } else if (isObject(part.functionResponse)) {
      into.push({
        type: 'tool_result',
        name: asString(part.functionResponse.name),
        output: formatJsonText(part.functionResponse.response),
      })
    }
  }
  return into
}

/** Markdown images in Gemini text are split out once the text is complete. */
function finishParts(parts: Part[]): Part[] {
  return parts.flatMap((part) =>
    part.type === 'text' ? splitMarkdownImages(part.text) : [part]
  )
}

/** Response chunks: SSE, a JSON array (stream without alt=sse) or one object. */
function responseChunks(
  response: unknown,
  text: string,
  out: Conversation
): unknown[] {
  if (Array.isArray(response)) return response
  if (response !== undefined) return [response]
  if (!isEventStream('', text)) return []
  const sse = parseSse(text)
  out.partial ||= sse.broken
  return sse.events.map((e) => e.data)
}

export function parseGemini(
  request: JsonObject,
  response: unknown,
  responseText: string,
  model: string
): Conversation {
  const out: Conversation = {
    format: 'gemini',
    request: [],
    response: [],
    params: model ? { model } : {},
  }
  const config = isObject(request.generationConfig)
    ? request.generationConfig
    : {}
  Object.assign(
    out.params,
    pickParams(config, [
      'temperature',
      'topP',
      'maxOutputTokens',
      'candidateCount',
    ])
  )
  const system = request.systemInstruction ?? request.system_instruction
  if (isObject(system)) {
    out.request.push({
      role: 'system',
      parts: finishParts(geminiParts(system.parts)),
    })
  }
  for (const content of asArray(request.contents).filter(isObject)) {
    const parts = finishParts(geminiParts(content.parts))
    const toolTurn =
      parts.length > 0 && parts.every((p) => p.type === 'tool_result')
    let role: Turn['role'] = 'user'
    if (content.role === 'model') role = 'assistant'
    else if (toolTurn) role = 'tool'
    out.request.push({ role, parts })
  }
  const candidates = new Map<number, Part[]>()
  for (const chunk of responseChunks(response, responseText, out)) {
    if (!isObject(chunk)) continue
    out.error ??= errorMessage(chunk)
    out.usage = flattenUsage(chunk.usageMetadata) ?? out.usage
    for (const candidate of asArray(chunk.candidates).filter(isObject)) {
      const index = typeof candidate.index === 'number' ? candidate.index : 0
      const parts = candidates.get(index) ?? []
      candidates.set(index, parts)
      if (isObject(candidate.content)) {
        geminiParts(candidate.content.parts, parts)
      }
      if (typeof candidate.finishReason === 'string') {
        out.finishReason = candidate.finishReason
      }
    }
  }
  out.response = [...candidates.entries()].map(([index, parts]) => ({
    role: 'assistant',
    parts: finishParts(parts),
    label: candidates.size > 1 ? `#${index + 1}` : undefined,
  }))
  return out
}
