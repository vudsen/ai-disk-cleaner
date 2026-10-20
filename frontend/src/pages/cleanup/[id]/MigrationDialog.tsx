import { Button, Description, Input, Label, Modal } from '@heroui/react'
import { toast } from '@/util/toast'
import {
  Check,
  Copy,
  FolderOpen,
  Link,
  LoaderCircle,
  RotateCcw,
  Trash2,
  Waypoints,
  X,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import {
  CopyMigrationSource,
  CreateMigrationLink,
  DeleteMigrationSource,
  SelectMigrationDirectory,
} from '../../../../wailsjs/go/main/App'
import ControlledNextUIFormWrapper from '@/components/ControlledNextUIFormWrapper'
import { toastError } from '@/util/toast-error'
import { useTranslation } from 'react-i18next'
import i18n from 'i18next'

type MigrationTarget = {
  name: string
  source: string
}

type MigrationDialogProps = {
  target: MigrationTarget | null
  onClose: () => void
}

type FormValues = {
  name: string
}

type StepStatus = 'pending' | 'running' | 'success' | 'error'

type MigrationStep = {
  description: string
  icon: typeof Copy
  label: string
}

const initialStatuses = (): StepStatus[] => ['pending', 'pending', 'pending']

export default function MigrationDialog({
  target,
  onClose,
}: MigrationDialogProps) {
  const { t } = useTranslation()
  const { control, getValues, reset, trigger } = useForm<FormValues>({
    defaultValues: { name: '' },
  })
  const [directory, setDirectory] = useState('')
  const [destination, setDestination] = useState('')
  const [isSelecting, setIsSelecting] = useState(false)
  const [statuses, setStatuses] = useState<StepStatus[]>(initialStatuses)
  const [stepErrors, setStepErrors] = useState<string[]>(['', '', ''])
  const hasStarted = statuses.some((status) => status !== 'pending')
  const isRunning = statuses.includes('running')
  const isComplete = statuses.every((status) => status === 'success')
  const failedStep = statuses.findIndex((status) => status === 'error')
  const completedCount = statuses.filter(
    (status) => status === 'success',
  ).length
  const canClose = !isRunning && (!hasStarted || isComplete || failedStep === 0)

  const progressLabel = useMemo(() => {
    if (isComplete) return t('cleanup.detail.allDone')
    if (failedStep >= 0)
      return t('cleanup.detail.migrationPartFail', { step: failedStep + 1 })
    if (isRunning)
      return t('cleanup.detail.migrationRunning', { step: failedStep + 1 })
    return t('cleanup.detail.readyToMigrate')
  }, [completedCount, failedStep, isComplete, isRunning])

  useEffect(() => {
    if (!target) return
    setDirectory('')
    setDestination('')
    setStatuses(initialStatuses())
    setStepErrors(['', '', ''])
    setIsSelecting(false)
    reset({ name: target.name })
  }, [reset, target])

  const selectDirectory = async () => {
    setIsSelecting(true)
    try {
      const selected = await SelectMigrationDirectory()
      if (selected) setDirectory(selected)
    } catch (reason) {
      toastError(reason, t('cleanup.detail.failedToMigrate'))
    } finally {
      setIsSelecting(false)
    }
  }

  const setStepStatus = (
    index: number,
    status: StepStatus,
    errorMessage = '',
  ) => {
    setStatuses((current) =>
      current.map((value, stepIndex) => (stepIndex === index ? status : value)),
    )
    setStepErrors((current) =>
      current.map((value, stepIndex) =>
        stepIndex === index ? errorMessage : value,
      ),
    )
  }

  const executeStep = async (
    index: number,
    source: string,
    dest: string,
    name: string,
  ) => {
    switch (index) {
      case 0:
        return CopyMigrationSource(source, directory, name)
      case 1:
        await DeleteMigrationSource(source, dest)
        return dest
      case 2:
        await CreateMigrationLink(source, dest, name)
        return dest
      default:
        throw new Error('Unknown Step: ' + index)
    }
  }

  const migrationSteps: MigrationStep[] = useMemo(
    () => [
      {
        description: t('cleanup.detail.migrationStep1Desc'),
        icon: Copy,
        label: t('cleanup.detail.migrationStep1'),
      },
      {
        description: t('cleanup.detail.migrationStep2Desc'),
        icon: Trash2,
        label: t('cleanup.detail.migrationStep2'),
      },
      {
        description: t('cleanup.detail.migrationStep3Desc'),
        icon: Link,
        label: t('cleanup.detail.migrationStep3'),
      },
    ],
    [t],
  )

  const runFromStep = async (
    startIndex: number,
    source: string,
    initialDestination: string,
    name: string,
  ) => {
    let currentDestination = initialDestination
    for (let index = startIndex; index < migrationSteps.length; index += 1) {
      setStepStatus(index, 'running')
      try {
        currentDestination = await executeStep(
          index,
          source,
          currentDestination,
          name,
        )
        if (index === 0) setDestination(currentDestination)
        setStepStatus(index, 'success')
      } catch (reason) {
        const message =
          reason instanceof Error ? reason.message : String(reason)
        setStepStatus(index, 'error', message)
        toastError(reason, `${migrationSteps[index].label}失败`)
        return
      }
    }
    toast(t('cleanup.detail.migrateSuccess'), {
      description: t('cleanup.detail.migrateSuccessDesc'),
      variant: 'success',
    })
  }

  const migrate = async () => {
    if (!target || !directory || !(await trigger('name'))) return
    const name = getValues('name').trim()
    await runFromStep(0, target.source, '', name)
  }

  const retryFailedStep = async () => {
    if (!target || failedStep < 0) return
    const name = getValues('name').trim()
    await runFromStep(failedStep, target.source, destination, name)
  }

  const handleClose = () => {
    if (canClose) onClose()
  }

  return (
    <Modal.Backdrop
      isDismissable={canClose}
      isOpen={target !== null}
      onOpenChange={(open) => {
        if (!open) handleClose()
      }}
    >
      <Modal.Container size="lg">
        <Modal.Dialog>
          <Modal.CloseTrigger isDisabled={!canClose} />
          <Modal.Header className="flex flex-row items-center">
            <Waypoints aria-hidden="true" />
            {t('cleanup.detail.migrateFile')}
          </Modal.Header>
          <Modal.Body className="space-y-5">
            <div>
              <p className="text-muted text-xs font-medium">
                {t('cleanup.detail.sourcePath')}
              </p>
              <p className="text-foreground mt-1 text-sm break-all">
                {target?.source}
              </p>
            </div>

            {!hasStarted && (
              <>
                <div>
                  <Label className="mb-1.5 block text-sm">
                    {t('cleanup.detail.destPath')}
                  </Label>
                  <Button
                    className="h-11 w-full justify-start gap-3 rounded-xl"
                    isDisabled={isSelecting}
                    variant="secondary"
                    onPress={() => void selectDirectory()}
                  >
                    {isSelecting ? (
                      <LoaderCircle
                        aria-hidden="true"
                        className="animate-spin"
                        size={17}
                      />
                    ) : (
                      <FolderOpen aria-hidden="true" size={17} />
                    )}
                    <span className="min-w-0 flex-1 truncate text-left">
                      {directory ||
                        (isSelecting
                          ? t('cleanup.detail.openingFileSelector')
                          : t('cleanup.detail.destPathSelect'))}
                    </span>
                  </Button>
                  <Description className="mt-1.5">
                    {t('cleanup.detail.destPathSelectDesc')}
                  </Description>
                </div>

                <ControlledNextUIFormWrapper
                  control={control}
                  description={t('cleanup.detail.migratedFileNameDesc')}
                  label={t('cleanup.detail.migratedFileName')}
                  name="name"
                  rules={{
                    required: t('cleanup.detail.migratedFileNameRequired'),
                    validate: (value) =>
                      (value.trim() !== '.' &&
                        value.trim() !== '..' &&
                        !/[\\/]/.test(value.trim())) ||
                      t('cleanup.detail.pathSeparatorInvalid'),
                  }}
                >
                  {(field) => <Input {...field} />}
                </ControlledNextUIFormWrapper>
              </>
            )}

            <div className="border-default-200 rounded-2xl border p-4">
              <div className="mb-4 flex items-center justify-between gap-3">
                <p className="text-sm font-semibold">
                  {t('cleanup.detail.migrateProgress')}
                </p>
                <span className="text-muted text-xs">{progressLabel}</span>
              </div>
              <ol className="space-y-1">
                {migrationSteps.map((step, index) => (
                  <MigrationStepItem
                    key={step.label}
                    index={index}
                    error={stepErrors[index]}
                    isLast={index === migrationSteps.length - 1}
                    status={statuses[index]}
                    step={step}
                  />
                ))}
              </ol>
            </div>
          </Modal.Body>
          <Modal.Footer>
            {isComplete ? (
              <Button variant="primary" onPress={handleClose}>
                {t('common.done')}
              </Button>
            ) : failedStep >= 0 ? (
              <>
                {failedStep === 0 && (
                  <Button variant="ghost" onPress={handleClose}>
                    {t('common.close')}
                  </Button>
                )}
                <Button
                  isPending={isRunning}
                  variant="primary"
                  onPress={() => void retryFailedStep()}
                >
                  <RotateCcw aria-hidden="true" size={16} />
                  {t('cleanup.detail.migrateRetryStep', {
                    step: failedStep + 1,
                  })}
                </Button>
              </>
            ) : hasStarted ? (
              <Button isDisabled isPending variant="primary">
                {t('cleanup.detail.migrating')}
              </Button>
            ) : (
              <>
                <Button variant="ghost" onPress={handleClose}>
                  {t('common.cancel')}
                </Button>
                <Button
                  isDisabled={!directory}
                  variant="primary"
                  onPress={() => void migrate()}
                >
                  {t('cleanup.detail.startMigrate')}
                </Button>
              </>
            )}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

function MigrationStepItem({
  error,
  isLast,
  status,
  step,
  index,
}: {
  index: number
  error: string
  isLast: boolean
  status: StepStatus
  step: MigrationStep
}) {
  const StepIcon = step.icon

  return (
    <li className="relative flex items-center gap-3 pb-4 last:pb-0">
      {!isLast && (
        <span className="bg-default absolute top-8 left-3.5 h-[calc(100%-1.25rem)] w-px" />
      )}
      <span
        className={[
          'relative z-10 flex size-7 shrink-0 items-center justify-center rounded-full',
          status === 'success' && 'bg-success text-white',
          status === 'error' && 'bg-danger text-danger-foreground',
          status === 'running' && 'bg-primary text-primary-foreground',
          status === 'pending' && 'bg-default-100 text-muted',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        {status === 'success' ? (
          <Check aria-hidden="true" size={15} />
        ) : status === 'error' ? (
          <X aria-hidden="true" size={15} />
        ) : status === 'running' ? (
          <LoaderCircle aria-hidden="true" className="animate-spin" size={15} />
        ) : (
          <StepIcon aria-hidden="true" size={14} />
        )}
      </span>
      <div className="min-w-0 pt-0.5">
        <p className="text-sm font-medium">
          {index + 1}. {step.label}
          <span className="text-muted ml-2 text-xs font-normal">
            {statusLabel(status)}
          </span>
        </p>
        <p className="text-muted mt-0.5 text-xs">{step.description}</p>
        {error && <p className="text-danger mt-1 text-xs break-all">{error}</p>}
      </div>
    </li>
  )
}

function statusLabel(status: StepStatus) {
  return {
    error: i18n.t('common.error'),
    pending: i18n.t('common.waiting'),
    running: i18n.t('common.running'),
    success: i18n.t('common.done'),
  }[status]
}
