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
import { describe, expect, test } from 'vitest'

import { revenueTrendPoints } from '../lib'

const HOUR = 3600
const DAY = 86400
// 2027-01-15T00:00:00Z, a UTC midnight.
const MIDNIGHT = 1_799_971_200

describe('revenue trend buckets', () => {
  test('day buckets are filled at UTC midnights and keep the server buckets', () => {
    // Starts and ends mid-day UTC, so local-day alignment would differ.
    const range = {
      start: MIDNIGHT + 13 * HOUR,
      end: MIDNIGHT + 3 * DAY + 5 * HOUR,
    }
    const server = { bucket: MIDNIGHT + DAY, archive_bytes: 7, revenue: 3 }

    const points = revenueTrendPoints([server], range)

    expect(points.map((point) => point.bucket.bucket)).toEqual([
      MIDNIGHT,
      MIDNIGHT + DAY,
      MIDNIGHT + 2 * DAY,
      MIDNIGHT + 3 * DAY,
    ])
    expect(points.map((point) => point.bucket.revenue)).toEqual([0, 3, 0, 0])
    expect(points.map((point) => point.label)).toEqual([
      '2027-01-15',
      '2027-01-16',
      '2027-01-17',
      '2027-01-18',
    ])
  })

  test('the first and last day buckets are clamped to the searched range', () => {
    const range = {
      start: MIDNIGHT + 13 * HOUR,
      end: MIDNIGHT + 3 * DAY + 5 * HOUR,
    }

    const points = revenueTrendPoints([], range)

    expect(points[0]).toMatchObject({
      start: range.start,
      end: MIDNIGHT + DAY - 1,
    })
    expect(points.at(-1)).toMatchObject({
      start: MIDNIGHT + 3 * DAY,
      end: range.end,
    })
  })

  test('hour buckets are filled on UTC hours with zero revenue', () => {
    const range = { start: MIDNIGHT + 10 * 60, end: MIDNIGHT + 3 * HOUR + 59 }
    const server = { bucket: MIDNIGHT + 2 * HOUR, archive_bytes: 5, revenue: 1 }

    const points = revenueTrendPoints([server], range)

    expect(points.map((point) => point.bucket.bucket)).toEqual([
      MIDNIGHT,
      MIDNIGHT + HOUR,
      MIDNIGHT + 2 * HOUR,
      MIDNIGHT + 3 * HOUR,
    ])
    expect(points.map((point) => point.bucket.archive_bytes)).toEqual([
      0, 0, 5, 0,
    ])
  })

  test('an inverted range yields no points', () => {
    expect(
      revenueTrendPoints([], { start: MIDNIGHT, end: MIDNIGHT - 1 })
    ).toEqual([])
  })
})
