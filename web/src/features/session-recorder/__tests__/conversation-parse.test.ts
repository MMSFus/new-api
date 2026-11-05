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
import { File as NodeFile } from 'node:buffer'

import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  detectFormat,
  parseConversation,
  type BodyInput,
} from '../content/conversation/parse'

function json(value: unknown, partial = false): BodyInput {
  return {
    contentType: 'application/json',
    text: JSON.stringify(value),
    bytes: null,
    partial,
  }
}

function sse(events: unknown[], named = false): BodyInput {
  const text = events
    .map((event) => {
      const type = (event as { type?: string }).type
      const head = named && type ? `event: ${type}\n` : ''
      return `${head}data: ${JSON.stringify(event)}\n\n`
    })
    .join('')
  return {
    contentType: 'text/event-stream',
    text: `${text}data: [DONE]\n\n`,
    bytes: null,
    partial: false,
  }
}

function parse(
  path: string,
  request: BodyInput | null,
  response: BodyInput | null
) {
  return parseConversation({ path, model: 'm', request, response })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('format detection', () => {
  test('known paths select their protocol', () => {
    expect(detectFormat('/v1/chat/completions', null)).toBe('openai-chat')
    expect(detectFormat('/v1/responses', null)).toBe('openai-responses')
    expect(detectFormat('/v1/messages', null)).toBe('anthropic')
    expect(
      detectFormat('/v1beta/models/gemini-3:streamGenerateContent', null)
    ).toBe('gemini')
    expect(detectFormat('/v1/images/edits', null)).toBe('image-generation')
  })

  test('an unknown path falls back to the request body shape', () => {
    expect(detectFormat('/proxy', '{"contents":[]}')).toBe('gemini')
    expect(detectFormat('/proxy', '{"messages":[]}')).toBe('openai-chat')
    expect(detectFormat('/v1/embeddings', '{"input":"x"}')).toBe(
      'openai-responses'
    )
    expect(detectFormat('/v1/audio/speech', 'not json')).toBeNull()
  })
})

describe('OpenAI chat completions', () => {
  test('a streamed answer is assembled from deltas with reasoning and tool calls', async () => {
    const conversation = await parse(
      '/v1/chat/completions',
      json({
        model: 'gpt-5',
        stream: true,
        messages: [
          { role: 'system', content: 'Be brief.' },
          { role: 'user', content: 'Weather in Paris?' },
        ],
      }),
      sse([
        { choices: [{ index: 0, delta: { reasoning_content: 'Need tool.' } }] },
        { choices: [{ index: 0, delta: { content: 'Checking' } }] },
        { choices: [{ index: 0, delta: { content: ' now' } }] },
        {
          choices: [
            {
              index: 0,
              delta: {
                tool_calls: [
                  {
                    index: 0,
                    id: 'call_1',
                    function: { name: 'weather', arguments: '{"city":' },
                  },
                ],
              },
            },
          ],
        },
        {
          choices: [
            {
              index: 0,
              delta: {
                tool_calls: [{ index: 0, function: { arguments: '"Paris"}' } }],
              },
              finish_reason: 'tool_calls',
            },
          ],
        },
        { choices: [], usage: { prompt_tokens: 12, completion_tokens: 7 } },
      ])
    )

    expect(conversation?.request.map((turn) => turn.role)).toEqual([
      'system',
      'user',
    ])
    expect(conversation?.response).toEqual([
      {
        role: 'assistant',
        parts: [
          { type: 'reasoning', text: 'Need tool.' },
          { type: 'text', text: 'Checking now' },
          {
            type: 'tool_call',
            id: 'call_1',
            name: 'weather',
            input: '{\n  "city": "Paris"\n}',
          },
        ],
      },
    ])
    expect(conversation?.finishReason).toBe('tool_calls')
    expect(conversation?.usage).toEqual({
      prompt_tokens: 12,
      completion_tokens: 7,
    })
  })

  test('several choices become labelled response turns', async () => {
    const conversation = await parse(
      '/v1/chat/completions',
      json({ messages: [{ role: 'user', content: 'Hi' }], n: 2 }),
      json({
        choices: [
          { index: 0, message: { role: 'assistant', content: 'A' } },
          { index: 1, message: { role: 'assistant', content: 'B' } },
        ],
      })
    )

    expect(conversation?.response.map((turn) => turn.label)).toEqual([
      '#1',
      '#2',
    ])
  })

  test('markdown images in an answer become image parts', async () => {
    const conversation = await parse(
      '/v1/chat/completions',
      json({ messages: [{ role: 'user', content: 'Draw' }] }),
      json({
        choices: [
          {
            message: {
              role: 'assistant',
              content:
                'Here:\n![cat](https://img.example/cat.png)\n![dog](data:image/png;base64,AAAA)',
            },
          },
        ],
      })
    )

    const parts = conversation?.response[0].parts ?? []
    expect(parts.map((part) => part.type)).toEqual(['text', 'image', 'image'])
    expect(parts[1]).toMatchObject({
      source: { kind: 'url', url: 'https://img.example/cat.png' },
    })
    expect(parts[2]).toMatchObject({
      source: { kind: 'data', mediaType: 'image/png', base64: 'AAAA' },
    })
  })
})

describe('OpenAI Responses', () => {
  test('instructions, function calls and their outputs keep their order', async () => {
    const conversation = await parse(
      '/v1/responses',
      json({
        instructions: 'Answer in French.',
        input: [
          {
            type: 'message',
            role: 'user',
            content: [{ type: 'input_text', text: 'Hello' }],
          },
          {
            type: 'function_call',
            call_id: 'c1',
            name: 'lookup',
            arguments: '{}',
          },
          { type: 'function_call_output', call_id: 'c1', output: 'found' },
        ],
      }),
      json({
        status: 'completed',
        output: [
          { type: 'reasoning', summary: [], encrypted_content: 'opaque' },
          {
            type: 'message',
            role: 'assistant',
            content: [{ type: 'output_text', text: 'Bonjour' }],
          },
        ],
      })
    )

    expect(conversation?.request.map((turn) => turn.role)).toEqual([
      'system',
      'user',
      'assistant',
      'tool',
    ])
    expect(conversation?.response[0].parts).toEqual([
      { type: 'reasoning', text: '', redacted: true },
      { type: 'text', text: 'Bonjour' },
    ])
    expect(conversation?.finishReason).toBe('completed')
  })
})

describe('Anthropic messages', () => {
  test('a streamed message is rebuilt from content block events', async () => {
    const conversation = await parse(
      '/v1/messages',
      json({
        system: 'You are terse.',
        messages: [{ role: 'user', content: 'Hi' }],
        stream: true,
      }),
      sse(
        [
          {
            type: 'message_start',
            message: { role: 'assistant', usage: { input_tokens: 5 } },
          },
          {
            type: 'content_block_start',
            index: 0,
            content_block: { type: 'thinking', thinking: '' },
          },
          {
            type: 'content_block_delta',
            index: 0,
            delta: { type: 'thinking_delta', thinking: 'Greet.' },
          },
          {
            type: 'content_block_start',
            index: 1,
            content_block: { type: 'text', text: '' },
          },
          {
            type: 'content_block_delta',
            index: 1,
            delta: { type: 'text_delta', text: 'Hello' },
          },
          {
            type: 'content_block_start',
            index: 2,
            content_block: {
              type: 'tool_use',
              id: 't1',
              name: 'wave',
              input: {},
            },
          },
          {
            type: 'content_block_delta',
            index: 2,
            delta: { type: 'input_json_delta', partial_json: '{"times":2}' },
          },
          {
            type: 'message_delta',
            delta: { stop_reason: 'tool_use' },
            usage: { output_tokens: 9 },
          },
        ],
        true
      )
    )

    expect(conversation?.request[0]).toEqual({
      role: 'system',
      parts: [{ type: 'text', text: 'You are terse.' }],
    })
    expect(conversation?.response[0].parts).toEqual([
      { type: 'reasoning', text: 'Greet.' },
      { type: 'text', text: 'Hello' },
      {
        type: 'tool_call',
        id: 't1',
        name: 'wave',
        input: '{\n  "times": 2\n}',
      },
    ])
    expect(conversation?.finishReason).toBe('tool_use')
    expect(conversation?.usage).toMatchObject({
      input_tokens: 5,
      output_tokens: 9,
    })
  })

  test('a user turn of tool results is shown as a tool turn', async () => {
    const conversation = await parse(
      '/v1/messages',
      json({
        messages: [
          {
            role: 'user',
            content: [
              {
                type: 'tool_result',
                tool_use_id: 't1',
                content: 'done',
                is_error: true,
              },
            ],
          },
        ],
      }),
      null
    )

    expect(conversation?.request[0]).toMatchObject({
      role: 'tool',
      parts: [{ type: 'tool_result', id: 't1', output: 'done', isError: true }],
    })
  })
})

describe('Gemini', () => {
  test('an inline image and thought parts are kept from a streamed array', async () => {
    const conversation = await parse(
      '/v1beta/models/gemini-3:generateContent',
      json({ contents: [{ role: 'user', parts: [{ text: 'Paint' }] }] }),
      json([
        {
          candidates: [
            {
              content: {
                role: 'model',
                parts: [{ text: 'Plan', thought: true }],
              },
            },
          ],
        },
        {
          candidates: [
            {
              content: {
                role: 'model',
                parts: [
                  { inlineData: { mimeType: 'image/png', data: 'QUJD' } },
                ],
              },
              finishReason: 'STOP',
            },
          ],
        },
      ])
    )

    expect(conversation?.response[0].parts).toEqual([
      { type: 'reasoning', text: 'Plan' },
      {
        type: 'image',
        source: { kind: 'data', mediaType: 'image/png', base64: 'QUJD' },
      },
    ])
    expect(conversation?.finishReason).toBe('STOP')
  })
})

describe('image generation', () => {
  test('every generated image is kept with its revised prompt', async () => {
    const conversation = await parse(
      '/v1/images/generations',
      json({
        model: 'gpt-image-2',
        prompt: 'two cats',
        n: 3,
        size: '1024x1024',
      }),
      json({
        data: [
          { b64_json: 'AAAA', revised_prompt: 'cat one' },
          { b64_json: 'BBBB' },
          { url: 'https://img.example/3.png' },
        ],
      })
    )

    expect(conversation?.request[0].parts).toEqual([
      { type: 'text', text: 'two cats' },
    ])
    expect(conversation?.response[0].parts).toEqual([
      {
        type: 'image',
        source: { kind: 'data', mediaType: 'image/png', base64: 'AAAA' },
        caption: 'cat one',
      },
      {
        type: 'image',
        source: { kind: 'data', mediaType: 'image/png', base64: 'BBBB' },
      },
      {
        type: 'image',
        source: { kind: 'url', url: 'https://img.example/3.png' },
      },
    ])
    expect(conversation?.params).toMatchObject({ n: '3', size: '1024x1024' })
  })

  test('a truncated response still yields the images that arrived whole', async () => {
    const text =
      '{"data":[{"b64_json":"AAAA"},{"b64_json":"BBBB"},{"b64_json":"CC'
    const conversation = await parse(
      '/v1/images/generations',
      json({ prompt: 'x' }),
      { contentType: 'application/json', text, bytes: null, partial: true }
    )

    expect(conversation?.response[0].parts).toHaveLength(2)
    expect(conversation?.partial).toBe(true)
  })

  test('streamed partial frames keep only the latest one and mark it partial', async () => {
    const conversation = await parse(
      '/v1/images/generations',
      json({ prompt: 'x', stream: true }),
      sse(
        [
          {
            type: 'image_generation.partial_image',
            b64_json: 'P1',
            partial_image_index: 0,
          },
          {
            type: 'image_generation.partial_image',
            b64_json: 'P2',
            partial_image_index: 1,
          },
        ],
        true
      )
    )

    expect(conversation?.response[0].parts).toEqual([
      {
        type: 'image',
        source: { kind: 'data', mediaType: 'image/png', base64: 'P2' },
      },
    ])
    expect(conversation?.partial).toBe(true)
  })

  test('an edit request shows its prompt and uploaded image from multipart', async () => {
    // jsdom's File global fails undici's own check in `formData()`;
    // browsers do not have this split.
    vi.stubGlobal('File', NodeFile)
    const boundary = 'b0'
    const text = [
      `--${boundary}`,
      'Content-Disposition: form-data; name="prompt"',
      '',
      'add a hat',
      `--${boundary}`,
      'Content-Disposition: form-data; name="image[]"; filename="a.png"',
      'Content-Type: image/png',
      '',
      'PNG',
      `--${boundary}--`,
      '',
    ].join('\r\n')
    const conversation = await parse(
      '/v1/images/edits',
      {
        contentType: `multipart/form-data; boundary=${boundary}`,
        text,
        bytes: null,
        partial: false,
      },
      json({ data: [{ b64_json: 'AAAA' }] })
    )

    expect(conversation?.request[0].parts).toEqual([
      { type: 'text', text: 'add a hat' },
      {
        type: 'image',
        source: { kind: 'data', mediaType: 'image/png', base64: 'UE5H' },
        caption: 'image[]',
      },
    ])
  })
})

test('a response that is neither JSON nor SSE marks the conversation partial', async () => {
  const conversation = await parse(
    '/v1/chat/completions',
    json({ messages: [{ role: 'user', content: 'Hi' }] }),
    {
      contentType: 'application/json',
      text: '{"choices":[{"mess',
      bytes: null,
      partial: false,
    }
  )

  expect(conversation?.partial).toBe(true)
  expect(conversation?.request).toHaveLength(1)
})
