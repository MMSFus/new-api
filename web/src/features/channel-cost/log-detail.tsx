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
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import {
  DetailRow,
  DetailSection,
} from '@/features/usage-logs/components/dialogs/log-detail-layout'
import { formatQuotaDelta } from '@/features/usage-logs/lib/simulated-cache'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { LOG_TYPE_REFUND, type ChannelCostLog } from './api'
import {
  formatCostFormula,
  formatCostQuota,
  hasCurrencyFactor,
  profitTone,
} from './lib'

/** Every field of one cost log, including how its upstream cost was derived. */
export function ChannelCostLogDetail(props: { log: ChannelCostLog }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const log = props.log
  const cost = log.cost
  const sign = log.type === LOG_TYPE_REFUND ? -1 : 1
  const profit = cost ? sign * (log.quota - cost.cost_quota) : null
  let source = t('Recorded at request time')
  if (!cost) source = t('Uncosted')
  else if (cost.source === 'historical_backfill') {
    source = t('Historical backfill · Estimate')
  }

  return (
    <div className='grid gap-4 lg:grid-cols-2'>
      <DetailSection label={t('Request')}>
        <DetailRow
          label={t('Time')}
          value={dayjs.unix(log.created_at).format('YYYY-MM-DD HH:mm:ss')}
          mono
        />
        <DetailRow
          label={t('Type')}
          value={log.type === LOG_TYPE_REFUND ? t('Refund') : t('Consume')}
        />
        <DetailRow
          label={t('Request ID')}
          value={
            log.request_id ? (
              <span className='inline-flex items-center gap-1'>
                {log.request_id}
                <CopyButton value={log.request_id} size='icon' />
              </span>
            ) : (
              '-'
            )
          }
          mono
        />
        <DetailRow
          label={t('Username')}
          value={`${log.username || '-'} #${log.user_id}`}
        />
        <DetailRow
          label={t('Channel')}
          value={[log.channel_name, `#${log.channel_id}`]
            .filter(Boolean)
            .join(' ')}
        />
        <DetailRow label={t('Model')} value={log.model_name || '-'} mono />
        <DetailRow label={t('Group')} value={log.group || '-'} />
        <DetailRow
          label={t('Input Tokens')}
          value={formatNumber(log.prompt_tokens, locale)}
          mono
        />
        <DetailRow
          label={t('Output Tokens')}
          value={formatNumber(log.completion_tokens, locale)}
          mono
        />
      </DetailSection>
      <DetailSection label={t('Cost calculation')}>
        <DetailRow
          label={t('Revenue')}
          value={formatLogQuota(sign * log.quota)}
          mono
        />
        {cost ? (
          <>
            <DetailRow
              label={t('Official price')}
              value={formatCostQuota(cost.base_quota)}
              mono
            />
            <DetailRow
              label={t('Group ratio')}
              value={String(cost.group_ratio)}
              mono
            />
            <DetailRow
              label={t('Upstream cost ratio')}
              value={String(cost.upstream_ratio)}
              mono
            />
            {cost.cost_currency && (
              <DetailRow
                label={t('Upstream ratio currency')}
                value={cost.cost_currency}
                mono
              />
            )}
            {hasCurrencyFactor(cost) && (
              <DetailRow
                label={t('Currency factor')}
                value={String(cost.currency_factor)}
                mono
              />
            )}
            <DetailRow
              label={t('Upstream cost')}
              value={
                <span>
                  {formatCostQuota(sign * cost.cost_quota)}
                  <span className='text-muted-foreground ml-1'>
                    = {formatCostFormula(t, cost)}
                  </span>
                </span>
              }
              mono
            />
            <DetailRow
              label={t('Gross profit')}
              value={
                <span className={cn(profitTone(profit ?? 0))}>
                  {formatCostQuota(profit ?? 0)}
                </span>
              }
              mono
            />
          </>
        ) : (
          <p className='text-warning text-xs'>
            {t('Uncosted requests are excluded from profit and margin.')}
          </p>
        )}
        <DetailRow label={t('Cost source')} value={source} />
        {cost?.backfilled_at ? (
          <DetailRow
            label={t('Backfilled at')}
            value={dayjs.unix(cost.backfilled_at).format('YYYY-MM-DD HH:mm:ss')}
            mono
          />
        ) : null}
        {cost?.task_id ? (
          <DetailRow label={t('Task ID')} value={cost.task_id} mono />
        ) : null}
        <DetailRow
          label={t('Simulated cache difference')}
          value={formatQuotaDelta(log.simulated_cache_delta)}
          mono
        />
      </DetailSection>
    </div>
  )
}
