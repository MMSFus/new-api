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
import { zodResolver } from '@hookform/resolvers/zod'
import i18next from 'i18next'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { CurrencyAmountInput } from '@/components/currency-amount-input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  convertAmountCurrency,
  parseAmountCurrency,
  type AmountCurrency,
} from '@/lib/currency'
import { formatQuota } from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
  SettingsFormGrid,
  SettingsFormGridItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

/** Reward options stored in quota units unless marked with a currency. */
const REWARD_FIELDS = [
  'QuotaForNewUser',
  'QuotaForInviter',
  'QuotaForInvitee',
] as const
type RewardField = (typeof REWARD_FIELDS)[number]
const TRUST_FIELD = 'trust_quota_usd'
const AMOUNT_CURRENCY_KEY = 'quota_setting.amount_currency'

type AmountCurrencyMap = Partial<
  Record<RewardField | typeof TRUST_FIELD, Exclude<AmountCurrency, 'USD'>>
>

/** Same key order as the backend's JSON map, so unchanged marks stay clean. */
function serializeAmountCurrency(map: AmountCurrencyMap): string {
  const entries = Object.entries(map)
    .filter(([, code]) => code === 'CNY' || code === 'CUSTOM')
    .sort(([left], [right]) => (left < right ? -1 : 1))
  return JSON.stringify(Object.fromEntries(entries))
}

function parseAmountCurrencyMap(value: string | undefined): AmountCurrencyMap {
  try {
    const parsed: unknown = JSON.parse(value || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }
    const result: AmountCurrencyMap = {}
    for (const [field, code] of Object.entries(parsed)) {
      const currency = parseAmountCurrency(code)
      if (currency !== 'USD') {
        result[field as keyof AmountCurrencyMap] = currency
      }
    }
    return result
  } catch {
    return {}
  }
}

const amountSchema = z.preprocess(
  (value) => (value === '' ? undefined : value),
  z.coerce
    .number({ error: () => i18next.t('Please enter a valid number') })
    .min(0, { error: () => i18next.t('Must be greater than or equal to 0') })
)

const quotaSchema = z
  .object({
    QuotaForNewUser: amountSchema,
    QuotaForInviter: amountSchema,
    QuotaForInvitee: amountSchema,
    TopUpLink: z.string(),
    quota_setting: z.object({
      enable_free_model_pre_consume: z.boolean(),
      trust_quota_usd: amountSchema,
      pre_consume_multiplier: z.coerce
        .number({ error: () => i18next.t('Please enter a valid number') })
        .positive({ error: () => i18next.t('Must be greater than 0') }),
      amount_currency: z.string().optional(),
    }),
  })
  .superRefine((data, ctx) => {
    // Unmarked rewards are parsed with Atoi; a decimal would become 0.
    const marks = parseAmountCurrencyMap(data.quota_setting.amount_currency)
    for (const field of REWARD_FIELDS) {
      if (!marks[field] && !Number.isInteger(data[field])) {
        ctx.addIssue({
          code: 'custom',
          path: [field],
          message: i18next.t('Quota must be a whole number'),
        })
      }
    }
  })

type QuotaFormValues = z.infer<typeof quotaSchema>

/**
 * Convert a field value to another currency without changing its real value.
 * Unmarked rewards are quota units; unmarked trust_quota_usd is USD. The
 * result for an unmarked reward is a whole quota amount because the backend
 * parses it with Atoi.
 */
function convertAmountField(
  value: number,
  from: AmountCurrency,
  to: AmountCurrency,
  quotaUnits: boolean,
  quotaPerUnit: number
): number | null {
  const source = quotaUnits && from === 'USD' ? value / quotaPerUnit : value
  const converted = convertAmountCurrency(source, from, to)
  if (converted === null) return null
  if (quotaUnits && to === 'USD') return Math.round(converted * quotaPerUnit)
  return Number(converted.toFixed(6))
}

type QuotaSettingsSectionProps = {
  defaultValues: QuotaFormValues
  complianceConfirmed?: boolean
}

export function QuotaSettingsSection({
  defaultValues,
  complianceConfirmed = true,
}: QuotaSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const quotaPerUnit = useSystemConfigStore(
    (state) => state.config.currency.quotaPerUnit
  )
  const unitsPerUSD =
    Number.isFinite(quotaPerUnit) && quotaPerUnit > 0 ? quotaPerUnit : 500000
  const normalizedDefaults = {
    ...defaultValues,
    quota_setting: {
      ...defaultValues.quota_setting,
      amount_currency: serializeAmountCurrency(
        parseAmountCurrencyMap(defaultValues.quota_setting.amount_currency)
      ),
    },
  }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<QuotaFormValues>({
      resolver: zodResolver(quotaSchema) as Resolver<
        QuotaFormValues,
        unknown,
        QuotaFormValues
      >,
      defaultValues: normalizedDefaults,
      onSubmit: async (_data, changedFields) => {
        const nextMarks = changedFields[AMOUNT_CURRENCY_KEY]
        const entries = Object.entries(changedFields).filter(
          ([key]) => key !== AMOUNT_CURRENCY_KEY
        )
        // Marks go first so a later amount is never read in the old unit. A
        // rejected mark (e.g. no valid rate) stops before amounts are saved.
        if (typeof nextMarks === 'string') {
          await updateOption.mutateAsync({
            key: AMOUNT_CURRENCY_KEY,
            value: nextMarks,
          })
        }
        for (const [key, value] of entries) {
          await updateOption.mutateAsync({
            key,
            value: value as string | number | boolean,
          })
        }
      },
    })

  const marks = parseAmountCurrencyMap(
    form.watch('quota_setting.amount_currency')
  )
  const changeCurrency = (
    field: RewardField | typeof TRUST_FIELD,
    next: AmountCurrency
  ) => {
    const path =
      field === TRUST_FIELD ? ('quota_setting.trust_quota_usd' as const) : field
    const current = marks[field] ?? 'USD'
    const value = Number(form.getValues(path))
    if (Number.isFinite(value)) {
      const converted = convertAmountField(
        value,
        current,
        next,
        field !== TRUST_FIELD,
        unitsPerUSD
      )
      if (converted === null) return
      form.setValue(path, converted, {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
    const nextMarks: AmountCurrencyMap = { ...marks }
    if (next === 'USD') delete nextMarks[field]
    else nextMarks[field] = next
    form.setValue(
      'quota_setting.amount_currency',
      serializeAmountCurrency(nextMarks),
      { shouldDirty: true, shouldValidate: true }
    )
  }
  const rewardDescription = (field: RewardField, value: unknown): string => {
    const amount = value === '' || value == null ? 0 : Number(value)
    const currency = marks[field] ?? 'USD'
    const quota =
      currency === 'USD'
        ? amount
        : (convertAmountField(amount, currency, 'USD', true, unitsPerUSD) ?? 0)
    return formatQuota(quota)
  }
  const trustCurrency = marks[TRUST_FIELD] ?? 'USD'
  const rewardFields: Array<{
    name: RewardField
    label: string
    description: (formattedQuota: string) => string
  }> = [
    {
      name: 'QuotaForNewUser',
      label: t('New User Quota'),
      description: (formattedQuota) =>
        t('Initial quota given to new users ({{formattedQuota}})', {
          formattedQuota,
        }),
    },
    {
      name: 'QuotaForInviter',
      label: t('Inviter Reward'),
      description: (formattedQuota) =>
        t('Quota given to users who invite others ({{formattedQuota}})', {
          formattedQuota,
        }),
    },
    {
      name: 'QuotaForInvitee',
      label: t('Invitee Reward'),
      description: (formattedQuota) =>
        t('Quota given to invited users ({{formattedQuota}})', {
          formattedQuota,
        }),
    },
  ]
  const renderRewardField = (config: (typeof rewardFields)[number]) => (
    <FormField
      key={config.name}
      control={form.control}
      name={config.name}
      render={({ field }) => {
        const currency = marks[config.name] ?? 'USD'
        return (
          <FormItem>
            <FormLabel>{config.label}</FormLabel>
            <FormControl>
              <CurrencyAmountInput
                ref={field.ref}
                name={field.name}
                value={field.value ?? ''}
                onValueChange={field.onChange}
                onBlur={field.onBlur}
                currency={currency}
                onCurrencyChange={(next) => changeCurrency(config.name, next)}
                currencyLabel={t('{{field}} currency', {
                  field: config.label,
                })}
                prefix={currency === 'USD' ? null : undefined}
                step={currency === 'USD' ? 1 : 'any'}
              />
            </FormControl>
            <FormDescription>
              {config.description(rewardDescription(config.name, field.value))}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )
      }}
    />
  )

  return (
    <SettingsSection title={t('Quota Settings')}>
      <FormNavigationGuard when={isDirty} />

      {!complianceConfirmed ? (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'Non-zero invitation rewards require compliance confirmation in Payment Gateway settings.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
          />
          <FormDirtyIndicator isDirty={isDirty} />
          <SettingsFormGrid>
            {renderRewardField(rewardFields[0])}

            <FormField
              control={form.control}
              name='quota_setting.trust_quota_usd'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {trustCurrency === 'USD'
                      ? t('Wallet pre-consume bypass threshold (USD)')
                      : t(
                          'Wallet pre-consume bypass threshold ({{currency}})',
                          { currency: trustCurrency }
                        )}
                  </FormLabel>
                  <FormControl>
                    <CurrencyAmountInput
                      ref={field.ref}
                      name={field.name}
                      value={field.value ?? ''}
                      onValueChange={field.onChange}
                      onBlur={field.onBlur}
                      currency={trustCurrency}
                      onCurrencyChange={(next) =>
                        changeCurrency(TRUST_FIELD, next)
                      }
                      currencyLabel={t('Bypass threshold currency')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Skip pre-consumption when the wallet balance and limited API key balance both exceed this amount. Set to 0 to always pre-consume. Subscriptions and asynchronous tasks always reserve quota.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='quota_setting.pre_consume_multiplier'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Input pre-consume multiplier')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step='any'
                      value={field.value ?? ''}
                      onChange={field.onChange}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Reserve the estimated input cost multiplied by this value, without estimating output tokens. Defaults to 1; positive decimals such as 0.5 and 1.5 are supported. Final charges use actual usage. Per-request and task prices are unaffected.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {renderRewardField(rewardFields[1])}
            {renderRewardField(rewardFields[2])}

            <SettingsFormGridItem span='full'>
              <FormField
                control={form.control}
                name='quota_setting.enable_free_model_pre_consume'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Pre-Consume for Free Models')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, zero-cost models also pre-consume quota before final settlement.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsFormGridItem>

            <FormField
              control={form.control}
              name='TopUpLink'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Top-Up Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://example.com/topup')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('External link for users to purchase quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
