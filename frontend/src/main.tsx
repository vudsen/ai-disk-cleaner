import React from 'react'
import { createRoot } from 'react-dom/client'
import './globals.css'
import AppRoutes from './pages/route'
import DialogProvider from '@/components/DialogProvider'
import { Toast } from '@heroui/react'
import { toastQueue } from '@/util/toast'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
  <React.StrictMode>
    <DialogProvider />
    <AppRoutes />
    <Toast.Provider queue={toastQueue} />
  </React.StrictMode>,
)
