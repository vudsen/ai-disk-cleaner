import { Button, Card, Checkbox, Label, toast } from '@heroui/react'
import type { cleaner } from '../../../../wailsjs/go/models'
import { ArrowLeft } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import useCleaningTask from '@/hooks/useCleaningTask'
import { DeleteTrashFiles } from '../../../../wailsjs/go/main/App'
import LLMOutput from './LLMOutput'
import ScanProgressHeader from './ScanProgressHeader'
import ScanResultStats from './ScanResultStats'
import TrashFileTable from './TrashFileTable'
import DeleteFailuresDrawer from './DeleteFailuresDrawer'
import { showDialog } from '@/components/DialogProvider'
import ControlledNextUIFormWrapper from '@/components/ControlledNextUIFormWrapper'
import { useForm } from 'react-hook-form'
import { toastError } from '@/util/toast-error'
import { useTranslation } from 'react-i18next'

const runningStates = new Set(['SCANNING', 'ANALYZING'])

export default function CleanupDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams()
  const { error, isLoading, isStopping, records, refreshRecords, stop, task } =
    useCleaningTask()
  const [isDeleting, setIsDeleting] = useState(false)
  const [deleteFailures, setDeleteFailures] = useState<cleaner.DeleteFailure[]>(
    [],
  )
  const [keepOriginalDirectoriesForRetry, setKeepOriginalDirectoriesForRetry] =
    useState(true)
  const recordID = Number(id)
  const currentTask = task?.id === recordID ? task : null
  const record = records.find((item) => item.id === recordID)
  const detail = currentTask ?? record
  const isRunning = currentTask ? runningStates.has(currentTask.state) : false
  const isAnalysisComplete = detail?.state === 'DONE'
  const trashFiles = record?.trashFiles ?? []

  useEffect(() => {
    setDeleteFailures([])
  }, [recordID])

  const handleDelete = async (path: string) => {
    let keepOriginalDirectories = true
    showDialog({
      title: t('cleanup.detail.deleteDialogTitle'),
      message: (
        <DeleteConfirmation
          path={path}
          onKeepOriginalDirectoriesChange={(value) => {
            keepOriginalDirectories = value
          }}
        />
      ),
      color: 'danger',
      onConfirm: async () => {
        setIsDeleting(true)
        try {
          setDeleteFailures([])
          setKeepOriginalDirectoriesForRetry(keepOriginalDirectories)
          const failures = await DeleteTrashFiles(
            record!.id,
            [path],
            keepOriginalDirectories,
          )
          setDeleteFailures(failures)
          await refreshRecords()
          toast(t('cleanup.detail.deleteSuccess'), {
            actionProps: {
              children: '关闭',
              onPress: () => toast.clear(),
              variant: 'tertiary',
            },
            description: (
              <div className="break-all">
                {t('cleanup.detail.deleteSuccessMsg', { path })}
              </div>
            ),
            variant: 'success',
          })
        } catch (reason) {
          toastError(reason, t('cleanup.detail.deleteFailed'))
        } finally {
          setIsDeleting(false)
        }
      },
    })
  }

  const handleRetrySuccess = async (path: string) => {
    setDeleteFailures((current) =>
      current.filter((failure) => failure.path !== path),
    )
    await refreshRecords()
  }

  return (
    <div className="space-y-6">
      <Button
        className="text-muted gap-2 rounded-xl"
        variant="ghost"
        onPress={() => navigate('/cleanup')}
      >
        <ArrowLeft aria-hidden="true" size={16} />
        {t('cleanup.detail.backToRecords')}
      </Button>

      {detail ? (
        <>
          <ScanProgressHeader
            detail={detail}
            isRunning={isRunning}
            isStopping={isStopping || Boolean(currentTask?.stopping)}
            scanProgress={currentTask?.scanProgress}
            onStop={() => void stop()}
          />

          {record && isAnalysisComplete && (
            <>
              <ScanResultStats
                topUsages={record.topUsages ?? []}
                trashFiles={trashFiles}
              />
              <TrashFileTable
                files={trashFiles}
                isDeleting={isDeleting}
                rootPath={record.path}
                onDelete={(paths) => void handleDelete(paths)}
              />
            </>
          )}

          <LLMOutput isRunning={isRunning} output={detail.llmOutput} />
        </>
      ) : (
        <Card className="rounded-3xl">
          <Card.Content className="text-muted flex min-h-56 items-center justify-center text-sm">
            {isLoading ? '正在加载清理任务…' : '没有找到这条清理任务'}
          </Card.Content>
        </Card>
      )}

      <DeleteFailuresDrawer
        failures={deleteFailures}
        keepOriginalDirectories={keepOriginalDirectoriesForRetry}
        recordID={recordID}
        onClose={() => setDeleteFailures([])}
        onRetrySuccess={handleRetrySuccess}
      />

      {error && <p className="text-danger text-sm">{error}</p>}
    </div>
  )
}

type DeleteConfirmationProps = {
  path: string
  onKeepOriginalDirectoriesChange: (value: boolean) => void
}

type DeleteConfirmationValues = {
  keepOriginalDirectories: boolean
}

function DeleteConfirmation({
  path,
  onKeepOriginalDirectoriesChange,
}: DeleteConfirmationProps) {
  const { t } = useTranslation()
  const { control } = useForm<DeleteConfirmationValues>({
    defaultValues: { keepOriginalDirectories: true },
  })

  return (
    <div className="space-y-4 break-all">
      <p>{t('cleanup.detail.deleteConfirmation', { path })}</p>
      <ControlledNextUIFormWrapper
        control={control}
        name="keepOriginalDirectories"
      >
        {(field) => (
          <Checkbox
            isSelected={field.value}
            onChange={(value) => {
              field.onChange(value)
              onKeepOriginalDirectoriesChange(value)
            }}
          >
            <Checkbox.Content>
              <Checkbox.Control>
                <Checkbox.Indicator />
              </Checkbox.Control>
              <Label>{t('cleanup.detail.keepOriginalDirectories')}</Label>
            </Checkbox.Content>
          </Checkbox>
        )}
      </ControlledNextUIFormWrapper>
    </div>
  )
}
