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
import { describe, expect, test } from 'vitest'

import { getUpstreamHost } from '../channel-utils'

describe('getUpstreamHost', () => {
  test('returns host with port for a full base URL', () => {
    expect(getUpstreamHost('http://wb-egress:7863/v1')).toBe('wb-egress:7863')
    expect(getUpstreamHost(' https://api.llmapi.ai ')).toBe('api.llmapi.ai')
  })

  test('returns null for empty or unparsable values', () => {
    expect(getUpstreamHost(undefined)).toBeNull()
    expect(getUpstreamHost('   ')).toBeNull()
    expect(getUpstreamHost('not a url')).toBeNull()
  })
})
