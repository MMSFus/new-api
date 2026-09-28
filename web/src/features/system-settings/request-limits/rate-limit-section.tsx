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
import { Code2, Palette } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
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
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  isValidRateLimitJSON,
  migrateLegacyRateLimits,
  parseRateLimitEntries,
  type RateLimitMode,
} from './rate-limit-rules'
import { RateLimitVisualEditor } from './rate-limit-visual-editor'

const groupRulesSchema = (t: (key: string) => string, mode: RateLimitMode) =>
  z
    .string()
    .optional()
    .refine((value) => isValidRateLimitJSON(mode, value), {
      message: t('Invalid JSON format or values out of allowed range'),
    })

const createRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    ModelRequestRateLimitEnabled: z.boolean(),
    ModelRequestRateLimitDurationMinutes: z.number().min(0),
    ModelRequestRateLimitCount: z.number().min(0).max(100000000),
    ModelRequestRateLimitSuccessCount: z.number().min(1).max(100000000),
    ModelRequestRateLimitGroup: groupRulesSchema(t, 'legacy'),
    ModelRequestRateLimitGlobalGroup: groupRulesSchema(t, 'global'),
    ModelRequestRateLimitPrivateGroup: groupRulesSchema(t, 'private'),
  })

type RateLimitFormValues = z.infer<ReturnType<typeof createRateLimitSchema>>

type GroupRuleFieldName =
  | 'ModelRequestRateLimitGlobalGroup'
  | 'ModelRequestRateLimitPrivateGroup'
  | 'ModelRequestRateLimitGroup'

type GroupRuleField = {
  name: GroupRuleFieldName
  mode: RateLimitMode
  label: string
  description: string
  placeholder: string
  format: string
}

const RULE_FORMAT = '{"total": n, "success": n, "duration"?: minutes}'

const GROUP_RULE_FIELDS: GroupRuleField[] = [
  {
    name: 'ModelRequestRateLimitGlobalGroup',
    mode: 'global',
    label: 'Global group rate limits',
    description:
      'Applies to every user who calls the group. Counted per user and called group.',
    placeholder: `{\n  "claude": {"total": 200, "success": 100},\n  "gpt": {"total": 0, "success": 1000, "duration": 5}\n}`,
    format: `{"calledGroup": ${RULE_FORMAT}}`,
  },
  {
    name: 'ModelRequestRateLimitPrivateGroup',
    mode: 'private',
    label: 'Private group rate limits',
    description:
      'Applies when users of a user group call a group. "*" matches any called group. Overrides global rules.',
    placeholder: `{\n  "vip": {\n    "claude": {"total": 0, "success": 500},\n    "*": {"total": 0, "success": 2000}\n  }\n}`,
    format: `{"userGroup": {"calledGroup" | "*": ${RULE_FORMAT}}}`,
  },
]

const LEGACY_RULE_FIELD: GroupRuleField = {
  name: 'ModelRequestRateLimitGroup',
  mode: 'legacy',
  label: 'Legacy group rate limits',
  description:
    'Deprecated. Looked up by the token group, falling back to the user group, and counted per user across all groups. Still applied when no global or private rule matches. Migrating copies each entry to a global rule for that group (existing global rules are kept) and clears this table.',
  placeholder: `{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}`,
  format: '{"groupName": [maxRequests, maxSuccess]}',
}

type RateLimitSectionProps = {
  defaultValues: RateLimitFormValues
}

export function RateLimitSection({ defaultValues }: RateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [useVisualEditor, setUseVisualEditor] = useState(true)

  const rateLimitSchema = createRateLimitSchema(t)

  const form = useForm<RateLimitFormValues>({
    resolver: zodResolver(rateLimitSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const legacyValue = form.watch('ModelRequestRateLimitGroup') || ''
  const hasLegacyRules = parseRateLimitEntries('legacy', legacyValue).length > 0
  // Keep the legacy table visible while it was non-empty when loaded, so a
  // migration can be reviewed before saving.
  const showLegacy =
    hasLegacyRules ||
    parseRateLimitEntries(
      'legacy',
      defaultValues.ModelRequestRateLimitGroup || ''
    ).length > 0

  const migrateLegacy = () => {
    const migrated = migrateLegacyRateLimits(
      legacyValue,
      form.getValues('ModelRequestRateLimitGlobalGroup') || ''
    )
    form.setValue('ModelRequestRateLimitGlobalGroup', migrated, {
      shouldDirty: true,
      shouldValidate: true,
    })
    form.setValue('ModelRequestRateLimitGroup', '{}', {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const onSubmit = async (values: RateLimitFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof RateLimitFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  const renderRuleField = (rule: GroupRuleField) => (
    <FormField
      key={rule.name}
      control={form.control}
      name={rule.name}
      render={({ field }) => (
        <FormItem>
          <div className='flex items-center justify-between gap-4'>
            <FormLabel>{t(rule.label)}</FormLabel>
            {rule.mode === 'legacy' && hasLegacyRules && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={migrateLegacy}
              >
                {t('Migrate to global rules')}
              </Button>
            )}
          </div>
          <FormDescription>{t(rule.description)}</FormDescription>
          <FormControl>
            {useVisualEditor ? (
              <RateLimitVisualEditor
                mode={rule.mode}
                value={field.value || ''}
                onChange={field.onChange}
              />
            ) : (
              <JsonCodeEditor
                value={field.value || ''}
                onChange={field.onChange}
                name={field.name}
                onBlur={field.onBlur}
                textareaRef={field.ref}
                placeholder={rule.placeholder}
                aria-invalid={Boolean(form.formState.errors[rule.name])}
              />
            )}
          </FormControl>
          {!useVisualEditor && (
            <FormDescription>
              <span className='text-xs'>
                {t('Format:')} {rule.format}
                {rule.mode === 'legacy' && (
                  <>
                    {' · '}
                    {t('maxRequests ≥ 0, maxSuccess ≥ 1, both ≤ 2,147,483,647')}
                  </>
                )}
              </span>
            </FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  )

  return (
    <SettingsSection title={t('Rate Limiting')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save rate limits'
          />
          <FormField
            control={form.control}
            name='ModelRequestRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
                  <FormDescription>
                    {t(
                      'This controls model request rate limiting. Web/API route throttling is configured by environment variables and may still return 429.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='ModelRequestRateLimitDurationMinutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Limit period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('minutes')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Time window for rate limiting')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max requests per period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Including failed requests, 0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitSuccessCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max successful requests')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={1}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 1)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Only successful requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='flex items-start justify-between gap-4'>
            <div className='space-y-1'>
              <p className='text-sm font-medium'>
                {t('Group-based rate limits')}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Priority: private rule for the called group > private "*" rule > global rule > legacy group limit > default limits above.'
                )}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setUseVisualEditor(!useVisualEditor)}
            >
              {useVisualEditor ? (
                <>
                  <Code2 className='mr-2 h-4 w-4' />
                  {t('JSON Mode')}
                </>
              ) : (
                <>
                  <Palette className='mr-2 h-4 w-4' />
                  {t('Visual Mode')}
                </>
              )}
            </Button>
          </div>

          {GROUP_RULE_FIELDS.map(renderRuleField)}
          {showLegacy && renderRuleField(LEGACY_RULE_FIELD)}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
