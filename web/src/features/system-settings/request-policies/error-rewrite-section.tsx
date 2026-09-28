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
import { MessageSquareWarning, Plus } from 'lucide-react'
import { useMemo } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { handleServerError } from '@/lib/handle-server-error'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import {
  createErrorRewriteSchema,
  emptyErrorRewriteRule,
  parseErrorRewriteRules,
  toErrorRewriteFormValues,
  toErrorRewriteRules,
  type ErrorRewriteFormValues,
} from './error-rewrite-form'
import { ErrorRewriteRuleFields } from './error-rewrite-rule-fields'
import { useSavePolicy } from './use-save-policy'

type ErrorRewriteSectionProps = {
  defaultValues: { ErrorRewriteRules: string }
}

export function ErrorRewriteSection(props: ErrorRewriteSectionProps) {
  const { t } = useTranslation()
  const savePolicy = useSavePolicy()
  const initialValues = useMemo(
    () =>
      toErrorRewriteFormValues(
        parseErrorRewriteRules(props.defaultValues.ErrorRewriteRules)
      ),
    [props.defaultValues.ErrorRewriteRules]
  )
  const schema = useMemo(() => createErrorRewriteSchema(t), [t])
  const form = useForm<ErrorRewriteFormValues>({
    resolver: zodResolver(schema),
    defaultValues: initialValues,
  })
  useResetForm(form, initialValues)
  const rules = useFieldArray({ control: form.control, name: 'rules' })

  const onSubmit = async (values: ErrorRewriteFormValues) => {
    const next = JSON.stringify(toErrorRewriteRules(values))
    if (next === JSON.stringify(toErrorRewriteRules(initialValues))) {
      toast.info(t('No changes to save'))
      return
    }
    try {
      await savePolicy.mutateAsync({ ErrorRewriteRules: next })
    } catch (error) {
      handleServerError(error)
    }
  }

  const addRule = () => rules.append({ ...emptyErrorRewriteRule })

  return (
    <SettingsSection title={t('Error responses')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Replace matched errors with a custom status, error code and message for customers. Server logs and error logs keep the original error. Rules are checked in order and the first match wins.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={form.formState.isSubmitting}
          />
          {rules.fields.length === 0 ? (
            <EmptyState
              icon={MessageSquareWarning}
              title={t('No error rewrite rules')}
              description={t('Customers see the original error message.')}
              action={
                <Button type='button' variant='outline' onClick={addRule}>
                  <Plus className='size-4' aria-hidden='true' />
                  {t('Add rule')}
                </Button>
              }
              bordered
            />
          ) : (
            <div className='space-y-4'>
              {rules.fields.map((field, index) => (
                <ErrorRewriteRuleFields
                  key={field.id}
                  control={form.control}
                  index={index}
                  onRemove={rules.remove}
                />
              ))}
              <Button type='button' variant='outline' onClick={addRule}>
                <Plus className='size-4' aria-hidden='true' />
                {t('Add rule')}
              </Button>
            </div>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
