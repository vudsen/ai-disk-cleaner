import {
  ToastQueue,
  type ToastContentValue,
  type toast as heroToast,
} from '@heroui/react'
import type { ReactNode } from 'react'

// Native WebViews can lose the page's composited surface during a document
// ViewTransition. Notifications only need to update their own region.
export const toastQueue = new ToastQueue<ToastContentValue>({
  wrapUpdate: (update) => update(),
})

type ToastOptions = Parameters<typeof heroToast>[1]

export const toast = Object.assign(
  (title: ReactNode, options?: ToastOptions) => {
    const { timeout, onClose, ...content } = options ?? {}
    return toastQueue.add({ title, ...content }, { timeout, onClose })
  },
  { clear: () => toastQueue.clear() },
)
