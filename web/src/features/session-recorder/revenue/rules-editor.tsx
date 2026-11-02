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
import { Plus } from 'lucide-react'
import { Reorder } from 'motion/react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { translateServerText } from '@/lib/server-error-message'

import type { RevenueRuleSet } from '../api'
import { RevenueRuleRow } from './rule-row'
import { RevenueRuleTester } from './rule-tester'
import {
  MAX_REVENUE_RULES,
  newRuleDraft,
  reorderMove,
  revenueRulesFormSchema,
  toFormValues,
  toRule,
  type RevenueRulesFormValues,
} from './rules-schema'

export const REVENUE_RULES_FORM_ID = 'session-recorder-revenue-rules'

/**
 * Ordered rule list of the revenue tab. The first enabled rule whose
 * expression matches a model name prices its records.
 */
export function RevenueRulesEditor(props: {
  initial: RevenueRuleSet
  /** Site currency symbol, the unit when the label is empty. */
  siteSymbol: string
  saveError: Error | null
  onSave: (set: RevenueRuleSet) => void
}) {
  const { t } = useTranslation()
  const form = useForm<RevenueRulesFormValues>({
    resolver: zodResolver(revenueRulesFormSchema),
    defaultValues: toFormValues(props.initial),
  })
  const rules = useFieldArray({
    control: form.control,
    name: 'rules',
    keyName: 'key',
  })
  const keys = rules.fields.map((field) => field.key)
  const label = form.watch('currency_label').trim()
  const unit = label || props.siteSymbol

  return (
    <Form {...form}>
      <form
        id={REVENUE_RULES_FORM_ID}
        className='space-y-4'
        noValidate
        onSubmit={form.handleSubmit((values) =>
          props.onSave({
            rules: values.rules.map(toRule),
            currency_label: values.currency_label.trim(),
          })
        )}
      >
        <FormField
          control={form.control}
          name='currency_label'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Currency label')}</FormLabel>
              <FormControl>
                <Input {...field} placeholder={props.siteSymbol} />
              </FormControl>
              <FormDescription>
                {t(
                  'Unit shown next to revenue. Leave empty to use the site currency symbol. Amounts are never converted.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='flex items-center justify-between gap-2'>
          <div>
            <h3 className='text-sm font-medium'>{t('Rules')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Drag to set priority. Prices are per TB (10^12 bytes) of compressed data.'
              )}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={rules.fields.length >= MAX_REVENUE_RULES}
            onClick={() => rules.append(newRuleDraft())}
          >
            <Plus aria-hidden='true' />
            {t('Add rule')}
          </Button>
        </div>
        {rules.fields.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('No rules. Every archived record counts as unmatched.')}
          </p>
        ) : (
          <Reorder.Group
            as='ol'
            axis='y'
            values={keys}
            onReorder={(next) => {
              const move = reorderMove(keys, next)
              if (move) rules.move(move[0], move[1])
            }}
            aria-label={t('Rules')}
            className='flex flex-col gap-2'
          >
            {rules.fields.map((field, index) => (
              <RevenueRuleRow
                key={field.key}
                rowKey={field.key}
                form={form}
                index={index}
                count={rules.fields.length}
                unit={unit}
                onMove={(from, direction) =>
                  rules.move(from, direction === 'up' ? from - 1 : from + 1)
                }
                onRemove={rules.remove}
              />
            ))}
          </Reorder.Group>
        )}
        <RevenueRuleTester
          getRules={() => form.getValues('rules').map(toRule)}
        />
        {props.saveError && (
          <Alert variant='destructive'>
            <AlertDescription role='alert'>
              {translateServerText(t, props.saveError.message)}
            </AlertDescription>
          </Alert>
        )}
      </form>
    </Form>
  )
}
