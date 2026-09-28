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

// Limits mirror setting/config_group.go; the server re-validates on save.
export const MAX_CONFIG_GROUPS = 50
export const MAX_CONFIG_GROUP_MEMBERS = 20
export const MAX_CONFIG_GROUP_KEY_LENGTH = 32
export const MAX_CONFIG_GROUP_NAME_LENGTH = 64
export const MAX_CONFIG_GROUP_DESCRIPTION_LENGTH = 255

/** One editable config group. `id` is a client-only React key. */
export type ConfigGroupDraft = {
  id: string
  key: string
  name: string
  description: string
  groups: string[]
  user_groups: string[]
  cross_group_retry: boolean
}

export type ConfigGroupIssue = {
  index: number
  field: 'key' | 'name' | 'description' | 'groups'
  /** i18n key; interpolate with `params`. */
  message: string
  params?: Record<string, string | number>
}

let nextDraftId = 0

export function createConfigGroupDraft(
  values: Partial<Omit<ConfigGroupDraft, 'id'>> = {}
): ConfigGroupDraft {
  nextDraftId += 1
  return {
    id: `config-group-${nextDraftId}`,
    key: values.key ?? '',
    name: values.name ?? '',
    description: values.description ?? '',
    groups: values.groups ?? [],
    user_groups: values.user_groups ?? [],
    cross_group_retry: values.cross_group_retry ?? true,
  }
}

function toStringList(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

function toText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

/** Parses the ConfigGroups option; malformed values yield an empty list. */
export function parseConfigGroupsOption(raw: string): ConfigGroupDraft[] {
  if (!raw.trim()) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  return parsed.flatMap((item) => {
    if (!item || typeof item !== 'object') return []
    const record = item as Record<string, unknown>
    return [
      createConfigGroupDraft({
        key: toText(record.key),
        name: toText(record.name),
        description: toText(record.description),
        groups: toStringList(record.groups),
        user_groups: toStringList(record.user_groups),
        cross_group_retry: record.cross_group_retry === true,
      }),
    ]
  })
}

/** Serializes drafts to the option value, dropping client-only fields. */
export function serializeConfigGroups(drafts: ConfigGroupDraft[]): string {
  return JSON.stringify(
    drafts.map((draft) => ({
      key: draft.key.trim(),
      name: draft.name.trim(),
      description: draft.description.trim(),
      groups: draft.groups,
      user_groups: draft.user_groups,
      cross_group_retry: draft.cross_group_retry,
    }))
  )
}

/** Returns the group names defined in a GroupRatio JSON object. */
export function parseGroupRatioNames(raw: string): string[] {
  if (!raw.trim()) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    return Object.keys(parsed)
  } catch {
    return []
  }
}

const INVALID_KEY_PATTERN = /[\s:,]/

/**
 * Client-side validation matching the server rules, so admins see field
 * errors before saving. `knownGroups` are the GroupRatio group names.
 */
export function validateConfigGroups(
  drafts: ConfigGroupDraft[],
  knownGroups: string[]
): ConfigGroupIssue[] {
  const issues: ConfigGroupIssue[] = []
  const known = new Set(knownGroups)
  const seenKeys = new Set<string>()

  drafts.forEach((draft, index) => {
    const key = draft.key.trim()
    if (!key) {
      issues.push({ index, field: 'key', message: 'Key is required' })
    } else if (key.toLowerCase() === 'auto') {
      issues.push({ index, field: 'key', message: 'Key "auto" is reserved' })
    } else if (INVALID_KEY_PATTERN.test(key)) {
      issues.push({
        index,
        field: 'key',
        message: 'Key must not contain spaces, colons or commas',
      })
    } else if ([...key].length > MAX_CONFIG_GROUP_KEY_LENGTH) {
      issues.push({
        index,
        field: 'key',
        message: 'Key must be at most {{max}} characters',
        params: { max: MAX_CONFIG_GROUP_KEY_LENGTH },
      })
    } else if (known.has(key)) {
      issues.push({
        index,
        field: 'key',
        message: 'Key conflicts with an existing group',
      })
    } else if (seenKeys.has(key)) {
      issues.push({ index, field: 'key', message: 'Key is duplicated' })
    }
    seenKeys.add(key)

    if ([...draft.name.trim()].length > MAX_CONFIG_GROUP_NAME_LENGTH) {
      issues.push({
        index,
        field: 'name',
        message: 'Name must be at most {{max}} characters',
        params: { max: MAX_CONFIG_GROUP_NAME_LENGTH },
      })
    }
    if (
      [...draft.description.trim()].length > MAX_CONFIG_GROUP_DESCRIPTION_LENGTH
    ) {
      issues.push({
        index,
        field: 'description',
        message: 'Description must be at most {{max}} characters',
        params: { max: MAX_CONFIG_GROUP_DESCRIPTION_LENGTH },
      })
    }

    if (draft.groups.length === 0) {
      issues.push({
        index,
        field: 'groups',
        message: 'Select at least one group',
      })
    } else if (draft.groups.length > MAX_CONFIG_GROUP_MEMBERS) {
      issues.push({
        index,
        field: 'groups',
        message: 'Select at most {{max}} groups',
        params: { max: MAX_CONFIG_GROUP_MEMBERS },
      })
    } else {
      const unknown = draft.groups.find((group) => !known.has(group))
      if (unknown) {
        issues.push({
          index,
          field: 'groups',
          message: 'Group {{group}} does not exist',
          params: { group: unknown },
        })
      }
    }
  })

  return issues
}

/**
 * Applies a membership change while keeping the existing routing order:
 * kept groups stay in place and newly added groups are appended.
 */
export function mergeGroupSelection(
  current: string[],
  selected: string[]
): string[] {
  const selectedSet = new Set(selected)
  const kept = current.filter((group) => selectedSet.has(group))
  const keptSet = new Set(kept)
  const added = selected.filter((group) => !keptSet.has(group))
  return [...kept, ...added]
}
