import { Alert, Button, Chip, ProgressBar } from '@heroui/react'
import {
  AlertCircle,
  Clock3,
  FolderSearch,
  LoaderCircle,
  Square,
} from 'lucide-react'
import type { scanner } from '../../../../wailsjs/go/models'
import { formatBytes, formatDate, stateLabel } from '@/hooks/useCleaningTask'
import { taskStateColor } from '..'
import { useTranslation } from 'react-i18next'

type ScanProgressHeaderProps = {
  detail: {
    errorMessage: string
    path: string
    startTime: unknown
    state: string
    scanSummary?: scanner.ScanSummary | null
  }
  isRunning: boolean
  isStopping: boolean
  onStop: () => void
  scanProgress?: scanner.ScanProgress | null
  scanSummary?: scanner.ScanSummary | null
}

export default function ScanProgressHeader({
  detail,
  isRunning,
  isStopping,
  onStop,
  scanProgress,
  scanSummary,
}: ScanProgressHeaderProps) {
  const { t } = useTranslation()
  return (
    <div className="from-accent/10 via-accent/5 to-success/10 relative overflow-hidden rounded-2xl bg-linear-to-br p-7">
      <div className="relative flex items-start justify-between gap-6">
        <div className="flex min-w-0 items-start gap-4">
          <span className="bg-accent text-accent-foreground inline-flex size-12 shrink-0 items-center justify-center rounded-2xl shadow-sm">
            <FolderSearch aria-hidden="true" size={22} strokeWidth={1.9} />
          </span>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-foreground truncate text-xl font-semibold">
                {detail.path}
              </h2>
              <Chip
                color={taskStateColor(detail.state)}
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
                {stateLabel(detail.state)}
              </Chip>
            </div>
            <p className="text-muted mt-2 flex items-center gap-1 text-xs">
              <Clock3 aria-hidden="true" size={12} />
              {formatDate(detail.startTime)}
            </p>
          </div>
        </div>

        {isRunning && (
          <Button
            className="text-danger shrink-0 gap-2 rounded-xl"
            isDisabled={isStopping}
            variant="secondary"
            onPress={onStop}
          >
            <Square aria-hidden="true" size={14} />
            {isStopping ? t('common.stopping') : t('common.stop')}
          </Button>
        )}
      </div>

      <ProgressBar
        aria-label="清理任务进度"
        className="mt-7"
        color={detail.state === 'ERROR' ? 'danger' : 'accent'}
        isIndeterminate={isRunning}
        value={isRunning ? undefined : 100}
      >
        <ProgressBar.Track className="bg-white/70">
          <ProgressBar.Fill />
        </ProgressBar.Track>
      </ProgressBar>

      <div className="mt-6 grid grid-cols-3 gap-3">
        <Metric
          label={t('cleanup.detail.foundFiles')}
          value={
            scanProgress
              ? scanProgress.itemCount.toLocaleString()
              : t('common.done')
          }
        />
        <Metric
          label={t('cleanup.detail.scannedSize')}
          value={scanProgress ? formatBytes(scanProgress.scannedSize) : '—'}
        />
        <Metric
          label={t('cleanup.detail.currentStatus')}
          value={stateLabel(detail.state)}
        />
      </div>

      {detail.errorMessage && (
        <div className="bg-danger/10 text-danger mt-5 flex items-start gap-2 rounded-2xl p-4 text-sm">
          <AlertCircle
            aria-hidden="true"
            className="mt-0.5 shrink-0"
            size={16}
          />
          {detail.errorMessage}
        </div>
      )}
      {scanSummary &&
        (scanSummary.excludedCount > 0 || scanSummary.readErrorCount > 0) && (
          <Alert status="warning" className="bg-warning/10 mt-5">
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Title>
                {t('cleanup.detail.scanIncompleteTitle')}
              </Alert.Title>
              <Alert.Description>
                {t('cleanup.detail.scanIncomplete', {
                  excluded: scanSummary.excludedCount,
                  errors: scanSummary.readErrorCount,
                })}
              </Alert.Description>
            </Alert.Content>
          </Alert>
        )}
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl bg-white/70 p-4">
      <p className="text-muted flex items-center gap-1 text-xs">{label}</p>
      <p className="text-foreground mt-1 text-base font-semibold">{value}</p>
    </div>
  )
}
