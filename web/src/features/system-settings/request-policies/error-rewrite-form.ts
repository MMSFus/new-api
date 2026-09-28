import type { TFunction } from 'i18next'
import { z } from 'zod'

import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

// Stored shape of one entry in the ErrorRewriteRules option (see
// setting/operation_setting/error_rewrite.go).
export type ErrorRewriteRule = {
  name: string
  enabled: boolean
  channel_ids?: number[]
  status_codes?: string
  error_codes?: string[]
  keywords?: string[]
  message_regex?: string
  response_status_code?: number
  response_error_code?: string
  response_message: string
  skip_retry?: boolean
}

export type ErrorRewriteRuleFormValue = {
  name: string
  enabled: boolean
  channel_ids: string
  status_codes: string
  error_codes: string
  keywords: string
  message_regex: string
  response_status_code: string
  response_error_code: string
  response_message: string
  skip_retry: boolean
}

export type ErrorRewriteFormValues = { rules: ErrorRewriteRuleFormValue[] }

export const emptyErrorRewriteRule: ErrorRewriteRuleFormValue = {
  name: '',
  enabled: true,
  channel_ids: '',
  status_codes: '',
  error_codes: '',
  keywords: '',
  message_regex: '',
  response_status_code: '',
  response_error_code: '',
  response_message: '',
  skip_retry: false,
}

function splitList(value: string, separator: RegExp): string[] {
  return value
    .split(separator)
    .map((item) => item.trim())
    .filter(Boolean)
}

function isValidRegex(pattern: string): boolean {
  try {
    new RegExp(pattern)
    return true
  } catch {
    return false
  }
}

export function createErrorRewriteSchema(t: TFunction) {
  const rule = z
    .object({
      name: z.string(),
      enabled: z.boolean(),
      channel_ids: z
        .string()
        .refine(
          (value) =>
            splitList(value, /[,，\s]+/).every((id) => /^\d+$/.test(id)),
          t('Enter channel IDs separated by commas')
        ),
      status_codes: z
        .string()
        .refine(
          (value) => parseHttpStatusCodeRules(value).ok,
          t('Invalid status code rules')
        ),
      error_codes: z.string(),
      keywords: z.string(),
      message_regex: z
        .string()
        .refine(
          (value) => value === '' || isValidRegex(value),
          t('Invalid regular expression')
        ),
      response_status_code: z
        .string()
        .refine(
          (value) =>
            value.trim() === '' ||
            (/^\d+$/.test(value.trim()) &&
              Number(value) >= 100 &&
              Number(value) <= 599),
          t('Enter a status code between 100 and 599, or leave empty')
        ),
      response_error_code: z.string(),
      response_message: z
        .string()
        .refine(
          (value) => value.trim() !== '',
          t('Customer message is required')
        ),
      skip_retry: z.boolean(),
    })
    .superRefine((value, context) => {
      const hasCondition =
        value.status_codes.trim() !== '' ||
        splitList(value.error_codes, /[,，\s]+/).length > 0 ||
        splitList(value.keywords, /\r?\n/).length > 0 ||
        value.message_regex !== ''
      if (!hasCondition) {
        context.addIssue({
          code: 'custom',
          path: ['keywords'],
          message: t(
            'Add at least one condition: status codes, error codes, keywords or message regex'
          ),
        })
      }
    })
  return z.object({ rules: z.array(rule) })
}

export function parseErrorRewriteRules(
  json: string | undefined
): ErrorRewriteRule[] {
  try {
    const parsed: unknown = JSON.parse(json || '[]')
    return Array.isArray(parsed) ? (parsed as ErrorRewriteRule[]) : []
  } catch {
    return []
  }
}

export function toErrorRewriteFormValues(
  rules: ErrorRewriteRule[]
): ErrorRewriteFormValues {
  return {
    rules: rules.map((rule) => ({
      name: rule.name ?? '',
      enabled: rule.enabled === true,
      channel_ids: (rule.channel_ids ?? []).join(', '),
      status_codes: rule.status_codes ?? '',
      error_codes: (rule.error_codes ?? []).join(', '),
      keywords: (rule.keywords ?? []).join('\n'),
      message_regex: rule.message_regex ?? '',
      response_status_code: rule.response_status_code
        ? String(rule.response_status_code)
        : '',
      response_error_code: rule.response_error_code ?? '',
      response_message: rule.response_message ?? '',
      skip_retry: rule.skip_retry === true,
    })),
  }
}

// Builds the stored rules, omitting empty optional conditions so that
// equivalent forms serialize identically.
export function toErrorRewriteRules(
  values: ErrorRewriteFormValues
): ErrorRewriteRule[] {
  return values.rules.map((value) => {
    const rule: ErrorRewriteRule = {
      name: value.name.trim(),
      enabled: value.enabled,
      response_message: value.response_message.trim(),
    }
    const channelIds = splitList(value.channel_ids, /[,，\s]+/).map(Number)
    if (channelIds.length > 0) rule.channel_ids = channelIds
    const statusCodes = parseHttpStatusCodeRules(value.status_codes).normalized
    if (statusCodes) rule.status_codes = statusCodes
    const errorCodes = splitList(value.error_codes, /[,，\s]+/)
    if (errorCodes.length > 0) rule.error_codes = errorCodes
    const keywords = splitList(value.keywords, /\r?\n/)
    if (keywords.length > 0) rule.keywords = keywords
    if (value.message_regex) rule.message_regex = value.message_regex
    if (value.response_status_code.trim()) {
      rule.response_status_code = Number(value.response_status_code)
    }
    const errorCode = value.response_error_code.trim()
    if (errorCode) rule.response_error_code = errorCode
    if (value.skip_retry) rule.skip_retry = true
    return rule
  })
}
