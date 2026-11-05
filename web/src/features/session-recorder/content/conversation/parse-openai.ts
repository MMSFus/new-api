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
  imageFromUrl,
  isObject,
  parseSse,
  pickParams,
  splitMarkdownImages,
  type JsonObject,
} from './parse-common'
import type { Conversation, Part, Role, Turn } from './types'

const CHAT_PARAMS = [
  'model',
  'stream',
  'temperature',
  'top_p',
  'max_tokens',
  'max_completion_tokens',
  'reasoning_effort',
  'n',
]

function chatRole(value: unknown): Role {
  if (value === 'system' || value === 'developer') return 'system'
  if (value === 'assistant') return 'assistant'
  if (value === 'tool' || value === 'function') return 'tool'
  return 'user'
}

/** Chat completions content: a string or a list of typed parts. */
function chatContentParts(content: unknown): Part[] {
  if (typeof content === 'string') return splitMarkdownImages(content)
  const parts: Part[] = []
  for (const item of asArray(content)) {
    if (!isObject(item)) continue
    if (
      item.type === 'text' ||
      item.type === 'input_text' ||
      item.type === 'output_text'
    ) {
      parts.push(...splitMarkdownImages(asString(item.text)))
    } else if (item.type === 'image_url' || item.type === 'input_image') {
      const url = isObject(item.image_url)
        ? asString(item.image_url.url)
        : asString(item.image_url)
      const source = imageFromUrl(url)
      if (source) parts.push({ type: 'image', source })
    } else if (
      item.type === 'input_audio' ||
      item.type === 'file' ||
      item.type === 'input_file'
    ) {
      const file = isObject(item.file) ? item.file : item
      parts.push({
        type: 'file',
        name: asString(file.filename) || asString(item.type),
      })
    } else if (item.type === 'refusal') {
      parts.push({ type: 'text', text: asString(item.refusal) })
    }
  }
  return parts
}

function chatToolCalls(calls: unknown): Part[] {
  return asArray(calls)
    .filter(isObject)
    .map((call) => {
      const fn = isObject(call.function) ? call.function : {}
      return {
        type: 'tool_call',
        id: asString(call.id),
        name: asString(fn.name),
        input: formatJsonText(fn.arguments),
      }
    })
}

/** One chat message: request history or a non-stream response message. */
function chatMessage(message: JsonObject): Turn {
  const role = chatRole(message.role)
  const parts: Part[] = []
  const reasoning =
    asString(message.reasoning_content) || asString(message.reasoning)
  if (reasoning) parts.push({ type: 'reasoning', text: reasoning })
  if (role === 'tool') {
    parts.push({
      type: 'tool_result',
      id: asString(message.tool_call_id),
      name: asString(message.name),
      output: formatJsonText(message.content),
    })
  } else {
    parts.push(...chatContentParts(message.content))
  }
  // Some image gateways return the pictures beside the text.
  for (const image of asArray(message.images)) {
    const url =
      isObject(image) && isObject(image.image_url)
        ? asString(image.image_url.url)
        : ''
    const source = imageFromUrl(url)
    if (source) parts.push({ type: 'image', source })
  }
  parts.push(...chatToolCalls(message.tool_calls))
  return { role, parts }
}

/** Rebuilds the response choices of a chat completions stream. */
function chatStreamTurns(text: string, out: Conversation): Turn[] {
  const sse = parseSse(text)
  out.partial ||= sse.broken
  const choices = new Map<
    number,
    { parts: Part[]; calls: Map<number, Part & { type: 'tool_call' }> }
  >()
  for (const { data } of sse.events) {
    if (!isObject(data)) continue
    out.error ??= errorMessage(data)
    out.usage = flattenUsage(data.usage) ?? out.usage
    for (const choice of asArray(data.choices).filter(isObject)) {
      const index = typeof choice.index === 'number' ? choice.index : 0
      let state = choices.get(index)
      if (!state) choices.set(index, (state = { parts: [], calls: new Map() }))
      const delta = isObject(choice.delta) ? choice.delta : {}
      appendText(
        state.parts,
        'reasoning',
        asString(delta.reasoning_content) || asString(delta.reasoning)
      )
      appendText(state.parts, 'text', asString(delta.content))
      for (const call of asArray(delta.tool_calls).filter(isObject)) {
        const callIndex =
          typeof call.index === 'number' ? call.index : state.calls.size
        const fn = isObject(call.function) ? call.function : {}
        let part = state.calls.get(callIndex)
        if (!part) {
          state.calls.set(
            callIndex,
            (part = { type: 'tool_call', id: '', name: '', input: '' })
          )
        }
        part.id ||= asString(call.id)
        part.name ||= asString(fn.name)
        part.input += asString(fn.arguments)
      }
      if (typeof choice.finish_reason === 'string') {
        out.finishReason = choice.finish_reason
      }
    }
  }
  return [...choices.entries()].map(([index, state]) => {
    const parts = state.parts.flatMap((part) =>
      part.type === 'text' ? splitMarkdownImages(part.text) : [part]
    )
    for (const call of state.calls.values()) {
      parts.push({ ...call, input: formatJsonText(call.input) })
    }
    return {
      role: 'assistant',
      parts,
      label: choices.size > 1 ? `#${index + 1}` : undefined,
    }
  })
}

export function parseOpenAIChat(
  request: JsonObject,
  response: unknown,
  responseText: string,
  stream: boolean
): Conversation {
  const out: Conversation = {
    format: 'openai-chat',
    request: [],
    response: [],
    params: pickParams(request, CHAT_PARAMS),
  }
  out.request = asArray(request.messages).filter(isObject).map(chatMessage)
  if (stream) {
    out.response = chatStreamTurns(responseText, out)
    return out
  }
  out.error = errorMessage(response)
  if (!isObject(response)) return out
  out.usage = flattenUsage(response.usage)
  const choices = asArray(response.choices).filter(isObject)
  out.response = choices.map((choice, i) => {
    const turn = chatMessage(isObject(choice.message) ? choice.message : {})
    if (typeof choice.finish_reason === 'string') {
      out.finishReason = choice.finish_reason
    }
    return {
      ...turn,
      role: 'assistant',
      label: choices.length > 1 ? `#${i + 1}` : undefined,
    }
  })
  return out
}

const RESPONSES_PARAMS = [
  'model',
  'stream',
  'temperature',
  'top_p',
  'max_output_tokens',
  'previous_response_id',
  'store',
]

/** Responses API input/output items as turns; adjacent same-role items merge. */
function responsesItems(items: unknown[], fallbackRole: Role): Turn[] {
  const turns: Turn[] = []
  const push = (role: Role, parts: Part[]) => {
    if (parts.length === 0) return
    const tail = turns.at(-1)
    if (tail && tail.role === role) tail.parts.push(...parts)
    else turns.push({ role, parts })
  }
  for (const item of items) {
    if (!isObject(item)) continue
    const type = asString(item.type) || 'message'
    if (type === 'message') {
      push(
        item.role ? chatRole(item.role) : fallbackRole,
        chatContentParts(item.content)
      )
    } else if (type === 'reasoning') {
      const text = asArray(item.summary)
        .filter(isObject)
        .map((s) => asString(s.text))
        .join('\n\n')
      push('assistant', [
        {
          type: 'reasoning',
          text,
          redacted: !text && Boolean(item.encrypted_content),
        },
      ])
    } else if (type === 'function_call' || type === 'custom_tool_call') {
      push('assistant', [
        {
          type: 'tool_call',
          id: asString(item.call_id),
          name: asString(item.name),
          input: formatJsonText(item.arguments ?? item.input),
        },
      ])
    } else if (
      type === 'function_call_output' ||
      type === 'custom_tool_call_output'
    ) {
      push('tool', [
        {
          type: 'tool_result',
          id: asString(item.call_id),
          output: formatJsonText(item.output),
        },
      ])
    } else if (type === 'image_generation_call') {
      const result = asString(item.result)
      if (result) {
        push('assistant', [
          {
            type: 'image',
            source: {
              kind: 'data',
              mediaType: `image/${asString(item.output_format) || 'png'}`,
              base64: result,
            },
            caption: asString(item.revised_prompt) || undefined,
          },
        ])
      }
    } else {
      push(type.endsWith('_output') ? 'tool' : 'assistant', [
        { type: 'tool_call', name: type, input: formatJsonText(item) },
      ])
    }
  }
  return turns
}

/** The final response object of a Responses stream, or one rebuilt from its items. */
function responsesStreamResult(
  text: string,
  out: Conversation
): JsonObject | undefined {
  const sse = parseSse(text)
  out.partial ||= sse.broken
  let final: JsonObject | undefined
  const done: unknown[] = []
  const pending: Part[] = []
  for (const { data } of sse.events) {
    if (!isObject(data)) continue
    const type = asString(data.type)
    if (
      (type === 'response.completed' ||
        type === 'response.incomplete' ||
        type === 'response.failed') &&
      isObject(data.response)
    ) {
      final = data.response
    } else if (type === 'response.output_item.done') done.push(data.item)
    else if (type === 'response.output_text.delta') {
      appendText(pending, 'text', asString(data.delta))
    } else if (type === 'response.reasoning_summary_text.delta') {
      appendText(pending, 'reasoning', asString(data.delta))
    } else if (type === 'error') {
      out.error ??= asString(data.message) || errorMessage(data)
    }
  }
  // Some upstreams send an empty output list in response.completed.
  if (final && asArray(final.output).length > 0) return final
  if (done.length > 0) return { ...final, output: done }
  if (pending.length > 0) {
    out.partial = true
    out.response = [{ role: 'assistant', parts: pending }]
  }
  return final
}

export function parseOpenAIResponses(
  request: JsonObject,
  response: unknown,
  responseText: string,
  stream: boolean
): Conversation {
  const out: Conversation = {
    format: 'openai-responses',
    request: [],
    response: [],
    params: pickParams(request, RESPONSES_PARAMS),
  }
  const instructions = asString(request.instructions)
  if (instructions) {
    out.request.push({
      role: 'system',
      parts: [{ type: 'text', text: instructions }],
    })
  }
  if (typeof request.input === 'string') {
    out.request.push({
      role: 'user',
      parts: [{ type: 'text', text: request.input }],
    })
  } else out.request.push(...responsesItems(asArray(request.input), 'user'))
  let result = isObject(response) ? response : undefined
  if (stream) result = responsesStreamResult(responseText, out)
  out.error ??= errorMessage(response) ?? errorMessage(result)
  if (!result) return out
  if (asArray(result.output).length > 0) {
    out.response = responsesItems(asArray(result.output), 'assistant')
  }
  out.usage = flattenUsage(result.usage)
  const status = asString(result.status)
  const reason = isObject(result.incomplete_details)
    ? asString(result.incomplete_details.reason)
    : ''
  out.finishReason = reason || status || undefined
  return out
}
