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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { JsonEditor } from '@/components/json-editor'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const BALANCE_BUCKET_KEYS = [
  'topup',
  'aff_rebate',
  'invite_bonus',
  'gift',
] as const

const OPTION_KEY = 'GroupBalanceBuckets'

/**
 * Returns an i18n key describing why the value is invalid, or null.
 * Mirrors the backend validation in setting/balance_bucket.go.
 */
function validateGroupBalanceBuckets(value: string): string | null {
  if (!value.trim()) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return 'Invalid JSON format'
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return 'Must be a JSON object mapping group names to balance type lists'
  }
  for (const [group, buckets] of Object.entries(parsed)) {
    if (!group.trim()) return 'Group name cannot be empty'
    if (!Array.isArray(buckets) || buckets.length === 0) {
      return 'Each group needs a non-empty list of balance types'
    }
    const seen = new Set<string>()
    for (const bucket of buckets) {
      if (
        typeof bucket !== 'string' ||
        !(BALANCE_BUCKET_KEYS as readonly string[]).includes(bucket)
      ) {
        return 'Unknown balance type. Allowed: topup, aff_rebate, invite_bonus, gift'
      }
      if (seen.has(bucket)) return 'Duplicate balance type in a group'
      seen.add(bucket)
    }
  }
  return null
}

const schema = z.object({
  value: z.string().superRefine((value, ctx) => {
    const error = validateGroupBalanceBuckets(value)
    if (error) ctx.addIssue({ code: 'custom', message: error })
  }),
})

type Values = z.infer<typeof schema>

function normalize(value: string): string {
  return value.trim() ? value : '{}'
}

export function BalanceBucketsSettingsSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initial = props.defaultValue === '{}' ? '' : props.defaultValue

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { value: initial },
  })

  const { isDirty, isSubmitting } = form.formState

  async function onSubmit(values: Values) {
    const next = normalize(values.value)
    if (next === normalize(initial)) {
      toast.info(t('No changes to save'))
      return
    }
    await updateOption.mutateAsync({ key: OPTION_KEY, value: next })
    form.reset(values)
  }

  return (
    <SettingsSection title={t('Group Balance Types')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save balance type settings'
          />
          <FormField
            control={form.control}
            name='value'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Usable balance types per group')}</FormLabel>
                <FormDescription>
                  {t(
                    'Map each group to the balance types it may spend, in deduction order. Types: topup (top-up), aff_rebate (referral cashback), invite_bonus (invite reward), gift (gifted). Use "*" as the fallback for unlisted groups. Unconfigured groups can use all balances.'
                  )}
                </FormDescription>
                <FormControl>
                  <JsonEditor
                    value={field.value}
                    onChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                    keyLabel={t('Group')}
                    valueLabel={t('Balance types (ordered)')}
                    keyPlaceholder='vip'
                    valuePlaceholder='["topup","gift"]'
                    emptyMessage={t(
                      'No group restrictions. All groups can use every balance type.'
                    )}
                    template={{ vip: ['topup'], default: ['gift', 'topup'] }}
                    valueType='any'
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
