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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { PaymentMethod } from '../../types'
import { getPaymentIcon } from '../../lib'

interface QrCodePayDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  qrCodeData: string
  paymentMethod: PaymentMethod | undefined
  topupAmount: number
  paymentAmount: number
}

export function QrCodePayDialog({
  open,
  onOpenChange,
  qrCodeData,
  paymentMethod,
  topupAmount,
  paymentAmount,
}: QrCodePayDialogProps) {
  const { t } = useTranslation()
  const [countdown, setCountdown] = useState(300)

  useEffect(() => {
    if (!open) return
    setCountdown(300)
    const timer = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          clearInterval(timer)
          onOpenChange(false)
          return 0
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(timer)
  }, [open, onOpenChange])

  const minutes = Math.floor(countdown / 60)
  const seconds = countdown % 60

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2 text-xl font-semibold'>
            {getPaymentIcon(
              paymentMethod?.type,
              'h-5 w-5',
              paymentMethod?.icon,
              paymentMethod?.name,
            )}
            {paymentMethod?.name || t('Scan to Pay')}
          </DialogTitle>
        </DialogHeader>

        <div className='flex flex-col items-center gap-4 py-4'>
          <div className='rounded-lg border bg-white p-3'>
            <QRCodeSVG
              value={qrCodeData}
              size={220}
              level='M'
              includeMargin
              bgColor='#ffffff'
              fgColor='#000000'
            />
          </div>

          <p className='text-muted-foreground text-center text-sm'>
            {t(
              'Please scan the QR code with your mobile app to complete payment.',
            )}
          </p>

          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-sm'>{t('Expires in')}</span>
            <span
              className={`font-mono font-semibold ${countdown <= 30 ? 'text-red-500' : ''}`}
            >
              {minutes}:{seconds.toString().padStart(2, '0')}
            </span>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
