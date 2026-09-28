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
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Dialog } from '@/components/dialog'
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

import {
  RATE_LIMIT_ANY_GROUP,
  RATE_LIMIT_MAX_COUNT,
  RATE_LIMIT_MAX_DURATION_MINUTES,
  rateLimitEntryKey,
  type RateLimitEntryData,
  type RateLimitMode,
} from './rate-limit-rules'

export type { RateLimitEntryData } from './rate-limit-rules'

const createRateLimitDialogSchema = (
  t: (key: string) => string,
  mode: RateLimitMode
) =>
  z
    .object({
      userGroup: z.string().trim(),
      groupName: z.string().trim().min(1, t('Group name is required')),
      maxRequests: z
        .number()
        .int()
        .min(0, t('Must be ≥ 0'))
        .max(RATE_LIMIT_MAX_COUNT, t('Must be ≤ 2,147,483,647')),
      maxSuccess: z
        .number()
        .int()
        .min(
          mode === 'legacy' ? 1 : 0,
          mode === 'legacy' ? t('Must be ≥ 1') : t('Must be ≥ 0')
        )
        .max(RATE_LIMIT_MAX_COUNT, t('Must be ≤ 2,147,483,647')),
      durationMinutes: z
        .number()
        .int()
        .min(0, t('Must be ≥ 0'))
        .max(RATE_LIMIT_MAX_DURATION_MINUTES, t('Must be ≤ 1440')),
    })
    .superRefine((values, ctx) => {
      if (mode === 'private' && values.userGroup === '') {
        ctx.addIssue({
          code: 'custom',
          path: ['userGroup'],
          message: t('Group name is required'),
        })
      }
      if (values.groupName === RATE_LIMIT_ANY_GROUP && mode !== 'private') {
        ctx.addIssue({
          code: 'custom',
          path: ['groupName'],
          message: t('"*" is only allowed in private rules'),
        })
      }
      if (mode === 'private' && values.userGroup === RATE_LIMIT_ANY_GROUP) {
        ctx.addIssue({
          code: 'custom',
          path: ['userGroup'],
          message: t('"*" is only allowed as the called group'),
        })
      }
    })

type RateLimitDialogFormValues = z.infer<
  ReturnType<typeof createRateLimitDialogSchema>
>

const RATE_LIMIT_FORM_ID = 'rate-limit-form'

const emptyValues = (mode: RateLimitMode): RateLimitDialogFormValues => ({
  userGroup: '',
  groupName: '',
  maxRequests: 0,
  maxSuccess: mode === 'legacy' ? 1 : 0,
  durationMinutes: 0,
})

type RateLimitDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: RateLimitEntryData) => void
  editData?: RateLimitEntryData | null
  mode?: RateLimitMode
  /** Keys of existing rows, to reject adding a duplicate rule. */
  existingKeys?: Set<string>
}

export function RateLimitDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  mode = 'legacy',
  existingKeys,
}: RateLimitDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData
  const schema = useMemo(() => createRateLimitDialogSchema(t, mode), [t, mode])

  const form = useForm<RateLimitDialogFormValues>({
    resolver: zodResolver(schema),
    defaultValues: emptyValues(mode),
  })

  useEffect(() => {
    if (editData) {
      form.reset({
        ...emptyValues(mode),
        ...editData,
        userGroup: editData.userGroup ?? '',
        durationMinutes: editData.durationMinutes ?? 0,
      })
    } else {
      form.reset(emptyValues(mode))
    }
  }, [editData, form, open, mode])

  const handleSubmit = (values: RateLimitDialogFormValues) => {
    const entry: RateLimitEntryData = {
      groupName: values.groupName,
      maxRequests: values.maxRequests,
      maxSuccess: values.maxSuccess,
    }
    if (mode === 'private') entry.userGroup = values.userGroup
    if (mode !== 'legacy') entry.durationMinutes = values.durationMinutes
    const key = rateLimitEntryKey(entry)
    const previousKey = editData ? rateLimitEntryKey(editData) : undefined
    if (key !== previousKey && existingKeys?.has(key)) {
      form.setError('groupName', {
        message: t('A rule for this group already exists'),
      })
      return
    }
    onSave(entry)
    form.reset()
    onOpenChange(false)
  }

  const titles: Record<RateLimitMode, [edit: string, add: string]> = {
    legacy: [t('Edit group rate limit'), t('Add group rate limit')],
    global: [
      t('Edit global group rate limit'),
      t('Add global group rate limit'),
    ],
    private: [
      t('Edit private rate limit rule'),
      t('Add private rate limit rule'),
    ],
  }
  const title = titles[mode][isEditMode ? 0 : 1]

  const descriptions: Record<RateLimitMode, string> = {
    legacy: t('Configure rate limiting rules for a specific user group.'),
    global: t('Limits every user who calls this group.'),
    private: t(
      'Limits users of a user group when they call a specific group. Overrides global rules.'
    ),
  }
  const description = descriptions[mode]

  let groupNameHint = t(
    'The group the request is served from (the token group, or the group picked for an auto token).'
  )
  if (mode === 'legacy') {
    groupNameHint = isEditMode
      ? t('Group name cannot be changed when editing.')
      : t('Unique identifier for this group.')
  }

  const numberField = (
    name: 'maxRequests' | 'maxSuccess' | 'durationMinutes',
    label: string,
    hint: string,
    unit: string,
    min: number,
    max: number
  ) => (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <div className='flex items-center gap-2'>
              <Input
                type='number'
                min={min}
                max={max}
                step={1}
                {...field}
                onChange={(e) =>
                  field.onChange(Number.parseInt(e.target.value) || min)
                }
              />
              <span className='text-muted-foreground text-sm'>{unit}</span>
            </div>
          </FormControl>
          <FormDescription>{hint}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      contentClassName='sm:max-w-[500px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={RATE_LIMIT_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={RATE_LIMIT_FORM_ID}
          onSubmit={(e) => {
            // The dialog renders inside the settings form; keep its submit
            // from bubbling up and saving the whole page.
            e.stopPropagation()
            void form.handleSubmit(handleSubmit)(e)
          }}
          className='space-y-4'
        >
          {mode === 'private' && (
            <FormField
              control={form.control}
              name='userGroup'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('User group')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g., default, vip, premium')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('The group the calling user belongs to.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <FormField
            control={form.control}
            name='groupName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {mode === 'legacy' ? t('Group Name') : t('Called group')}
                </FormLabel>
                <FormControl>
                  <Input
                    placeholder={
                      mode === 'private'
                        ? t('e.g., claude, or * for any group')
                        : t('e.g., default, vip, premium')
                    }
                    {...field}
                    disabled={mode === 'legacy' && isEditMode}
                  />
                </FormControl>
                <FormDescription>{groupNameHint}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {numberField(
            'maxRequests',
            t('Max Requests (including failures)'),
            t('Total requests allowed per period. 0 = unlimited.'),
            t('times'),
            0,
            RATE_LIMIT_MAX_COUNT
          )}
          {numberField(
            'maxSuccess',
            t('Max Successful Requests'),
            mode === 'legacy'
              ? t('Only successful requests count toward this limit.')
              : t('Only successful requests count. 0 = unlimited.'),
            t('times'),
            mode === 'legacy' ? 1 : 0,
            RATE_LIMIT_MAX_COUNT
          )}
          {mode !== 'legacy' &&
            numberField(
              'durationMinutes',
              t('Limit period'),
              t('0 = use the default limit period.'),
              t('minutes'),
              0,
              RATE_LIMIT_MAX_DURATION_MINUTES
            )}
        </form>
      </Form>
    </Dialog>
  )
}
