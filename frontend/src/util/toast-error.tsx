import { toast } from '@/util/toast'

export const toastError = (e: unknown, title: string) => {
  let msg: string
  if (e instanceof Error) {
    msg = e.message
  } else if (typeof e === 'string') {
    msg = e
  } else {
    msg = JSON.stringify(e)
  }
  console.log(msg)
  toast(title, {
    actionProps: {
      children: '关闭',
      onPress: () => toast.clear(),
      variant: 'tertiary',
    },
    description: <div className="break-all">{msg}</div>,
    variant: 'danger',
  })
}
