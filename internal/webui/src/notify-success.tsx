import { createContext, useContext } from 'react'
import { useToastManager } from '@appica/ui-react/toast'

const NotifySuccessContext = createContext<(title: string) => void>(() => {})

export function NotifySuccessBridge({ children }: { children: React.ReactNode }) {
  const toast = useToastManager()
  return (
    <NotifySuccessContext.Provider value={(title) => toast.add({ title })}>
      {children}
    </NotifySuccessContext.Provider>
  )
}

export function useNotifySuccess(): (title: string) => void {
  return useContext(NotifySuccessContext)
}
