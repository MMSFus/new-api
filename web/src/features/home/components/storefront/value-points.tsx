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
import { Coins, Gauge, Layers } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export function ValuePoints() {
  const { t } = useTranslation()

  const points = [
    {
      icon: Coins,
      title: t('Low price'),
      desc: t(
        'Pay as you go. You are only charged for the tokens you actually use.'
      ),
    },
    {
      icon: Layers,
      title: t('One unified API'),
      desc: t(
        'The same key and base URL work for every model. Switch models by changing one parameter.'
      ),
    },
    {
      icon: Gauge,
      title: t('Clear usage'),
      desc: t(
        'Every request and its cost shows up in your usage log, so you always know where the balance went.'
      ),
    },
  ]

  return (
    <section
      aria-label={t('Why choose us')}
      className='border-border/60 border-y px-4 py-12 sm:px-6'
    >
      <ul className='mx-auto grid max-w-6xl grid-cols-1 gap-8 md:grid-cols-3'>
        {points.map((point) => (
          <li key={point.title} className='flex gap-4'>
            <span className='bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-lg'>
              <point.icon className='size-5' aria-hidden='true' />
            </span>
            <div className='min-w-0 space-y-1'>
              <h2 className='font-semibold'>{point.title}</h2>
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {point.desc}
              </p>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
