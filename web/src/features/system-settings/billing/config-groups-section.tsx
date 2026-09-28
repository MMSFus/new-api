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
import { Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Reorder } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import { EmptyState } from '@/components/empty-state'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  MAX_CONFIG_GROUPS,
  createConfigGroupDraft,
  mergeGroupSelection,
  parseConfigGroupsOption,
  parseGroupRatioNames,
  serializeConfigGroups,
  validateConfigGroups,
  type ConfigGroupDraft,
  type ConfigGroupIssue,
} from './config-groups'

export const CONFIG_GROUPS_OPTION_KEY = 'ConfigGroups'

type ConfigGroupsSectionProps = {
  defaultValue: string
  groupRatio: string
  userUsableGroups: string
}

export function ConfigGroupsSection(props: ConfigGroupsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [drafts, setDrafts] = useState<ConfigGroupDraft[]>(() =>
    parseConfigGroupsOption(props.defaultValue)
  )
  const [showIssues, setShowIssues] = useState(false)

  useEffect(() => {
    setDrafts(parseConfigGroupsOption(props.defaultValue))
    setShowIssues(false)
  }, [props.defaultValue])

  const knownGroups = useMemo(
    () => parseGroupRatioNames(props.groupRatio),
    [props.groupRatio]
  )
  const groupOptions = useMemo(
    () =>
      knownGroups
        .filter((group) => group !== 'auto')
        .map((group) => ({ value: group, label: group })),
    [knownGroups]
  )
  const userGroupOptions = useMemo(() => {
    const names = new Set([
      ...knownGroups,
      ...parseGroupRatioNames(props.userUsableGroups),
    ])
    names.delete('auto')
    return [...names].map((group) => ({ value: group, label: group }))
  }, [knownGroups, props.userUsableGroups])

  const baseline = useMemo(
    () => serializeConfigGroups(parseConfigGroupsOption(props.defaultValue)),
    [props.defaultValue]
  )
  const serialized = serializeConfigGroups(drafts)
  const isDirty = serialized !== baseline
  const issues = useMemo(
    () => validateConfigGroups(drafts, knownGroups),
    [drafts, knownGroups]
  )

  const updateDraft = (id: string, patch: Partial<ConfigGroupDraft>) => {
    setDrafts((current) =>
      current.map((draft) => (draft.id === id ? { ...draft, ...patch } : draft))
    )
  }

  const handleSave = async () => {
    if (issues.length > 0) {
      setShowIssues(true)
      const first = issues[0]
      toast.error(
        t('Config group #{{index}}: {{message}}', {
          index: first.index + 1,
          message: t(first.message, first.params),
        })
      )
      return
    }
    await updateOption.mutateAsync({
      key: CONFIG_GROUPS_OPTION_KEY,
      value: serialized,
    })
  }

  return (
    <SettingsSection title={t('Config Groups')}>
      <SettingsPageFormActions
        onSave={handleSave}
        onReset={() => {
          setDrafts(parseConfigGroupsOption(props.defaultValue))
          setShowIssues(false)
        }}
        isSaving={updateOption.isPending}
        isSaveDisabled={!isDirty}
        isResetDisabled={!isDirty}
        saveLabel='Save config groups'
      />
      <div className='flex flex-col gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Config groups are named Auto routing plans. Users can pick one for an API key; requests try its groups in order and bill at the group that serves them. Tokens that use a deleted plan are rejected until they pick another group.'
          )}
        </p>

        {drafts.length === 0 && (
          <EmptyState
            className='min-h-40'
            title={t('No config groups')}
            description={t(
              'Add a config group to offer users a preset group order.'
            )}
          />
        )}

        {drafts.map((draft, index) => (
          <ConfigGroupCard
            key={draft.id}
            draft={draft}
            index={index}
            groupOptions={groupOptions}
            userGroupOptions={userGroupOptions}
            knownGroups={knownGroups}
            issues={
              showIssues ? issues.filter((issue) => issue.index === index) : []
            }
            onChange={(patch) => updateDraft(draft.id, patch)}
            onRemove={() =>
              setDrafts((current) =>
                current.filter((item) => item.id !== draft.id)
              )
            }
          />
        ))}

        <Button
          type='button'
          variant='outline'
          className='self-start'
          disabled={drafts.length >= MAX_CONFIG_GROUPS}
          onClick={() =>
            setDrafts((current) => [...current, createConfigGroupDraft()])
          }
        >
          {t('Add config group')}
        </Button>
      </div>
    </SettingsSection>
  )
}

type ConfigGroupCardProps = {
  draft: ConfigGroupDraft
  index: number
  groupOptions: Array<{ value: string; label: string }>
  userGroupOptions: Array<{ value: string; label: string }>
  knownGroups: string[]
  issues: ConfigGroupIssue[]
  onChange: (patch: Partial<ConfigGroupDraft>) => void
  onRemove: () => void
}

function ConfigGroupCard(props: ConfigGroupCardProps) {
  const { t } = useTranslation()
  const fieldId = `config-group-${props.draft.id}`
  const issueFor = (field: ConfigGroupIssue['field']) => {
    const issue = props.issues.find((item) => item.field === field)
    return issue ? t(issue.message, issue.params) : undefined
  }
  const keyIssue = issueFor('key')
  const nameIssue = issueFor('name')
  const descriptionIssue = issueFor('description')
  const groupsIssue = issueFor('groups')

  const handleMove = (index: number, direction: 'up' | 'down') => {
    const targetIndex = direction === 'up' ? index - 1 : index + 1
    if (targetIndex < 0 || targetIndex >= props.draft.groups.length) return
    const next = [...props.draft.groups]
    ;[next[index], next[targetIndex]] = [next[targetIndex], next[index]]
    props.onChange({ groups: next })
  }

  return (
    <section
      data-slot='config-group-card'
      aria-label={t('Config group #{{index}}', { index: props.index + 1 })}
      className='bg-muted/20 flex flex-col gap-4 rounded-lg border p-4'
    >
      <div className='flex items-center justify-between gap-2'>
        <h4 className='text-sm font-semibold'>
          {props.draft.name.trim() ||
            props.draft.key.trim() ||
            t('Config group #{{index}}', { index: props.index + 1 })}
        </h4>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          aria-label={t('Delete config group')}
          title={t('Delete config group')}
          onClick={props.onRemove}
        >
          <HugeiconsIcon
            icon={Delete02Icon}
            strokeWidth={2}
            aria-hidden='true'
          />
        </Button>
      </div>

      <div className='grid gap-4 sm:grid-cols-2'>
        <div className='flex flex-col gap-1.5'>
          <Label htmlFor={`${fieldId}-key`}>{t('Plan key')}</Label>
          <Input
            id={`${fieldId}-key`}
            value={props.draft.key}
            aria-invalid={keyIssue ? true : undefined}
            placeholder='claude-best'
            onChange={(event) => props.onChange({ key: event.target.value })}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Stable identifier stored in API keys. Changing it detaches existing keys.'
            )}
          </p>
          {keyIssue && <p className='text-destructive text-xs'>{keyIssue}</p>}
        </div>
        <div className='flex flex-col gap-1.5'>
          <Label htmlFor={`${fieldId}-name`}>{t('Display name')}</Label>
          <Input
            id={`${fieldId}-name`}
            value={props.draft.name}
            aria-invalid={nameIssue ? true : undefined}
            onChange={(event) => props.onChange({ name: event.target.value })}
          />
          {nameIssue && <p className='text-destructive text-xs'>{nameIssue}</p>}
        </div>
      </div>

      <div className='flex flex-col gap-1.5'>
        <Label htmlFor={`${fieldId}-description`}>{t('Description')}</Label>
        <Input
          id={`${fieldId}-description`}
          value={props.draft.description}
          aria-invalid={descriptionIssue ? true : undefined}
          onChange={(event) =>
            props.onChange({ description: event.target.value })
          }
        />
        {descriptionIssue && (
          <p className='text-destructive text-xs'>{descriptionIssue}</p>
        )}
      </div>

      <div className='flex flex-col gap-2'>
        <Label htmlFor={`${fieldId}-groups`}>{t('Group order')}</Label>
        <MultiSelect
          id={`${fieldId}-groups`}
          options={props.groupOptions}
          selected={props.draft.groups}
          onChange={(values) =>
            props.onChange({
              groups: mergeGroupSelection(props.draft.groups, values),
            })
          }
          placeholder={t('Add groups')}
          renderSelectedSummary={(values) =>
            t('{{count}} groups selected', { count: values.length })
          }
        />
        {props.draft.groups.length > 0 && (
          <Reorder.Group
            as='ol'
            axis='y'
            values={props.draft.groups}
            onReorder={(groups) => props.onChange({ groups })}
            aria-label={t('Group order')}
            className='flex flex-col gap-2'
          >
            {props.draft.groups.map((group, index) => (
              <AutoGroupOrderItem
                key={group}
                group={group}
                index={index}
                count={props.draft.groups.length}
                onMove={handleMove}
                onRemove={(removed) =>
                  props.onChange({
                    groups: props.draft.groups.filter(
                      (item) => item !== removed
                    ),
                  })
                }
                leading={
                  <span className='text-muted-foreground w-5 shrink-0 text-center text-sm tabular-nums'>
                    {index + 1}
                  </span>
                }
              >
                {!props.knownGroups.includes(group) && (
                  <span className='text-destructive text-xs'>
                    {t('Unknown group')}
                  </span>
                )}
              </AutoGroupOrderItem>
            ))}
          </Reorder.Group>
        )}
        {groupsIssue && (
          <p className='text-destructive text-xs'>{groupsIssue}</p>
        )}
      </div>

      <div className='flex flex-col gap-1.5'>
        <Label htmlFor={`${fieldId}-user-groups`}>
          {t('Allowed user groups')}
        </Label>
        <MultiSelect
          id={`${fieldId}-user-groups`}
          options={props.userGroupOptions}
          selected={props.draft.user_groups}
          onChange={(values) => props.onChange({ user_groups: values })}
          placeholder={t('All user groups')}
          allowCreate
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Leave empty to offer this plan to every user group. Groups a user cannot use are skipped.'
          )}
        </p>
      </div>

      <div className='flex items-center justify-between gap-4'>
        <div className='flex flex-col gap-0.5'>
          <Label htmlFor={`${fieldId}-retry`}>
            {t('Cross-group retry by default')}
          </Label>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Preselected when a user picks this plan; users can still change it per key.'
            )}
          </p>
        </div>
        <Switch
          id={`${fieldId}-retry`}
          checked={props.draft.cross_group_retry}
          onCheckedChange={(checked) =>
            props.onChange({ cross_group_retry: checked })
          }
        />
      </div>
    </section>
  )
}
