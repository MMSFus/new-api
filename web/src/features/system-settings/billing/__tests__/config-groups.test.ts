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

import {
  createConfigGroupDraft,
  mergeGroupSelection,
  parseConfigGroupsOption,
  parseGroupRatioNames,
  serializeConfigGroups,
  validateConfigGroups,
} from '../config-groups'

const knownGroups = ['default', 'vip', 'svip']

describe('config groups option mapping', () => {
  test('round-trips a stored option without client-only ids', () => {
    const raw =
      '[{"key":"best","name":"Best","description":"d","groups":["vip","default"],"user_groups":["vip"],"cross_group_retry":true}]'

    const drafts = parseConfigGroupsOption(raw)

    expect(drafts).toHaveLength(1)
    expect(drafts[0]?.groups).toEqual(['vip', 'default'])
    expect(JSON.parse(serializeConfigGroups(drafts))).toEqual(JSON.parse(raw))
  })

  test('treats empty or malformed options as an empty list', () => {
    expect(parseConfigGroupsOption('')).toEqual([])
    expect(parseConfigGroupsOption('not-json')).toEqual([])
    expect(parseConfigGroupsOption('{"key":"best"}')).toEqual([])
  })

  test('trims text fields when serializing', () => {
    const draft = createConfigGroupDraft({
      key: ' best ',
      name: ' Best ',
      groups: ['vip'],
    })

    expect(JSON.parse(serializeConfigGroups([draft]))[0]).toMatchObject({
      key: 'best',
      name: 'Best',
    })
  })

  test('reads group names from the GroupRatio option', () => {
    expect(parseGroupRatioNames('{"default":1,"vip":2}')).toEqual([
      'default',
      'vip',
    ])
    expect(parseGroupRatioNames('[]')).toEqual([])
    expect(parseGroupRatioNames('')).toEqual([])
  })
})

describe('config groups validation', () => {
  test('accepts a valid plan', () => {
    const drafts = [
      createConfigGroupDraft({ key: 'best', groups: ['svip', 'vip'] }),
    ]

    expect(validateConfigGroups(drafts, knownGroups)).toEqual([])
  })

  test.each([
    ['', ['vip'], 'Key is required'],
    ['Auto', ['vip'], 'Key "auto" is reserved'],
    ['a:b', ['vip'], 'Key must not contain spaces, colons or commas'],
    ['a b', ['vip'], 'Key must not contain spaces, colons or commas'],
    ['x'.repeat(33), ['vip'], 'Key must be at most {{max}} characters'],
    ['vip', ['default'], 'Key conflicts with an existing group'],
    ['best', [], 'Select at least one group'],
    ['best', ['vip', 'gone'], 'Group {{group}} does not exist'],
  ])('rejects key %j with groups %j', (key, groups, message) => {
    const issues = validateConfigGroups(
      [createConfigGroupDraft({ key, groups })],
      knownGroups
    )

    expect(issues.map((issue) => issue.message)).toContain(message)
  })

  test('reports the second of two plans that share a key', () => {
    const issues = validateConfigGroups(
      [
        createConfigGroupDraft({ key: 'best', groups: ['vip'] }),
        createConfigGroupDraft({ key: 'best', groups: ['default'] }),
      ],
      knownGroups
    )

    expect(issues).toEqual([
      { index: 1, field: 'key', message: 'Key is duplicated' },
    ])
  })
})

describe('config group membership changes', () => {
  test('keeps the existing order and appends newly selected groups', () => {
    expect(
      mergeGroupSelection(['svip', 'vip'], ['vip', 'default', 'svip'])
    ).toEqual(['svip', 'vip', 'default'])
  })

  test('drops deselected groups without reordering the rest', () => {
    expect(
      mergeGroupSelection(['svip', 'vip', 'default'], ['default', 'svip'])
    ).toEqual(['svip', 'default'])
  })
})
