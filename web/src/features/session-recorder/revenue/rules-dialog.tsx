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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ListOrdered } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'

import { saveRevenueRules } from '../api'
import {
  REVENUE_RULES_QUERY_KEY,
  REVENUE_SUMMARY_QUERY_KEY,
  useRevenueRules,
} from '../use-session-recorder-data'
import { REVENUE_RULES_FORM_ID, RevenueRulesEditor } from './rules-editor'

/** Header button of the revenue tab that edits the rules (write access). */
export function RevenueRulesDialog(props: { siteSymbol: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const rulesQuery = useRevenueRules(open)
  const mutation = useMutation({
    mutationFn: saveRevenueRules,
    // The error is shown in the dialog next to the rules.
    meta: { errorToast: false },
    onSuccess: () => {
      toast.success(t('Revenue rules saved'))
      void queryClient.invalidateQueries({ queryKey: REVENUE_RULES_QUERY_KEY })
      void queryClient.invalidateQueries({
        queryKey: REVENUE_SUMMARY_QUERY_KEY,
      })
      setOpen(false)
    },
  })

  let body = <LoadingState />
  if (rulesQuery.isError) {
    body = (
      <ErrorState
        title={t('Failed to load revenue rules')}
        description={rulesQuery.error.message}
        onRetry={() => void rulesQuery.refetch()}
      />
    )
  } else if (rulesQuery.data) {
    body = (
      <RevenueRulesEditor
        initial={rulesQuery.data}
        siteSymbol={props.siteSymbol}
        saveError={mutation.error}
        onSave={(set) => mutation.mutate(set)}
      />
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (mutation.isPending) return
        setOpen(next)
        mutation.reset()
      }}
      title={t('Revenue rules')}
      description={t(
        'Rules match model names in order; the first enabled match prices the record. Unmatched records earn nothing.'
      )}
      contentClassName='sm:max-w-5xl'
      trigger={
        <Button type='button' variant='outline' size='sm'>
          <ListOrdered aria-hidden='true' />
          {t('Edit rules')}
        </Button>
      }
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            disabled={mutation.isPending}
            onClick={() => setOpen(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form={REVENUE_RULES_FORM_ID}
            disabled={!rulesQuery.data || mutation.isPending}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      {body}
    </Dialog>
  )
}
