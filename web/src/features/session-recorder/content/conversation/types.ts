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

/**
 * Provider-neutral model of one recorded request/response, built by
 * `parseConversation` and rendered by `ConversationView`.
 */

/** An image as recorded: inline data or a remote URL. */
export type ImageSource =
  | { kind: 'data'; mediaType: string; base64: string }
  | { kind: 'url'; url: string }

export type Part =
  | { type: 'text'; text: string }
  | { type: 'reasoning'; text: string; redacted?: boolean }
  | { type: 'image'; source: ImageSource; caption?: string }
  | { type: 'tool_call'; id?: string; name: string; input: string }
  | {
      type: 'tool_result'
      id?: string
      name?: string
      output: string
      isError?: boolean
    }
  | { type: 'file'; name?: string; mediaType?: string; size?: number }

export type Role = 'system' | 'user' | 'assistant' | 'tool'

export interface Turn {
  role: Role
  parts: Part[]
  /** e.g. "choice 2" when a response carries several choices. */
  label?: string
}

export type ConversationFormat =
  | 'openai-chat'
  | 'openai-responses'
  | 'anthropic'
  | 'gemini'
  | 'image-generation'

export interface Conversation {
  format: ConversationFormat
  /** System prompt, instructions and history sent by the client. */
  request: Turn[]
  /** What the model produced; empty when the response was an error. */
  response: Turn[]
  /** Key request parameters worth showing (model, size, n, ...). */
  params: Record<string, string>
  usage?: Record<string, number>
  finishReason?: string
  /** Upstream error message when the response is an error object. */
  error?: string
  /** The response body could not be parsed completely. */
  partial?: boolean
}
