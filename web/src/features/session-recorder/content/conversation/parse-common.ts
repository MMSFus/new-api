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
import type { ImageSource, Part } from './types'

/**
 * Shared helpers for the recorded-conversation parsers. The playground has
 * its own stream helpers, but they parse a live OpenAI chat stream one chunk
 * at a time and know no other protocol, so they are not reused here.
 */

export type JsonObject = Record<string, unknown>

export function isObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

export function asString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

/** Parses JSON, returning undefined instead of throwing. */
export function tryParseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

/** Tool arguments and results as readable text: JSON is pretty-printed. */
export function formatJsonText(value: unknown): string {
  if (typeof value === 'string') {
    const parsed = tryParseJson(value)
    return parsed === undefined ? value : JSON.stringify(parsed, null, 2)
  }
  if (value === undefined) return ''
  return JSON.stringify(value, null, 2)
}

export interface SseEvent {
  event?: string
  data: unknown
}

/**
 * Splits a recorded text/event-stream body into its JSON data events.
 * `[DONE]` and comment lines are dropped. A data line that is not JSON (in
 * practice the last event of a truncated recording) sets `broken`.
 */
export function parseSse(text: string): {
  events: SseEvent[]
  broken: boolean
} {
  const events: SseEvent[] = []
  let broken = false
  for (const block of text.split(/\r?\n\r?\n/)) {
    let event: string | undefined
    const data: string[] = []
    for (const line of block.split(/\r?\n/)) {
      if (line.startsWith('event:')) event = line.slice(6).trim()
      else if (line.startsWith('data:')) {
        data.push(line.slice(5).replace(/^ /, ''))
      }
    }
    const payload = data.join('\n')
    if (!payload || payload === '[DONE]') continue
    const parsed = tryParseJson(payload)
    if (parsed === undefined) {
      broken = true
      continue
    }
    events.push({ event, data: parsed })
  }
  return { events, broken }
}

export function isEventStream(contentType: string, text: string): boolean {
  if (contentType.toLowerCase().includes('event-stream')) return true
  return /^(event|data):/m.test(text.slice(0, 256))
}

/** A data: URL or remote URL as an image source; null for anything else. */
export function imageFromUrl(url: string): ImageSource | null {
  const match = /^data:([^;,]+);base64,(.*)$/s.exec(url)
  if (match) return { kind: 'data', mediaType: match[1], base64: match[2] }
  if (/^https?:\/\//i.test(url)) return { kind: 'url', url }
  return null
}

const MARKDOWN_IMAGE =
  /!\[([^\]]*)\]\(\s*(data:image\/[^;]+;base64,[A-Za-z0-9+/=\s]+|https?:\/\/[^\s)]+)\s*\)/g

/**
 * Splits text into text and image parts. Image models served through the
 * chat endpoint often return their pictures as markdown images in the
 * message text; pulling them out lets the gallery show them and keeps
 * multi-megabyte data URLs out of the text view.
 */
export function splitMarkdownImages(text: string): Part[] {
  const parts: Part[] = []
  let last = 0
  for (const match of text.matchAll(MARKDOWN_IMAGE)) {
    const source = imageFromUrl(match[2].replaceAll(/\s+/g, ''))
    if (!source) continue
    const before = text.slice(last, match.index)
    if (before.trim()) parts.push({ type: 'text', text: before })
    parts.push({ type: 'image', source, caption: match[1] || undefined })
    last = match.index + match[0].length
  }
  const rest = text.slice(last)
  if (rest.trim() || parts.length === 0) {
    parts.push({ type: 'text', text: rest })
  }
  return parts
}

/** Appends text to the trailing part of the same type, or starts one. */
export function appendText(
  parts: Part[],
  type: 'text' | 'reasoning',
  text: string
) {
  if (!text) return
  const tail = parts.at(-1)
  if (tail && tail.type === type) {
    tail.text += text
    return
  }
  parts.push({ type, text })
}

/** Upstream error message of an `{error: ...}` body, if any. */
export function errorMessage(body: unknown): string | undefined {
  if (!isObject(body)) return undefined
  const error = body.error
  if (typeof error === 'string') return error
  if (isObject(error)) return asString(error.message) || JSON.stringify(error)
  return undefined
}

/** Numeric fields of a usage object, nested objects flattened with a dot. */
export function flattenUsage(
  usage: unknown
): Record<string, number> | undefined {
  if (!isObject(usage)) return undefined
  const out: Record<string, number> = {}
  for (const [key, value] of Object.entries(usage)) {
    if (typeof value === 'number') out[key] = value
    if (!isObject(value)) continue
    for (const [inner, n] of Object.entries(value)) {
      if (typeof n === 'number') out[`${key}.${inner}`] = n
    }
  }
  return Object.keys(out).length ? out : undefined
}

/** Scalar request fields worth showing above the conversation. */
export function pickParams(
  body: JsonObject,
  keys: string[]
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const key of keys) {
    const value = body[key]
    if (
      typeof value === 'string' ||
      typeof value === 'number' ||
      typeof value === 'boolean'
    ) {
      out[key] = String(value)
    }
  }
  return out
}
