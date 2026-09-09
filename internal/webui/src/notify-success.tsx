import { createContext, useContext, type ReactNode } from 'react'
import { useToastManager } from '@appica/ui-react/toast'

// A toast that carries detail the reader may want to act on has to wait for
// them: pass `timeout: 0` to keep it up until dismissed.
export interface NotifyOptions {
  description?: ReactNode
  timeout?: number
}

type Notify = (title: string, options?: NotifyOptions) => void

const NotifySuccessContext = createContext<Notify>(() => {})

export function NotifySuccessBridge({ children }: { children: React.ReactNode }) {
  const toast = useToastManager()
  return (
    <NotifySuccessContext.Provider
      value={(title, options) => toast.add({ title, ...options })}
    >
      {children}
    </NotifySuccessContext.Provider>
  )
}

export function useNotifySuccess(): Notify {
  return useContext(NotifySuccessContext)
}
