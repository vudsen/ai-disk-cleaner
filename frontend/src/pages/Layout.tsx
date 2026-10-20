import { Button } from '@heroui/react'
import {
  HardDrive,
  Home,
  Settings,
  Waypoints,
  type LucideIcon,
} from 'lucide-react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useEffect, useMemo } from 'react'
import { showDialog } from '@/components/DialogProvider'
import {
  GetPlatformInfo,
  IsRunningAsAdministrator,
} from '../../wailsjs/go/main/App'
import { useTranslation } from 'react-i18next'

let administratorWarningChecked = false

const hasWailsRuntime = () =>
  typeof window !== 'undefined' && 'go' in window && 'runtime' in window

type NavigationItem = {
  label: string
  path: string
  icon: LucideIcon
}

export default function Layout() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    if (administratorWarningChecked || !hasWailsRuntime()) {
      return
    }
    administratorWarningChecked = true

    void GetPlatformInfo()
      .then(async (platformInfo) => {
        if (platformInfo.os !== 'windows') {
          return
        }
        const isAdministrator = await IsRunningAsAdministrator()
        if (isAdministrator) {
          return
        }
        showDialog({
          title: t('route.adminPermissionRequired'),
          message: (
            <div className="space-y-2 text-sm">
              <p>{t('route.adminPermissionRequiredDesc')}</p>
              <p>{t('route.adminPermissionRequiredDesc1')}</p>
            </div>
          ),
          hideCancel: true,
        })
      })
      .catch(() => {
        // The warning is advisory; a failed check must not block application use.
      })
  }, [])

  const navigation: NavigationItem[] = useMemo(
    () => [
      {
        label: t('route.home'),
        path: '/home',
        icon: Home,
      },
      {
        label: t('route.cleanupRecord'),
        path: '/cleanup',
        icon: HardDrive,
      },
      {
        label: t('route.migration'),
        path: '/migration',
        icon: Waypoints,
      },
      {
        label: t('route.settings'),
        path: '/settings',
        icon: Settings,
      },
    ],
    [t],
  )

  const pageDetails: Record<
    string,
    {
      title: string
      description: string
    }
  > = useMemo(
    () => ({
      '/home': {
        title: t('route.home'),
        description: t('route.homeDesc'),
      },
      '/cleanup': {
        title: t('route.cleanupRecord'),
        description: t('route.cleanupRecordDesc'),
      },
      '/migration': {
        title: t('route.migration'),
        description: t('route.migrationDesc'),
      },
      '/settings': {
        title: t('route.settings'),
        description: t('route.settingsDesc'),
      },
    }),
    [t],
  )

  const currentPage = location.pathname.startsWith('/cleanup/')
    ? {
        title: t('route.cleanupDetail'),
        description: t('route.cleanupDetailDesc'),
      }
    : (pageDetails[location.pathname] ?? pageDetails['/home'])

  return (
    <div className="text-foreground min-h-screen bg-white text-left">
      <div className="border-default mx-auto flex h-screen overflow-hidden border shadow-sm">
        <aside className="border-default flex w-52 shrink-0 flex-col border-r px-5 py-7">
          <div className="flex h-12 items-center gap-3 px-3">
            <span className="bg-accent text-accent-foreground flex size-9 items-center justify-center rounded-xl shadow-sm">
              <img src="/broom.svg" width={18} alt="logo" />
            </span>
            <div>
              <p className="text-base font-semibold tracking-tight">
                AIDiskCleaner
              </p>
              <p className="text-muted text-xs">AI Disk Cleaner</p>
            </div>
          </div>

          <p className="text-muted mt-10 mb-3 px-3 text-xs font-semibold tracking-[0.14em]">
            {t('route.workbench')}
          </p>

          <nav aria-label="主导航" className="flex flex-col gap-2">
            {navigation.map((item) => {
              const Icon = item.icon
              const isActive =
                location.pathname === item.path ||
                (item.path === '/cleanup' &&
                  location.pathname.startsWith('/cleanup/'))

              return (
                <Button
                  key={item.path}
                  className={`h-12 w-full justify-start gap-3 rounded-lg px-4 text-sm font-semibold transition-colors ${
                    isActive ? 'bg-default/40 text-foreground' : 'text-muted'
                  }`}
                  variant="ghost"
                  onPress={() => navigate(item.path)}
                >
                  <Icon aria-hidden="true" size={18} strokeWidth={1.8} />
                  {item.label}
                </Button>
              )
            })}
          </nav>
        </aside>

        <main className="min-w-0 flex-1 overflow-y-auto">
          <header className="flex min-h-36 items-end justify-between gap-8 px-10 pt-8 pb-7">
            <div>
              <p className="text-accent mb-2 text-xs font-semibold tracking-[0.14em]">
                AIDISKCLEANER
              </p>
              <h1 className="text-foreground text-3xl font-semibold tracking-tight">
                {currentPage.title}
              </h1>
              <p className="text-muted mt-2 text-sm">
                {currentPage.description}
              </p>
            </div>
          </header>

          <div className="px-10 pb-10">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
