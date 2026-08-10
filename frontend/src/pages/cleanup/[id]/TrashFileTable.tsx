import {
  Button,
  Card,
  Chip,
  Dropdown,
  Label,
  Table,
  Tooltip,
} from '@heroui/react'
import { EllipsisVertical, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { cleaningrecord } from '../../../../wailsjs/go/models'
import { OpenTrashFileDirectory } from '../../../../wailsjs/go/main/App'
import { ClipboardSetText } from '../../../../wailsjs/runtime'
import { formatBytes } from '@/hooks/useCleaningTask'
import { toastError } from '@/util/toast-error'
import MigrationDialog from './MigrationDialog'
import { useTranslation } from 'react-i18next'

type TrashFileTableProps = {
  files: cleaningrecord.TrashFile[]
  isDeleting: boolean
  rootPath: string
  onDelete: (paths: string) => void
}

export default function TrashFileTable({
  files,
  isDeleting,
  rootPath,
  onDelete,
}: TrashFileTableProps) {
  const { t } = useTranslation()
  const [migrationTarget, setMigrationTarget] = useState<{
    name: string
    source: string
  } | null>(null)
  const sortedFiles = useMemo(
    () =>
      [...files].sort((a, b) =>
        a.isDeleted === b.isDeleted
          ? a.level === b.level
            ? b.size - a.size
            : a.level - b.level
          : Number(a.isDeleted) - Number(b.isDeleted),
      ),
    [files],
  )

  const handleAction = async (
    action: string,
    file: cleaningrecord.TrashFile,
  ) => {
    const fullPath = resolvePath(rootPath, file.path)
    try {
      switch (action) {
        case 'copy-path': {
          const copied = await ClipboardSetText(fullPath)
          if (!copied) {
            throw new Error('复制文件路径失败')
          }
          break
        }
        case 'open-directory':
          await OpenTrashFileDirectory(fullPath)
          break
        case 'migrate-file':
          setMigrationTarget({
            name: pathName(fullPath),
            source: fullPath,
          })
          break
        case 'delete-file':
          onDelete(file.path)
          break
      }
    } catch (reason) {
      toastError(reason, '操作失败')
    }
  }

  return (
    <Card className="rounded-3xl">
      <Card.Header className="flex-row items-center justify-between">
        <div>
          <Card.Title>{t('cleanup.detail.cleanableFiles')}</Card.Title>
          <Card.Description>
            {t('cleanup.detail.cleanableFilesDesc')}
          </Card.Description>
        </div>
      </Card.Header>
      <Card.Content>
        <Table variant="secondary">
          <Table.ScrollContainer>
            <Table.Content aria-label="可清理文件列表">
              <Table.Header>
                <Table.Column>{t('common.actions')}</Table.Column>
                <Table.Column isRowHeader>{t('common.name')}</Table.Column>
                <Table.Column>{t('common.path')}</Table.Column>
                <Table.Column>{t('common.category')}</Table.Column>
                <Table.Column id="size">{t('common.size')}</Table.Column>
                <Table.Column>{t('cleanup.detail.reason')}</Table.Column>
              </Table.Header>
              <Table.Body>
                {sortedFiles.map((file) => (
                  <Table.Row
                    key={file.path}
                    id={file.path}
                    className={file.isDeleted ? 'line-through' : undefined}
                  >
                    <Table.Cell>
                      <Dropdown>
                        <Button
                          isIconOnly
                          aria-label={`打开 ${file.name} 的操作菜单`}
                          variant="ghost"
                          size="sm"
                        >
                          <EllipsisVertical size={16} />
                        </Button>
                        <Dropdown.Popover>
                          <Dropdown.Menu
                            onAction={(key) =>
                              void handleAction(String(key), file)
                            }
                          >
                            <Dropdown.Item
                              id="copy-path"
                              textValue={t('cleanup.detail.copyFilePath')}
                            >
                              <Label>{t('cleanup.detail.copyFilePath')}</Label>
                            </Dropdown.Item>
                            <Dropdown.Item
                              id="open-directory"
                              textValue={t('cleanup.detail.openDirectory')}
                            >
                              <Label>{t('cleanup.detail.openDirectory')}</Label>
                            </Dropdown.Item>
                            <Dropdown.Item
                              id="migrate-file"
                              isDisabled={file.isDeleted}
                              textValue={t('cleanup.detail.moveFile')}
                            >
                              <Label>{t('cleanup.detail.moveFile')}</Label>
                            </Dropdown.Item>
                            <Dropdown.Item
                              id="delete-file"
                              isDisabled={file.isDeleted || isDeleting}
                              textValue={t('cleanup.detail.deleteFile')}
                              variant="danger"
                            >
                              <Label>{t('cleanup.detail.deleteFile')}</Label>
                            </Dropdown.Item>
                          </Dropdown.Menu>
                        </Dropdown.Popover>
                      </Dropdown>
                      <Button
                        onPress={() => onDelete(file.path)}
                        isIconOnly
                        size="sm"
                        variant="ghost"
                        className="text-danger"
                      >
                        <Trash2 />
                      </Button>
                    </Table.Cell>
                    <Table.Cell>{file.name}</Table.Cell>
                    <Table.Cell>
                      <Tooltip delay={0}>
                        <Tooltip.Trigger className="text-muted block max-w-82 truncate">
                          {file.path}
                        </Tooltip.Trigger>
                        <Tooltip.Content>
                          <Tooltip.Arrow />
                          {file.path}
                        </Tooltip.Content>
                      </Tooltip>
                    </Table.Cell>
                    <Table.Cell>
                      <Chip color={levelColor(file.level)} variant="soft">
                        {levelLabel(file.level)}
                      </Chip>
                    </Table.Cell>
                    <Table.Cell>{formatBytes(file.size ?? 0)}</Table.Cell>
                    <Table.Cell>
                      <Tooltip delay={0}>
                        <Tooltip.Trigger className="text-muted block max-w-72 truncate">
                          {file.reason}
                        </Tooltip.Trigger>
                        <Tooltip.Content>
                          <Tooltip.Arrow />
                          {file.reason}
                        </Tooltip.Content>
                      </Tooltip>
                    </Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>
        </Table>
        {files.length === 0 && (
          <div className="text-muted flex min-h-28 items-center justify-center text-sm">
            {t('cleanup.detail.noAvailableFiles')}
          </div>
        )}
      </Card.Content>
      <MigrationDialog
        target={migrationTarget}
        onClose={() => setMigrationTarget(null)}
      />
    </Card>
  )
}

function pathName(path: string) {
  return (
    path
      .replace(/[\\/]+$/, '')
      .split(/[\\/]/)
      .pop() || path
  )
}

function resolvePath(rootPath: string, relativePath: string) {
  if (!rootPath) {
    return relativePath
  }
  if (!relativePath) {
    return rootPath
  }

  const separator = rootPath.includes('\\') ? '\\' : '/'
  const root = rootPath.replace(/[\\/]+$/, '')
  const relative = relativePath
    .replace(/^[\\/]+/, '')
    .replace(/[\\/]+/g, separator)
  return `${root}${separator}${relative}`
}

function levelLabel(level: number) {
  return ['A', 'B', 'C'][level] ?? '未知'
}

function levelColor(
  level: number,
): 'success' | 'warning' | 'danger' | 'default' {
  return (
    (['success', 'warning', 'danger'][level] as
      'success' | 'warning' | 'danger' | undefined) ?? 'default'
  )
}
