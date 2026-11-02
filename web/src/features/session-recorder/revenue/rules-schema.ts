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
import z from 'zod'

import {
  REVENUE_MATCH_TYPES,
  type RevenueRule,
  type RevenueRuleSet,
} from '../api'

/** Server limits of the rule list (contract §1.2). */
export const MAX_REVENUE_RULES = 100
export const MAX_PATTERN_LENGTH = 512
export const MAX_CURRENCY_LABEL_LENGTH = 16

// Messages are i18n keys; FormMessage translates them.
const ruleSchema = z.object({
  id: z.string().min(1),
  name: z.string().trim().min(1, 'Rule name is required'),
  match_type: z.enum(REVENUE_MATCH_TYPES),
  pattern: z
    .string()
    .trim()
    .min(1, 'Expression is required')
    .max(MAX_PATTERN_LENGTH, 'Expression must be at most 512 characters'),
  price_per_tb: z
    .string()
    .trim()
    .refine((value) => {
      if (value === '') return false
      const price = Number(value)
      return Number.isFinite(price) && price >= 0
    }, 'Enter a price of 0 or more'),
  enabled: z.boolean(),
})

export const revenueRulesFormSchema = z.object({
  rules: z.array(ruleSchema).max(MAX_REVENUE_RULES),
  currency_label: z
    .string()
    .trim()
    .max(
      MAX_CURRENCY_LABEL_LENGTH,
      'The currency label can be at most 16 characters'
    ),
})

export type RevenueRulesFormValues = z.infer<typeof revenueRulesFormSchema>
export type RevenueRuleDraft = RevenueRulesFormValues['rules'][number]

export function newRuleDraft(): RevenueRuleDraft {
  return {
    id: `r-${crypto.randomUUID().slice(0, 8)}`,
    name: '',
    match_type: 'keyword',
    pattern: '',
    price_per_tb: '0',
    enabled: true,
  }
}

export function toFormValues(
  set: RevenueRuleSet | undefined
): RevenueRulesFormValues {
  return {
    rules: (set?.rules ?? []).map((rule) => ({
      ...rule,
      price_per_tb: String(rule.price_per_tb),
    })),
    currency_label: set?.currency_label ?? '',
  }
}

/**
 * The single move that turns `prev` into `next` (a drag step), as
 * `[from, to]` indices; null when the orders are equal or differ otherwise.
 */
export function reorderMove(
  prev: readonly string[],
  next: readonly string[]
): [number, number] | null {
  const first = prev.findIndex((key, index) => key !== next[index])
  if (first < 0 || prev.length !== next.length) return null
  const candidates: [number, number][] = [
    [first, next.indexOf(prev[first])],
    [prev.indexOf(next[first]), first],
  ]
  for (const [from, to] of candidates) {
    const moved = [...prev]
    moved.splice(to, 0, ...moved.splice(from, 1))
    if (moved.join('\n') === next.join('\n')) return [from, to]
  }
  return null
}

/** Draft rule as sent to the server; an unparsable price becomes 0. */
export function toRule(draft: RevenueRuleDraft): RevenueRule {
  const price = Number(draft.price_per_tb)
  return {
    id: draft.id,
    name: draft.name.trim(),
    match_type: draft.match_type,
    pattern: draft.pattern.trim(),
    price_per_tb: Number.isFinite(price) ? price : 0,
    enabled: draft.enabled,
  }
}
