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
 * Group rate limit tables.
 *
 * - legacy  (ModelRequestRateLimitGroup):  {"group": [total, success]}
 * - global  (ModelRequestRateLimitGlobalGroup):  {"calledGroup": Rule}
 * - private (ModelRequestRateLimitPrivateGroup):
 *   {"userGroup": {"calledGroup" | "*": Rule}}
 *
 * Rule = {"total": n, "success": n, "duration"?: minutes}
 */
export type RateLimitMode = 'legacy' | 'global' | 'private'

export const RATE_LIMIT_ANY_GROUP = '*'
export const RATE_LIMIT_MAX_COUNT = 2147483647
export const RATE_LIMIT_MAX_DURATION_MINUTES = 1440

export type RateLimitEntryData = {
  /** Private rules only: the caller's user group. */
  userGroup?: string
  /** Legacy: the group; global/private: the called group. */
  groupName: string
  maxRequests: number
  maxSuccess: number
  /** Global/private only; 0 uses the default period. */
  durationMinutes?: number
}

type GroupRule = { total: number; success: number; duration?: number }

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

const isCount = (value: unknown, min: number) =>
  typeof value === 'number' &&
  Number.isInteger(value) &&
  value >= min &&
  value <= RATE_LIMIT_MAX_COUNT

const isValidRule = (value: unknown): value is GroupRule => {
  if (!isRecord(value)) return false
  if (!isCount(value.total, 0) || !isCount(value.success, 0)) return false
  if (value.duration === undefined) return true
  return (
    typeof value.duration === 'number' &&
    Number.isInteger(value.duration) &&
    value.duration >= 0 &&
    value.duration <= RATE_LIMIT_MAX_DURATION_MINUTES
  )
}

const isValidGroupName = (name: string) =>
  name.trim() !== '' && name === name.trim()

const parseObject = (value: string | undefined): unknown => {
  if (!value || value.trim() === '') return {}
  try {
    return JSON.parse(value)
  } catch {
    return undefined
  }
}

export function isValidRateLimitJSON(
  mode: RateLimitMode,
  value: string | undefined
): boolean {
  const parsed = parseObject(value)
  if (!isRecord(parsed)) return false
  return Object.entries(parsed).every(([group, rule]) => {
    if (!isValidGroupName(group)) return false
    if (mode === 'legacy') {
      return (
        Array.isArray(rule) &&
        rule.length === 2 &&
        isCount(rule[0], 0) &&
        isCount(rule[1], 1)
      )
    }
    if (group === RATE_LIMIT_ANY_GROUP) return false
    if (mode === 'global') return isValidRule(rule)
    return (
      isRecord(rule) &&
      Object.entries(rule).every(
        ([called, limit]) => isValidGroupName(called) && isValidRule(limit)
      )
    )
  })
}

const toEntry = (
  groupName: string,
  rule: unknown,
  userGroup?: string
): RateLimitEntryData | null => {
  if (!isValidRule(rule)) return null
  return {
    userGroup,
    groupName,
    maxRequests: rule.total,
    maxSuccess: rule.success,
    durationMinutes: rule.duration ?? 0,
  }
}

export function parseRateLimitEntries(
  mode: RateLimitMode,
  value: string
): RateLimitEntryData[] {
  const parsed = parseObject(value)
  if (!isRecord(parsed)) return []
  const entries: RateLimitEntryData[] = []
  for (const [group, rule] of Object.entries(parsed)) {
    if (mode === 'legacy') {
      if (
        Array.isArray(rule) &&
        rule.length === 2 &&
        typeof rule[0] === 'number' &&
        typeof rule[1] === 'number'
      ) {
        entries.push({
          groupName: group,
          maxRequests: rule[0],
          maxSuccess: rule[1],
        })
      }
    } else if (mode === 'global') {
      const entry = toEntry(group, rule)
      if (entry) entries.push(entry)
    } else if (isRecord(rule)) {
      for (const [called, limit] of Object.entries(rule)) {
        const entry = toEntry(called, limit, group)
        if (entry) entries.push(entry)
      }
    }
  }
  return entries
}

const toRule = (entry: RateLimitEntryData): GroupRule => {
  const rule: GroupRule = {
    total: entry.maxRequests,
    success: entry.maxSuccess,
  }
  if (entry.durationMinutes) rule.duration = entry.durationMinutes
  return rule
}

const parseForWrite = (value: string): Record<string, unknown> => {
  const parsed = parseObject(value)
  return isRecord(parsed) ? parsed : {}
}

export function rateLimitEntryKey(entry: RateLimitEntryData): string {
  return entry.userGroup !== undefined
    ? `${entry.userGroup}\u0000${entry.groupName}`
    : entry.groupName
}

export function removeRateLimitEntry(
  mode: RateLimitMode,
  value: string,
  entry: RateLimitEntryData
): string {
  const parsed = parseForWrite(value)
  if (mode === 'private') {
    const userGroup = entry.userGroup ?? ''
    const rules = parsed[userGroup]
    if (isRecord(rules)) {
      delete rules[entry.groupName]
      if (Object.keys(rules).length === 0) delete parsed[userGroup]
    }
  } else {
    delete parsed[entry.groupName]
  }
  return JSON.stringify(parsed, null, 2)
}

export function upsertRateLimitEntry(
  mode: RateLimitMode,
  value: string,
  entry: RateLimitEntryData,
  previous?: RateLimitEntryData | null
): string {
  const base = previous
    ? removeRateLimitEntry(mode, value, previous)
    : JSON.stringify(parseForWrite(value))
  const parsed = parseForWrite(base)
  if (mode === 'legacy') {
    parsed[entry.groupName] = [entry.maxRequests, entry.maxSuccess]
  } else if (mode === 'global') {
    parsed[entry.groupName] = toRule(entry)
  } else {
    const userGroup = entry.userGroup ?? ''
    const existing = parsed[userGroup]
    const rules: Record<string, unknown> = isRecord(existing) ? existing : {}
    rules[entry.groupName] = toRule(entry)
    parsed[userGroup] = rules
  }
  return JSON.stringify(parsed, null, 2)
}

/**
 * Converts the legacy table into global rules. The legacy table was looked
 * up by the token group, falling back to the user group — that is, by the
 * group being called — so each entry becomes a global rule for that group.
 * Existing global rules win over legacy entries of the same group.
 */
export function migrateLegacyRateLimits(
  legacyValue: string,
  globalValue: string
): string {
  const global = parseForWrite(globalValue)
  for (const entry of parseRateLimitEntries('legacy', legacyValue)) {
    if (entry.groupName in global) continue
    global[entry.groupName] = toRule(entry)
  }
  return JSON.stringify(global, null, 2)
}
