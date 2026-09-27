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
import { MoreHorizontal, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { handleServerError } from '@/lib/handle-server-error'

import { deleteInvalidRedemptions } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { useRedemptions } from './redemptions-provider'

export function RedemptionsPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, triggerRefresh } = useRedemptions()
  const [showDeleteInvalidConfirm, setShowDeleteInvalidConfirm] =
    useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDeleteInvalid = async () => {
    setIsDeleting(true)
    try {
      const result = await deleteInvalidRedemptions()
      if (result.success) {
        const count = result.data || 0
        toast.success(
          t('Successfully deleted {{count}} invalid redemption codes', {
            count,
          })
        )
        triggerRefresh()
        setShowDeleteInvalidConfirm(false)
      } else {
        handleServerError(result, t(ERROR_MESSAGES.DELETE_INVALID_FAILED))
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <div className='flex items-center gap-2'>
        <Button size='sm' onClick={() => setOpen('create')}>
          <Plus className='h-4 w-4' />
          {t('Create Code')}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant='outline'
                size='sm'
                aria-label={t('More actions')}
              />
            }
          >
            <MoreHorizontal className='h-4 w-4' />
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-52'>
            <DropdownMenuItem
              onSelect={(e) => {
                e.preventDefault()
                setShowDeleteInvalidConfirm(true)
              }}
              className='text-destructive focus:text-destructive'
            >
              {t('Delete Invalid')}
              <DropdownMenuShortcut>
                <Trash2 className='h-4 w-4' />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <ConfirmDialog
        destructive
        open={showDeleteInvalidConfirm}
        onOpenChange={setShowDeleteInvalidConfirm}
        handleConfirm={handleDeleteInvalid}
        isLoading={isDeleting}
        className='max-w-md'
        title={t('Delete Invalid Redemption Codes?')}
        desc={
          <>
            {t('This will delete all')} <strong>{t('used')}</strong>,{' '}
            <strong>{t('disabled')}</strong>
            {t(', and')} <strong>{t('expired')}</strong>{' '}
            {t('redemption codes.')}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Invalid')}
      />
    </>
  )
}
