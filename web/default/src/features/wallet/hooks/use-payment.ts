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
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestWechatPayPayment,
  requestAlipayPcWebPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isAlipayPcWebPayment,
  isStripePayment,
  isWechatPayPayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'

// ============================================================================
// Payment Hook
// ============================================================================

function getStringField(data: unknown, key: string): string | undefined {
  if (!data || typeof data !== 'object' || !(key in data)) {
    return undefined
  }
  const value = (data as Record<string, unknown>)[key]
  return typeof value === 'string' && value ? value : undefined
}

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)

        const isStripe = isStripePayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)
        const response = isStripe
          ? await calculateStripeAmount({ amount: topupAmount })
          : isPancake
            ? await calculateWaffoPancakeAmount({ amount: topupAmount })
            : await calculateAmount({ amount: topupAmount })

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = parseFloat(response.data)
          setAmount(calculatedAmount)
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        return 0
      } catch (_error) {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
    ): Promise<{ success: boolean; qrCodeData?: string }> => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const isWechatPay = isWechatPayPayment(paymentType)
        const isAlipayPcWeb = isAlipayPcWebPayment(paymentType)
        const amount = Math.floor(topupAmount)

        let response
        if (isStripe) {
          response = await requestStripePayment({
            amount,
            payment_method: 'stripe',
          })
        } else if (isWechatPay) {
          response = await requestWechatPayPayment({
            amount,
            payment_method: paymentType,
          })
        } else if (isAlipayPcWeb) {
          response = await requestAlipayPcWebPayment({
            amount,
            payment_method: paymentType,
          })
        } else {
          response = await requestPayment({
            amount,
            payment_method: paymentType,
          })
        }

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return { success: false }
        }

        // Handle Stripe payment
        const stripePayLink = getStringField(response.data, 'pay_link')
        if (isStripe && stripePayLink) {
          window.open(stripePayLink, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return { success: true }
        }

        const alipayPayUrl = getStringField(response.data, 'pay_url')
        if (isAlipayPcWeb && alipayPayUrl) {
          window.open(alipayPayUrl, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return { success: true }
        }

        // Handle native QR-code payments (WeChat Pay)
        if (isWechatPay && response.data) {
          const qrCodeData =
            'code_url' in response.data
              ? (response.data.code_url as string | undefined)
              : 'qr_code' in response.data
                ? (response.data.qr_code as string | undefined)
                : undefined
          if (qrCodeData) {
            return { success: true, qrCodeData }
          }
        }

        // Handle generic form-submit payment
        if (!isStripe && !isWechatPay && !isAlipayPcWeb && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return { success: true }
          }
        }

        return { success: false }
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return { success: false }
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
