import { Toast } from '@heroui/react/toast'

/** Lazily mounted by `ToastMount`. */
export default function ToastHost() {
  return <Toast.Provider placement="top" maxVisibleToasts={3} />
}
