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
import { useMutation } from '@tanstack/react-query'
import { FlaskConical } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { translateServerText } from '@/lib/server-error-message'

import { testRevenueRules, type RevenueRule } from '../api'

/**
 * "Test model name": asks the server which of the unsaved draft rules a model
 * name would match, using the same matcher as the revenue summary.
 */
export function RevenueRuleTester(props: { getRules: () => RevenueRule[] }) {
  const { t } = useTranslation()
  const inputId = useId()
  const [model, setModel] = useState('')
  const mutation = useMutation({
    mutationFn: testRevenueRules,
    meta: { errorToast: false },
  })
  const result = mutation.data

  let status = null
  if (mutation.isError) {
    status = (
      <p role='alert' className='text-destructive text-sm'>
        {translateServerText(t, mutation.error.message)}
      </p>
    )
  } else if (result && result.matched_rule_id) {
    status = (
      <p role='status' className='text-sm'>
        {t('Matches rule: {{name}}', {
          name: result.matched_rule_name || result.matched_rule_id,
        })}
      </p>
    )
  } else if (result) {
    status = (
      <p role='status' className='text-muted-foreground text-sm'>
        {t('No rule matches; the record counts as unmatched.')}
      </p>
    )
  }

  return (
    <section className='bg-muted/30 space-y-2 rounded-lg border p-3'>
      <Label htmlFor={inputId}>{t('Test model name')}</Label>
      <div className='flex gap-2'>
        <Input
          id={inputId}
          value={model}
          placeholder='claude-opus-4-5'
          className='font-mono'
          onChange={(event) => {
            setModel(event.target.value)
            mutation.reset()
          }}
          onKeyDown={(event) => {
            if (event.key !== 'Enter') return
            event.preventDefault()
            if (model.trim()) {
              mutation.mutate({ rules: props.getRules(), model: model.trim() })
            }
          }}
        />
        <Button
          type='button'
          variant='outline'
          disabled={!model.trim() || mutation.isPending}
          onClick={() =>
            mutation.mutate({ rules: props.getRules(), model: model.trim() })
          }
        >
          <FlaskConical aria-hidden='true' />
          {t('Test')}
        </Button>
      </div>
      {status}
    </section>
  )
}
