import { Button, Card, Chip, ProgressBar } from '@heroui/react'
import {
  AlertCircle,
  ArrowRight,
  Ban,
  CheckCircle2,
  Clock3,
  Eraser,
  FileArchive,
  FolderSearch,
  HardDrive,
  LoaderCircle,
  Square,
} from 'lucide-react'
import { useNavigate } from 'react-router'
import useCleaningTask, {
  formatBytes,
  formatDate,
  stateLabel,
} from '@/hooks/useCleaningTask'
import { useTranslation } from 'react-i18next'

const runningStates = new Set(['SCANNING', 'ANALYZING'])

export default function CleanupPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { error, isStopping, records, stop, task } = useCleaningTask()
  const isRunning = task ? runningStates.has(task.state) : false

  return (
    <div className="space-y-8">
      <section aria-labelledby="active-cleanup-heading">
        <div className="mb-4">
          <h2
            id="active-cleanup-heading"
            className="text-foreground text-lg font-semibold"
          >
            {t('cleanup.currentTask')}
          </h2>
          <p className="text-muted mt-1 text-sm">
            {t('cleanup.currentTaskDesc')}
          </p>
        </div>

        {task ? (
          <Card className="overflow-hidden rounded-3xl">
            <Card.Content className="p-0">
              <div className="from-accent/10 via-accent/5 to-success/10 relative overflow-hidden bg-linear-to-br p-7">
                <div className="relative flex items-center gap-5">
                  <span className="bg-accent text-accent-foreground inline-flex size-12 shrink-0 items-center justify-center rounded-2xl shadow-sm">
                    <FolderSearch
                      aria-hidden="true"
                      size={22}
                      strokeWidth={1.9}
                    />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="text-foreground truncate text-lg font-semibold">
                        {task.path}
                      </h3>
                      <Chip
                        color={taskStateColor(task.state)}
                        size="sm"
                        variant="soft"
                      >
                        {isRunning && (
                          <LoaderCircle
                            aria-hidden="true"
                            className="animate-spin"
                            size={12}
                          />
                        )}
                        {stateLabel(task.state)}
                      </Chip>
                    </div>
                    <p className="text-muted mt-2 text-xs">
                      {t('cleanup.found', {
                        cnt: (
                          task.scanProgress?.itemCount ?? 0
                        ).toLocaleString(),
                        size: formatBytes(task.scanProgress?.scannedSize ?? 0),
                      })}
                    </p>
                    <ProgressBar
                      aria-label="当前清理任务进度"
                      className="mt-4 max-w-xl"
                      color={task.state === 'ERROR' ? 'danger' : 'accent'}
                      isIndeterminate={isRunning}
                      value={isRunning ? undefined : 100}
                    >
                      <ProgressBar.Track className="bg-white/70">
                        <ProgressBar.Fill />
                      </ProgressBar.Track>
                    </ProgressBar>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {isRunning && (
                      <Button
                        className="text-danger gap-2 rounded-xl"
                        isDisabled={isStopping || task.stopping}
                        variant="secondary"
                        onPress={() => void stop()}
                      >
                        <Square aria-hidden="true" size={14} />
                        {isStopping || task.stopping
                          ? t('common.stopping')
                          : t('common.stop')}
                      </Button>
                    )}
                    <Button
                      className="gap-2 rounded-xl"
                      variant="primary"
                      onPress={() => navigate(`/cleanup/${task.id}`)}
                    >
                      {t('common.detail')}
                      <ArrowRight aria-hidden="true" size={15} />
                    </Button>
                  </div>
                </div>
              </div>
            </Card.Content>
          </Card>
        ) : (
          <Card className="rounded-3xl">
            <Card.Content className="text-muted flex min-h-40 items-center justify-center gap-3 text-sm">
              <HardDrive aria-hidden="true" size={19} />
              {t('cleanup.noRunningTask')}
            </Card.Content>
          </Card>
        )}

        {error && <p className="text-danger mt-3 text-sm">{error}</p>}
      </section>

      <section aria-labelledby="cleanup-history-heading">
        <div className="mb-4">
          <h2
            id="cleanup-history-heading"
            className="text-foreground text-lg font-semibold"
          >
            {t('home.historyScan')}
          </h2>
          <p className="text-muted mt-1 text-sm">{t('home.historyScan')}</p>
        </div>

        <Card className="rounded-3xl">
          <Card.Content className="divide-default divide-y p-2">
            {records.map((record) => (
              <button
                key={record.id}
                className="hover:bg-default/30 flex w-full items-center gap-4 rounded-2xl px-4 py-4 text-left transition-colors"
                type="button"
                onClick={() => navigate(`/cleanup/${record.id}`)}
              >
                <span
                  className={`inline-flex size-11 shrink-0 items-center justify-center rounded-2xl ${historyIconStyle(record.state)}`}
                >
                  <HistoryIcon state={record.state} />
                </span>

                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <p className="text-foreground truncate text-sm font-semibold">
                      {record.path}
                    </p>
                    <Chip
                      color={taskStateColor(record.state)}
                      size="sm"
                      variant="soft"
                    >
                      {stateLabel(record.state)}
                    </Chip>
                  </div>
                  <p className="text-muted mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                    <span className="inline-flex items-center gap-1">
                      <Clock3 aria-hidden="true" size={12} />
                      {formatDate(record.startTime)}
                    </span>
                    <span className="inline-flex items-center gap-1">
                      <FileArchive aria-hidden="true" size={12} />
                      {t('cleanup.suggestions', {
                        cnt: record.trashFiles?.length ?? 0,
                      })}
                    </span>
                    <span className="inline-flex items-center gap-1">
                      <Eraser size={12} />
                      {t('common.freed', {
                        size: formatBytes(record.freedSize),
                      })}
                    </span>
                  </p>
                </div>

                <div className="shrink-0 text-right">
                  <p className="text-muted text-xs">{t('cleanup.tokenCost')}</p>
                  <p className="text-foreground mt-0.5 text-base font-semibold">
                    {(record.tokenUsage ?? 0).toLocaleString()}
                  </p>
                </div>
                <ArrowRight
                  aria-hidden="true"
                  className="text-muted"
                  size={17}
                />
              </button>
            ))}

            {records.length === 0 && (
              <div className="text-muted flex min-h-40 items-center justify-center text-sm">
                {t('home.noHistoryRecord')}
              </div>
            )}
          </Card.Content>
        </Card>
      </section>
    </div>
  )
}

function HistoryIcon({ state }: { state: string }) {
  if (state === 'DONE') {
    return <CheckCircle2 aria-hidden="true" size={20} strokeWidth={1.8} />
  }
  if (state === 'CANCELLED') {
    return <Ban aria-hidden="true" size={20} strokeWidth={1.8} />
  }
  return <AlertCircle aria-hidden="true" size={20} strokeWidth={1.8} />
}

export function taskStateColor(
  state: string,
): 'success' | 'danger' | 'accent' | 'default' {
  if (state === 'DONE') return 'success'
  if (state === 'ERROR') return 'danger'
  if (state === 'SCANNING' || state === 'ANALYZING') return 'accent'
  return 'default'
}

function historyIconStyle(state: string) {
  if (state === 'DONE') return 'bg-success/10 text-success'
  if (state === 'ERROR') return 'bg-danger/10 text-danger'
  return 'bg-default/60 text-muted'
}
