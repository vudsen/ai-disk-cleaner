import { AlertDialog, Button, Spinner, useOverlayState } from '@heroui/react'

import React, { useImperativeHandle, useState } from 'react'
import { isPromise } from '@/util/common'
import i18n from 'i18next'

export type DialogConfig = {
  title: React.ReactNode
  message?: React.ReactNode
  confirmBtnText?: string
  cancelBtnText?: string
  onConfirm?: () => Promise<unknown> | void
  onCancel?: () => void
  color?: 'accent' | 'danger' | 'warning'
  hideCancel?: boolean
  isDismissable?: boolean
}

export interface ConfirmDialogRef {
  showDialog: (config: DialogConfig) => void
}

interface ConfirmDialogProps {
  ref: React.RefObject<ConfirmDialogRef | null>
}

const ConfirmDialog: React.FC<ConfirmDialogProps> = (props) => {
  const [config, setConfig] = useState<DialogConfig>({
    title: '',
    cancelBtnText: i18n.t('common.cancel'),
    confirmBtnText: i18n.t('common.confirm'),
  })
  const [isLoading, setLoading] = useState(false)
  const { isOpen, setOpen, open, close } = useOverlayState()

  const onConfirm = () => {
    const val = config.onConfirm?.()
    if (isPromise(val)) {
      setLoading(true)
      val.finally(() => {
        setLoading(false)
        close()
      })
    } else {
      close()
    }
  }

  useImperativeHandle(props.ref, () => ({
    showDialog(config) {
      setConfig({
        cancelBtnText: i18n.t('common.cancel'),
        confirmBtnText: i18n.t('common.confirm'),
        hideCancel: false,
        ...config,
      })
      open()
    },
  }))

  return (
    <AlertDialog.Backdrop isOpen={isOpen} onOpenChange={setOpen}>
      <AlertDialog.Container>
        <AlertDialog.Dialog className="sm:max-w-100">
          <AlertDialog.CloseTrigger />
          <AlertDialog.Header>
            <AlertDialog.Icon status={config.color} />
            <AlertDialog.Heading>{config.title}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>{config.message}</AlertDialog.Body>
          <AlertDialog.Footer>
            {!config.hideCancel && (
              <Button
                slot="close"
                variant="ghost"
                onPress={close}
                isDisabled={isLoading}
              >
                {config.cancelBtnText}
              </Button>
            )}
            <Button
              onPress={onConfirm}
              isPending={isLoading}
              variant={config.color === 'danger' ? 'danger' : 'tertiary'}
            >
              {({ isPending }) => (
                <>
                  {isPending ? <Spinner color="current" size="sm" /> : null}
                  {config.confirmBtnText}
                </>
              )}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}

export default ConfirmDialog
