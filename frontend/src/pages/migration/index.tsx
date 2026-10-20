import type { Key } from '@heroui/react'
import { Button, Card, Dropdown, Label, Table, Tooltip } from '@heroui/react'
import { EllipsisVertical, LoaderCircle, Undo2, Waypoints } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import type { migration } from '../../../wailsjs/go/models'
import {
  ListMigrations,
  OpenTrashFileDirectory,
  RestoreMigration,
} from '../../../wailsjs/go/main/App'
import { showDialog } from '@/components/DialogProvider'
import { formatDate } from '@/hooks/useCleaningTask'
import { toastError } from '@/util/toast-error'
import { toast } from '@/util/toast'
import { useTranslation } from 'react-i18next'

const hasWailsRuntime = () =>
  typeof window !== 'undefined' && 'go' in window && 'runtime' in window

const enum Actions {
  OPEN_ORIGIN_PATH = 'open-origin-path',
  OPEN_MIGRATED_PATH = 'open-migrated-path',
}
export default function MigrationPage() {
  const [records, setRecords] = useState<migration.Migration[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [restoringID, setRestoringID] = useState<number | null>(null)
  const [error, setError] = useState('')
  const { t } = useTranslation()

  const load = useCallback(async () => {
    if (!hasWailsRuntime()) {
      setIsLoading(false)
      return
    }
    setIsLoading(true)
    setError('')
    try {
      setRecords((await ListMigrations()) ?? [])
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const restore = async (record: migration.Migration) => {
    setRestoringID(record.id)
    try {
      await RestoreMigration(record.id)
      setRecords((current) => current.filter((item) => item.id !== record.id))
      toast(t('migration.recoverSuccess'), {
        description: t('migration.recoverSuccess', { path: record.name }),
        variant: 'success',
      })
    } catch (reason) {
      toastError(reason, t('migration.recoverFailed'))
    } finally {
      setRestoringID(null)
    }
  }

  const confirmRestore = (record: migration.Migration) => {
    showDialog({
      title: t('migration.recoverMigratedFile'),
      message: (
        <div className="space-y-2 text-sm">
          <p>{t('migration.recoverMigratedFileDesc')}</p>
          <p className="text-muted break-all">{record.source}</p>
        </div>
      ),
      confirmBtnText: t('common.confirm'),
      onConfirm: () => restore(record),
    })
  }

  const onAction = useCallback(async (key: Key, mi: migration.Migration) => {
    try {
      switch (key) {
        case Actions.OPEN_ORIGIN_PATH:
          await OpenTrashFileDirectory(mi.source)
          break
        case Actions.OPEN_MIGRATED_PATH:
          await OpenTrashFileDirectory(mi.dest)
          break
        default:
          break
      }
    } catch (e) {
      toastError(e, '操作失败')
    }
  }, [])

  return (
    <Card className="rounded-3xl" variant="transparent">
      <Card.Content>
        <Table variant="secondary">
          <Table.ScrollContainer>
            <Table.Content aria-label="迁移记录列表">
              <Table.Header>
                <Table.Column isRowHeader>{t('common.name')}</Table.Column>
                <Table.Column>{t('migration.sourcePath')}</Table.Column>
                <Table.Column>{t('migration.destPath')}</Table.Column>
                <Table.Column>{t('migration.migrationTime')}</Table.Column>
                <Table.Column>{t('common.actions')}</Table.Column>
              </Table.Header>
              <Table.Body>
                {records.map((record) => (
                  <Table.Row key={record.id} id={String(record.id)}>
                    <Table.Cell>
                      <div className="flex items-center gap-2">
                        <span className="bg-accent/10 text-accent inline-flex size-8 shrink-0 items-center justify-center rounded-lg">
                          <Waypoints aria-hidden="true" size={15} />
                        </span>
                        <span className="font-medium">{record.name}</span>
                      </div>
                    </Table.Cell>
                    <Table.Cell>
                      <PathCell path={record.source} />
                    </Table.Cell>
                    <Table.Cell>
                      <PathCell path={record.dest} />
                    </Table.Cell>
                    <Table.Cell>{formatDate(record.createdAt)}</Table.Cell>
                    <Table.Cell>
                      <Tooltip delay={0}>
                        <Button
                          isIconOnly
                          className="gap-2 rounded-xl"
                          isDisabled={
                            restoringID !== null && restoringID !== record.id
                          }
                          isPending={restoringID === record.id}
                          size="sm"
                          variant="secondary"
                          onPress={() => confirmRestore(record)}
                        >
                          {restoringID === record.id ? (
                            <LoaderCircle
                              aria-hidden="true"
                              className="animate-spin"
                              size={14}
                            />
                          ) : (
                            <Undo2 aria-hidden="true" size={14} />
                          )}
                        </Button>
                        <Tooltip.Content>
                          {t('migration.recover')}
                        </Tooltip.Content>
                      </Tooltip>
                      <Dropdown>
                        <Button
                          isIconOnly
                          aria-label="打开操作菜单"
                          variant="ghost"
                        >
                          <EllipsisVertical size={16} />
                        </Button>
                        <Dropdown.Popover>
                          <Dropdown.Menu
                            onAction={(key) => onAction(key, record)}
                          >
                            <Dropdown.Item
                              id={Actions.OPEN_ORIGIN_PATH}
                              textValue={t('migration.openSourceDirectory')}
                            >
                              <Label>
                                {t('migration.openSourceDirectory')}
                              </Label>
                            </Dropdown.Item>
                            <Dropdown.Item
                              id={Actions.OPEN_MIGRATED_PATH}
                              textValue={t('migration.openDestDirectory')}
                            >
                              <Label>{t('migration.openDestDirectory')}</Label>
                            </Dropdown.Item>
                          </Dropdown.Menu>
                        </Dropdown.Popover>
                      </Dropdown>
                    </Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>

        {records.length === 0 && (
          <div className="text-muted flex min-h-44 items-center justify-center text-sm">
            {isLoading ? (
              <span className="inline-flex items-center gap-2">
                <LoaderCircle
                  aria-hidden="true"
                  className="animate-spin"
                  size={17}
                />
                {t('migration.loadingMigrations')}
              </span>
            ) : (
              t('migration.noMigration')
            )}
          </div>
        )}
        {error && <p className="text-danger mt-3 text-sm">{error}</p>}
      </Card.Content>
    </Card>
  )
}

function PathCell({ path }: { path: string }) {
  return (
    <Tooltip>
      <Tooltip.Trigger className="text-muted block max-w-64 truncate">
        {path}
      </Tooltip.Trigger>
      <Tooltip.Content>
        <Tooltip.Arrow />
        <span className="max-w-96 break-all">{path}</span>
      </Tooltip.Content>
    </Tooltip>
  )
}
