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
  isEventStream,
  isObject,
  parseSse,
  pickParams,
  type JsonObject,
} from './parse-common'
import type { Conversation, ImageSource, Part } from './types'

const PARAMS = [
  'model',
  'size',
  'quality',
  'n',
  'style',
  'output_format',
  'background',
  'moderation',
  'response_format',
  'stream',
]

/** A decoded image request: JSON fields, or the fields and files of a multipart form. */
export interface ImageRequest {
  fields: JsonObject
  /** Input images of an edit request, with their form field names. */
  images: { field: string; source: ImageSource }[]
}

function mediaTypeOf(format: string): string {
  if (format === 'jpg' || format === 'jpeg') return 'image/jpeg'
  if (format === 'webp') return 'image/webp'
  return 'image/png'
}

/** `data[]` entries of an images response. */
function dataImages(data: unknown, mediaType: string): Part[] {
  const parts: Part[] = []
  for (const item of asArray(data).filter(isObject)) {
    const caption = asString(item.revised_prompt) || undefined
    if (item.b64_json) {
      parts.push({
        type: 'image',
        source: { kind: 'data', mediaType, base64: asString(item.b64_json) },
        caption,
      })
    } else if (item.url) {
      parts.push({
        type: 'image',
        source: { kind: 'url', url: asString(item.url) },
        caption,
      })
    }
  }
  return parts
}

/**
 * Images of a body that is not valid JSON, normally because the recording
 * stopped at its size limit: every complete b64_json or url value is kept.
 */
function salvageImages(text: string, mediaType: string): Part[] {
  const parts: Part[] = []
  for (const match of text.matchAll(/"(b64_json|url)"\s*:\s*"([^"]+)"/g)) {
    if (match[1] === 'b64_json') {
      parts.push({
        type: 'image',
        source: { kind: 'data', mediaType, base64: match[2] },
      })
    } else if (/^https?:\/\//.test(match[2])) {
      parts.push({ type: 'image', source: { kind: 'url', url: match[2] } })
    }
  }
  return parts
}

/** gpt-image streams: completed images win; otherwise the last partial of each. */
function streamImages(
  text: string,
  mediaType: string,
  out: Conversation
): Part[] {
  const sse = parseSse(text)
  out.partial ||= sse.broken
  const completed: Part[] = []
  // Partial events carry no image index, only a frame counter of one
  // image, so only the latest frame is kept.
  let latestPartial: Part | undefined
  for (const { data } of sse.events) {
    if (!isObject(data)) continue
    out.error ??= errorMessage(data)
    const type = asString(data.type)
    const b64 = asString(data.b64_json)
    if (!b64) continue
    const source: ImageSource = {
      kind: 'data',
      mediaType: data.output_format
        ? mediaTypeOf(asString(data.output_format))
        : mediaType,
      base64: b64,
    }
    if (type.endsWith('.completed')) {
      completed.push({ type: 'image', source })
      out.usage = flattenUsage(data.usage) ?? out.usage
    } else if (type.endsWith('.partial_image')) {
      latestPartial = { type: 'image', source }
    }
  }
  if (completed.length > 0 || !latestPartial) return completed
  out.partial = true
  return [latestPartial]
}

export function parseImageGeneration(
  request: ImageRequest,
  response: unknown,
  responseText: string,
  contentType: string
): Conversation {
  const out: Conversation = {
    format: 'image-generation',
    request: [],
    response: [],
    params: pickParams(request.fields, PARAMS),
  }
  const prompt: Part[] = []
  if (request.fields.prompt) {
    prompt.push({ type: 'text', text: asString(request.fields.prompt) })
  }
  for (const image of request.images) {
    prompt.push({ type: 'image', source: image.source, caption: image.field })
  }
  if (prompt.length > 0) out.request.push({ role: 'user', parts: prompt })

  const mediaType = mediaTypeOf(asString(request.fields.output_format))
  let images: Part[] = []
  if (isObject(response)) {
    out.error = errorMessage(response)
    out.usage = flattenUsage(response.usage)
    images = dataImages(response.data, mediaType)
  } else if (isEventStream(contentType, responseText)) {
    images = streamImages(responseText, mediaType, out)
  } else if (responseText) {
    images = salvageImages(responseText, mediaType)
    out.partial = true
  }
  if (images.length > 0) out.response.push({ role: 'assistant', parts: images })
  return out
}
