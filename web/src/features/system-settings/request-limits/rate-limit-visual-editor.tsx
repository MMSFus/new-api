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
import { Plus, Search } from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { RateLimitDialog } from './rate-limit-dialog'
import {
  RATE_LIMIT_ANY_GROUP,
  parseRateLimitEntries,
  rateLimitEntryKey,
  removeRateLimitEntry,
  upsertRateLimitEntry,
  type RateLimitEntryData,
  type RateLimitMode,
} from './rate-limit-rules'

type RateLimitVisualEditorProps = {
  value: string
  onChange: (value: string) => void
  mode?: RateLimitMode
}

export function RateLimitVisualEditor({
  value,
  onChange,
  mode = 'legacy',
}: RateLimitVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<RateLimitEntryData | null>(null)

  const rateLimits = useMemo(
    () => parseRateLimitEntries(mode, value),
    [mode, value]
  )

  const filteredRateLimits = useMemo(() => {
    if (!searchText) return rateLimits
    const lowerSearch = searchText.toLowerCase()
    return rateLimits.filter(
      (limit) =>
        limit.groupName.toLowerCase().includes(lowerSearch) ||
        (limit.userGroup ?? '').toLowerCase().includes(lowerSearch)
    )
  }, [rateLimits, searchText])

  const existingKeys = useMemo(
    () => new Set(rateLimits.map(rateLimitEntryKey)),
    [rateLimits]
  )

  const handleSave = (data: RateLimitEntryData) => {
    onChange(upsertRateLimitEntry(mode, value, data, editData))
  }

  const handleDelete = (limit: RateLimitEntryData) => {
    onChange(removeRateLimitEntry(mode, value, limit))
  }

  const handleEdit = (limit: RateLimitEntryData) => {
    setEditData(limit)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setEditData(null)
    setDialogOpen(true)
  }

  const formatCount = (count: number) =>
    count === 0 ? t('Unlimited') : count.toLocaleString()

  const calledGroupLabel = (group: string) =>
    group === RATE_LIMIT_ANY_GROUP ? t('Any group (*)') : group

  const columns = [
    ...(mode === 'private'
      ? [
          {
            id: 'user-group',
            header: t('User group'),
            cellClassName: 'font-medium',
            cell: (limit: RateLimitEntryData) => limit.userGroup,
          },
        ]
      : []),
    {
      id: 'group',
      header: mode === 'legacy' ? t('Group Name') : t('Called group'),
      cellClassName: mode === 'private' ? undefined : 'font-medium',
      cell: (limit: RateLimitEntryData) =>
        mode === 'legacy' ? limit.groupName : calledGroupLabel(limit.groupName),
    },
    {
      id: 'max-requests',
      header: t('Max Requests (incl. failures)'),
      className: 'text-right',
      cellClassName: 'text-right',
      cell: (limit: RateLimitEntryData) => (
        <span className='font-mono'>{formatCount(limit.maxRequests)}</span>
      ),
    },
    {
      id: 'max-success',
      header: t('Max Success'),
      className: 'text-right',
      cellClassName: 'text-right',
      cell: (limit: RateLimitEntryData) => (
        <span className='font-mono'>
          {mode === 'legacy'
            ? limit.maxSuccess.toLocaleString()
            : formatCount(limit.maxSuccess)}
        </span>
      ),
    },
    ...(mode === 'legacy'
      ? []
      : [
          {
            id: 'duration',
            header: t('Limit period'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (limit: RateLimitEntryData) => (
              <span className='font-mono'>
                {limit.durationMinutes
                  ? t('{{minutes}} min', { minutes: limit.durationMinutes })
                  : t('Default')}
              </span>
            ),
          },
        ]),
    {
      id: 'actions',
      header: t('Actions'),
      className: 'text-right',
      cellClassName: 'text-right',
      cell: (limit: RateLimitEntryData) => (
        <StaticRowActions
          editLabel={t('Edit')}
          deleteLabel={t('Delete')}
          menuLabel={t('Open menu')}
          onEdit={() => handleEdit(limit)}
          onDelete={() => handleDelete(limit)}
        />
      ),
    },
  ]

  let emptyContent = t('No rules configured. Click "Add rule" to get started.')
  if (searchText) {
    emptyContent = t('No groups match your search')
  } else if (mode === 'legacy') {
    emptyContent = t(
      'No group-based rate limits configured. Click "Add group" to get started.'
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-4'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search group names...')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className='pl-9'
          />
        </div>
        <Button type='button' onClick={handleAdd}>
          <Plus className='mr-2 h-4 w-4' />
          {mode === 'legacy' ? t('Add group') : t('Add rule')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredRateLimits}
        getRowKey={rateLimitEntryKey}
        emptyContent={emptyContent}
        columns={columns}
      />

      <RateLimitDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        mode={mode}
        existingKeys={existingKeys}
      />
    </div>
  )
}
