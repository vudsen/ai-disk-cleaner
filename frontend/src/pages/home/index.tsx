import {
  Alert,
  Button,
  ButtonGroup,
  Card,
  Chip,
  Dropdown,
  Link,
  ListBox,
  ProgressBar,
  Select,
} from '@heroui/react'
import {
  ArrowUpRight,
  CheckCircle2,
  ChevronDown,
  Clock3,
  FolderOpen,
  HardDrive,
  LoaderCircle,
  ScanSearch,
  Sparkles,
  Square,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useNavigate } from 'react-router'
import ControlledNextUIFormWrapper from '@/components/ControlledNextUIFormWrapper'
import { showDialog } from '@/components/DialogProvider'
import useCleaningTask, {
  formatBytes,
  formatDate,
  stateLabel,
} from '@/hooks/useCleaningTask'
import {
  GetDisks,
  ListSettings,
  SelectDirectory,
  StartCleaning,
} from '../../../wailsjs/go/main/App'
import type { model, setting } from '../../../wailsjs/go/models'
import clsx from 'clsx'
import { useTranslation } from 'react-i18next'
import { toastError } from '@/util/toast-error'

type ScanValues = {
  target: string
  directory: string
}

type ScanMode = 'fast' | 'deep'

const hasWailsRuntime = () =>
  typeof window !== 'undefined' && 'go' in window && 'runtime' in window

const requiredLLMSettingKeys = ['llm.secret', 'llm.url', 'llm.model'] as const

function hasCompleteLLMConfiguration(settings: setting.Setting[]) {
  const values = new Map(settings.map((item) => [item.key, item.value.trim()]))
  return requiredLLMSettingKeys.every((key) => Boolean(values.get(key)))
}

export default function HomePage() {
  const { i18n, t } = useTranslation()
  const navigate = useNavigate()
  const { clearErrors, control, getValues, setError, trigger } =
    useForm<ScanValues>({
      defaultValues: {
        target: 'C:\\',
        directory: '',
      },
    })
  const scanTarget = useWatch({ control, name: 'target' })
  const [isSelectingDirectory, setIsSelectingDirectory] = useState(false)
  const [isCheckingLLM, setIsCheckingLLM] = useState(false)
  const [isLLMConfigured, setIsLLMConfigured] = useState<boolean | null>(null)
  const [isStarting, setIsStarting] = useState(false)
  const [scanMode, setScanMode] = useState<ScanMode>('fast')
  const [startError, setStartError] = useState('')
  const [disks, setDisks] = useState<model.DiskInfo[]>([])
  const [diskError, setDiskError] = useState('')
  const [isLoadingDisks, setIsLoadingDisks] = useState(true)
  const { error, isLoading, isStopping, records, stop, task } =
    useCleaningTask()
  const isTaskRunning =
    task?.state === 'SCANNING' || task?.state === 'ANALYZING'
  const systemDisk = useMemo(
    () => disks.find((disk) => disk.path.toUpperCase() === 'C:\\'),
    [disks],
  )
  const usedPercent =
    systemDisk && systemDisk.total > 0
      ? Math.round((systemDisk.used / systemDisk.total) * 100)
      : 0

  useEffect(() => {
    if (!hasWailsRuntime()) {
      setIsLoadingDisks(false)
      return
    }

    let mounted = true
    void GetDisks()
      .then((result) => {
        if (mounted) {
          setDisks(result ?? [])
        }
      })
      .catch((reason: unknown) => {
        if (mounted) {
          setDiskError(
            reason instanceof Error ? reason.message : String(reason),
          )
        }
      })
      .finally(() => {
        if (mounted) {
          setIsLoadingDisks(false)
        }
      })
    return () => {
      mounted = false
    }
  }, [])

  useEffect(() => {
    if (!hasWailsRuntime()) {
      return
    }

    let mounted = true
    void ListSettings()
      .then((settings) => {
        if (mounted) {
          setIsLLMConfigured(hasCompleteLLMConfiguration(settings ?? []))
        }
      })
      .catch((reason: unknown) => {
        if (mounted) {
          toastError(reason, t('home.llmConfigCheckFailed'))
        }
      })
    return () => {
      mounted = false
    }
  }, [t])

  const handleSelectDirectory = async (onChange: (path: string) => void) => {
    setIsSelectingDirectory(true)

    try {
      const directory = await SelectDirectory()

      if (directory) {
        onChange(directory)
        clearErrors('directory')
      }
    } catch {
      setError('directory', {
        message: t('home.openDirectorySelectorFailed'),
      })
    } finally {
      setIsSelectingDirectory(false)
    }
  }

  const handleStartCleaning = async () => {
    if (hasWailsRuntime()) {
      setIsCheckingLLM(true)
      try {
        const configured = hasCompleteLLMConfiguration(
          (await ListSettings()) ?? [],
        )
        setIsLLMConfigured(configured)
        if (!configured) {
          showDialog({
            title: t('home.llmNotConfiguredDialogTitle'),
            message: t('home.llmNotConfiguredDialogMessage'),
            confirmBtnText: t('home.goToSettings'),
            onConfirm: () => navigate('/settings'),
          })
          return
        }
      } catch (reason: unknown) {
        toastError(reason, t('home.llmConfigCheckFailed'))
        return
      } finally {
        setIsCheckingLLM(false)
      }
    }

    const fieldsToValidate: (keyof ScanValues)[] =
      scanTarget === 'directory' ? ['target', 'directory'] : ['target']

    if (!(await trigger(fieldsToValidate))) {
      return
    }

    const values = getValues()
    const path =
      values.target === 'directory'
        ? values.directory
        : disks.find((disk) => disk.path === values.target)?.path
    if (!path) {
      setError('target', { message: 'Unknown disk' })
      return
    }

    setIsStarting(true)
    setStartError('')
    try {
      const snapshot = await StartCleaning(
        path,
        i18n.resolvedLanguage ?? i18n.language,
        scanMode,
      )
      navigate(`/cleanup/${snapshot.id}`)
    } catch (reason) {
      setStartError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      setIsStarting(false)
    }
  }

  let usageColor: 'accent' | 'danger' | 'warning' = 'accent'
  if (usedPercent >= 85) {
    usageColor = 'danger'
  } else if (usedPercent >= 60) {
    usageColor = 'warning'
  }

  return (
    <div className="space-y-6">
      {isLLMConfigured === false && (
        <Alert status="warning" className="bg-warning/10">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{t('home.llmNotConfigured')}</Alert.Title>
            <Alert.Description>
              {t('home.llmNotConfiguredDescription')}{' '}
              <Link onPress={() => navigate('/settings')}>
                {t('home.goToSettings')}
              </Link>
            </Alert.Description>
          </Alert.Content>
        </Alert>
      )}

      <section
        className={`from-accent/10 via-accent/5 to-success/10 relative flex overflow-hidden rounded-3xl bg-gradient-to-br p-8 ${
          scanTarget === 'directory' ? 'min-h-[30rem]' : 'min-h-80'
        }`}
      >
        <div className="bg-accent/10 absolute -top-16 -right-12 size-56 rounded-full blur-3xl" />
        <div className="bg-success/10 absolute -bottom-20 left-1/3 size-64 rounded-full blur-3xl" />

        <div className="relative z-10 mx-auto flex w-full max-w-3xl flex-col items-center justify-center text-center">
          <div className="flex max-w-xl flex-col items-center">
            <span className="bg-accent/10 text-accent mb-5 inline-flex size-11 items-center justify-center rounded-xl">
              <Sparkles aria-hidden="true" size={21} strokeWidth={1.8} />
            </span>
            <h2 className="text-foreground text-2xl font-semibold tracking-tight">
              {t('home.startup')}
            </h2>
            <p className="text-muted mt-3 max-w-lg text-sm leading-6">
              {t('home.intro')}
            </p>
          </div>

          <div className="mt-8 flex w-full items-end justify-center gap-3">
            <div className="min-w-0 flex-1 text-left">
              <ControlledNextUIFormWrapper
                control={control}
                label={t('home.scanLoc')}
                name="target"
                rules={{ required: t('home.scanLocPlaceholder') }}
              >
                {(field) => (
                  <Select
                    isDisabled={isTaskRunning || isStarting || isLoadingDisks}
                    selectedKey={field.value}
                    variant="secondary"
                    onSelectionChange={(key) => field.onChange(key)}
                  >
                    <Select.Trigger className="h-12 rounded-xl bg-white/80 shadow-sm backdrop-blur-sm">
                      <HardDrive
                        aria-hidden="true"
                        className="text-muted"
                        size={18}
                        strokeWidth={1.8}
                      />
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {disks.map((disk) => (
                          <ListBox.Item
                            key={disk.path}
                            id={disk.path}
                            textValue={disk.name}
                          >
                            <div>
                              <p className="text-sm font-medium">{disk.name}</p>
                              <p className="text-muted text-xs">
                                {t('home.diskUsage', {
                                  available: formatBytes(disk.free),
                                  total: formatBytes(disk.total),
                                })}
                              </p>
                            </div>
                          </ListBox.Item>
                        ))}
                        <ListBox.Item
                          id="directory"
                          textValue={t('home.selectDirectory')}
                        >
                          <div>
                            <p className="text-sm font-medium">
                              {t('home.selectDirectory')}
                            </p>
                            <p className="text-muted text-xs">
                              {t('home.selectDirectoryPlaceholder')}
                            </p>
                          </div>
                        </ListBox.Item>
                      </ListBox>
                    </Select.Popover>
                  </Select>
                )}
              </ControlledNextUIFormWrapper>
            </div>

            <ButtonGroup
              className="mb-1 h-12 shrink-0 rounded-xl"
              isDisabled={isTaskRunning || isStarting || isCheckingLLM}
              variant="primary"
            >
              <Button
                className="h-12 gap-2 rounded-l-xl rounded-r-none px-6"
                onPress={() => void handleStartCleaning()}
              >
                {isStarting || isCheckingLLM ? (
                  <LoaderCircle
                    aria-hidden="true"
                    className="animate-spin"
                    size={18}
                  />
                ) : (
                  <ScanSearch aria-hidden="true" size={18} strokeWidth={1.9} />
                )}
                {isCheckingLLM
                  ? t('home.checkingLLMConfiguration')
                  : isStarting
                    ? t('home.creatingTask')
                    : t(`home.${scanMode}Scan`)}
              </Button>
              <Dropdown>
                <Button
                  isIconOnly
                  aria-label={t('home.selectScanMode')}
                  className="h-12 min-w-10 rounded-l-none rounded-r-xl px-0"
                  isDisabled={isTaskRunning || isStarting || isCheckingLLM}
                  variant="primary"
                >
                  <ChevronDown aria-hidden="true" size={16} strokeWidth={2} />
                </Button>
                <Dropdown.Popover placement="bottom end">
                  <Dropdown.Menu
                    aria-label={t('home.selectScanMode')}
                    selectionMode="single"
                    selectedKeys={new Set([scanMode])}
                    onAction={(key) => setScanMode(key as ScanMode)}
                  >
                    <Dropdown.Item id="fast" textValue={t('home.fastScan')}>
                      <div>
                        <p className="text-sm font-medium">
                          {t('home.fastScan')}
                        </p>
                        <p className="text-muted text-xs">
                          {t('home.fastScanDescription')}
                        </p>
                      </div>
                    </Dropdown.Item>
                    <Dropdown.Item id="deep" textValue={t('home.deepScan')}>
                      <div>
                        <p className="text-sm font-medium">
                          {t('home.deepScan')}
                        </p>
                        <p className="text-muted text-xs">
                          {t('home.deepScanDescription')}
                        </p>
                      </div>
                    </Dropdown.Item>
                  </Dropdown.Menu>
                </Dropdown.Popover>
              </Dropdown>
            </ButtonGroup>
          </div>

          {scanTarget === 'directory' && (
            <div className="mt-3 w-full text-left">
              <ControlledNextUIFormWrapper
                control={control}
                label={t('home.specDirectory')}
                name="directory"
                rules={{ required: t('home.specDirectoryPlaceholder') }}
              >
                {(field) => (
                  <Button
                    aria-label={t('home.openSysSelector')}
                    className="h-12 w-full justify-start gap-3 rounded-xl bg-white/80 px-4 shadow-sm backdrop-blur-sm"
                    isDisabled={isSelectingDirectory}
                    variant="secondary"
                    onPress={() => void handleSelectDirectory(field.onChange)}
                  >
                    <FolderOpen
                      aria-hidden="true"
                      className="text-muted shrink-0"
                      size={18}
                      strokeWidth={1.8}
                    />
                    <span
                      className={`min-w-0 flex-1 truncate text-left ${
                        field.value ? 'text-foreground' : 'text-muted'
                      }`}
                    >
                      {isSelectingDirectory
                        ? t('home.openingSysSelector')
                        : field.value || t('home.specDirectoryPlaceholder')}
                    </span>
                  </Button>
                )}
              </ControlledNextUIFormWrapper>
            </div>
          )}

          {(startError || diskError || error) && (
            <p className="text-danger mt-3 text-sm">
              {startError || diskError || error}
            </p>
          )}
        </div>
      </section>

      {task && isTaskRunning && (
        <Card className="rounded-2xl">
          <Card.Content className="flex-row items-center gap-4 p-5">
            <span className="bg-accent/10 text-accent inline-flex size-11 shrink-0 items-center justify-center rounded-2xl">
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin"
                size={20}
              />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="text-foreground truncate text-sm font-semibold">
                  {stateLabel(task.state)}
                </p>
                <Chip color="accent" size="sm" variant="soft">
                  {t('common.running')}
                </Chip>
              </div>
              <p className="text-muted mt-1 truncate text-xs">{task.path}</p>
              <p className="text-muted mt-1 text-xs">
                {t('common.running', {
                  cnt: task.scanProgress?.itemCount ?? 0,
                  size: formatBytes(task.scanProgress?.scannedSize ?? 0),
                })}
              </p>
            </div>
            <Button
              className="text-danger gap-2 rounded-xl"
              isDisabled={isStopping}
              variant="secondary"
              onPress={() => void stop()}
            >
              <Square aria-hidden="true" size={14} />
              {isStopping ? '正在停止…' : '停止任务'}
            </Button>
          </Card.Content>
        </Card>
      )}

      <section className="grid min-h-52 grid-cols-12 gap-6">
        <Card className="col-span-5 rounded-2xl">
          <Card.Header className="flex-row items-start justify-between">
            <div>
              <Card.Title>{t('home.currentUsage')}</Card.Title>
              <Card.Description>
                {systemDisk?.name ?? '本地磁盘 (C:)'}
              </Card.Description>
            </div>
            <span className="bg-default/60 text-muted inline-flex size-9 items-center justify-center rounded-xl">
              <HardDrive aria-hidden="true" size={18} strokeWidth={1.8} />
            </span>
          </Card.Header>

          <Card.Content className="space-y-5">
            <div>
              <div className="flex items-end justify-between">
                <div>
                  <span
                    className={clsx('text-3xl font-semibold', {
                      'text-danger': usedPercent >= 85,
                      'text-warning': usedPercent >= 60 && usedPercent < 85,
                      'text-foreground': usedPercent < 60,
                    })}
                  >
                    {systemDisk ? formatBytes(systemDisk.free) : '—'}
                  </span>
                  <span className="text-muted ml-1 text-sm">
                    {t('home.available')}
                  </span>
                </div>
                <span className="text-muted text-sm">
                  {systemDisk ? `${usedPercent}%` : '—'}
                </span>
              </div>

              <ProgressBar
                aria-label="本地磁盘使用率"
                className="mt-4"
                color={usageColor}
                value={usedPercent}
              >
                <ProgressBar.Track>
                  <ProgressBar.Fill />
                </ProgressBar.Track>
              </ProgressBar>
            </div>

            <div className="grid grid-cols-2 gap-3 text-sm">
              <div className="bg-default/40 rounded-xl p-3">
                <p className="text-muted text-xs">{t('home.used')}</p>
                <p className="text-foreground mt-1 font-semibold">
                  {systemDisk ? formatBytes(systemDisk.used) : '—'}
                </p>
              </div>
              <div className="bg-default/40 rounded-xl p-3">
                <p className="text-muted text-xs">{t('home.total')}</p>
                <p className="text-foreground mt-1 font-semibold">
                  {systemDisk ? formatBytes(systemDisk.total) : '—'}
                </p>
              </div>
            </div>
          </Card.Content>
        </Card>

        <Card className="col-span-7 rounded-2xl">
          <Card.Header className="flex-row items-start justify-between">
            <div>
              <Card.Title>{t('home.historyScan')}</Card.Title>
              <Card.Description>{t('home.historyScanDesc')}</Card.Description>
            </div>
            <Button
              aria-label="查看全部扫描记录"
              className="size-9 min-w-9 rounded-xl"
              variant="ghost"
              onPress={() => navigate('/cleanup')}
            >
              <ArrowUpRight aria-hidden="true" size={18} strokeWidth={1.8} />
            </Button>
          </Card.Header>

          <Card.Content className="space-y-1">
            {records.slice(0, 3).map((record) => (
              <button
                key={record.id}
                className="hover:bg-default/30 flex w-full cursor-pointer items-center justify-between gap-4 rounded-xl px-2 py-2.5 text-left transition-colors"
                type="button"
                onClick={() => navigate(`/cleanup/${record.id}`)}
              >
                <div className="flex min-w-0 items-center gap-3">
                  <span className="bg-success/10 text-success inline-flex size-9 shrink-0 items-center justify-center rounded-xl">
                    <CheckCircle2
                      aria-hidden="true"
                      size={17}
                      strokeWidth={1.8}
                    />
                  </span>
                  <div className="min-w-0">
                    <p className="text-foreground truncate text-sm font-medium">
                      {record.path}
                    </p>
                    <p className="text-muted mt-0.5 flex items-center gap-1 text-xs">
                      <Clock3 aria-hidden="true" size={12} />
                      {formatDate(record.startTime)}
                    </p>
                  </div>
                </div>

                <Chip
                  color={record.state === 'ERROR' ? 'danger' : 'success'}
                  size="sm"
                  variant="soft"
                >
                  {record.state === 'DONE'
                    ? t('common.freed', { size: formatBytes(record.freedSize) })
                    : stateLabel(record.state)}
                </Chip>
              </button>
            ))}
            {records.length === 0 && (
              <div className="text-muted flex min-h-32 items-center justify-center text-sm">
                {isLoading ? t('common.loading') : t('home.noHistoryRecord')}
              </div>
            )}
          </Card.Content>
        </Card>
      </section>
    </div>
  )
}
