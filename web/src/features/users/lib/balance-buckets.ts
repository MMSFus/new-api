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
import type { BalanceBucketKey, User, UserBalanceBuckets } from '../types'

export const BALANCE_BUCKET_OPTIONS: {
  key: BalanceBucketKey
  labelKey: string
}[] = [
  { key: 'topup', labelKey: 'Top-up Balance' },
  { key: 'aff_rebate', labelKey: 'Referral Cashback' },
  { key: 'invite_bonus', labelKey: 'Invite Reward' },
  { key: 'gift', labelKey: 'Gift Balance' },
]

/** Extracts the balance buckets from an admin user payload. */
export function toUserBalanceBuckets(user: User): UserBalanceBuckets {
  return {
    topup: user.quota_topup ?? 0,
    aff_rebate: user.quota_aff_rebate ?? 0,
    invite_bonus: user.quota_invite_bonus ?? 0,
    gift: user.quota_gift ?? 0,
    debt: user.quota_debt ?? 0,
  }
}
