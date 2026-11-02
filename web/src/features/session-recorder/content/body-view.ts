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
import type { RecordBody } from '../api'

/**
 * How a recorded body is shown: JSON is pretty-printed, SSE streams and other
 * text stay verbatim, base64 (binary) bodies are shown encoded. Truncated
 * JSON cannot be parsed and stays verbatim too.
 */
export function bodyView(body: RecordBody): {
  code: string
  language: 'json' | 'plaintext'
  extension: string
} {
  const type = body.content_type.toLowerCase()
  if (body.encoding === 'base64') {
    return { code: body.text, language: 'plaintext', extension: 'b64' }
  }
  if (type.includes('json') && !body.truncated) {
    try {
      const code = JSON.stringify(JSON.parse(body.text), null, 2)
      return { code, language: 'json', extension: 'json' }
    } catch {
      // Not valid JSON despite the header; fall through to plain text.
    }
  }
  if (type.includes('event-stream')) {
    return { code: body.text, language: 'plaintext', extension: 'sse' }
  }
  return { code: body.text, language: 'plaintext', extension: 'txt' }
}
