import {
  Button,
  Card,
  Checkbox,
  Input,
  Label,
  Link,
  ListBox,
  Select,
  TextArea,
  toast,
} from '@heroui/react'
import {
  Bot,
  ExternalLink,
  LoaderCircle,
  PlugZap,
  Save,
  SlidersHorizontal,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { setting } from '../../../wailsjs/go/models'
import {
  ListSettings,
  SaveSettings,
  TestLLMConnection,
} from '../../../wailsjs/go/main/App'
import ControlledNextUIFormWrapper from '@/components/ControlledNextUIFormWrapper'
import { showDialog } from '@/components/DialogProvider'
import { toastError } from '@/util/toast-error'
import i18n, {
  changeLanguage,
  supportedLanguages,
  type SupportedLanguage,
} from '@/i18n'

type SettingValues = {
  language: SupportedLanguage
  llmSecret: string
  llmURL: string
  llmModel: string
  llmMaxToken: string
  llmAutoContextCompress: string
  llmExtraBody: string
  recordMaxCount: string
}

const initialValues: SettingValues = {
  language: i18n.resolvedLanguage as SupportedLanguage,
  llmSecret: '',
  llmURL: '',
  llmModel: '',
  llmMaxToken: '50000',
  llmAutoContextCompress: 'false',
  llmExtraBody: '',
  recordMaxCount: '10',
}

const hasWailsRuntime = () =>
  typeof window !== 'undefined' && 'go' in window && 'runtime' in window

function valuesFromSettings(
  settings: setting.Setting[],
  language: SupportedLanguage,
): SettingValues {
  const values = new Map(
    settings.map((setting) => [setting.key, setting.value]),
  )
  return {
    language,
    llmSecret: values.get('llm.secret') ?? '',
    llmURL: values.get('llm.url') ?? '',
    llmModel: values.get('llm.model') ?? '',
    llmMaxToken: values.get('llm.max-token') ?? '50000',
    llmAutoContextCompress: values.get('llm.auto-context-compress') ?? 'false',
    llmExtraBody: values.get('llm.extra-body') ?? '',
    recordMaxCount: values.get('record.max.count') ?? '10',
  }
}

function settingsFromValues(values: SettingValues): setting.Setting[] {
  return [
    { key: 'llm.secret', value: values.llmSecret },
    { key: 'llm.url', value: values.llmURL.trim() },
    { key: 'llm.model', value: values.llmModel.trim() },
    { key: 'llm.max-token', value: values.llmMaxToken.trim() },
    {
      key: 'llm.auto-context-compress',
      value: values.llmAutoContextCompress,
    },
    { key: 'llm.extra-body', value: values.llmExtraBody.trim() },
    { key: 'record.max.count', value: values.recordMaxCount.trim() },
  ]
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [loadError, setLoadError] = useState('')
  const { control, getValues, reset, trigger } = useForm<SettingValues>({
    defaultValues: initialValues,
  })

  const load = useCallback(async () => {
    if (!hasWailsRuntime()) {
      setIsLoading(false)
      return
    }
    setIsLoading(true)
    setLoadError('')
    try {
      reset(
        valuesFromSettings(
          (await ListSettings()) ?? [],
          i18n.resolvedLanguage as SupportedLanguage,
        ),
      )
    } catch (reason) {
      setLoadError(reason instanceof Error ? reason.message : String(reason))
      toastError(reason, t('settings.loadFailed'))
    } finally {
      setIsLoading(false)
    }
  }, [reset])

  useEffect(() => {
    void load()
  }, [load])

  const save = async () => {
    if (!(await trigger())) {
      return
    }

    const values = getValues()
    setIsSaving(true)
    try {
      await SaveSettings(settingsFromValues(values))
      reset(values)
      toast('设置已保存', {
        description: '配置已更新',
        variant: 'success',
      })
    } catch (reason) {
      toastError(reason, '保存设置失败')
    } finally {
      setIsSaving(false)
    }
  }

  const testConnection = async () => {
    if (
      !(await trigger([
        'llmSecret',
        'llmURL',
        'llmModel',
        'llmMaxToken',
        'llmExtraBody',
      ]))
    ) {
      return
    }

    try {
      const result = await TestLLMConnection(settingsFromValues(getValues()))
      console.log(result.type)
      console.log(result.type === 'warning')
      if (result.type === 'warning') {
        setTimeout(() => {
          showDialog({
            title: 'Warning',
            message: t(result.i18nMessage),
            confirmBtnText: t('common.confirm'),
            color: 'warning',
            hideCancel: true,
          })
        }, 100)
        return
      }
      toast(t('settings.testConnection.success'), {
        description: t('settings.testConnection.successDescription'),
        variant: 'success',
      })
    } catch (reason) {
      toastError(reason, t('settings.testConnection.failed'))
    }
  }

  const confirmTestConnection = () => {
    showDialog({
      title: t('settings.testConnection.confirmTitle'),
      message: t('settings.testConnection.confirmMessage'),
      confirmBtnText: t('settings.testConnection.confirm'),
      onConfirm: testConnection,
      color: 'warning',
    })
  }

  if (isLoading) {
    return (
      <div className="text-muted flex min-h-80 items-center justify-center gap-2 text-sm">
        <LoaderCircle aria-hidden="true" className="animate-spin" size={18} />
        {t('common.loading')}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <Card className="rounded-3xl">
        <Card.Header className="flex-row items-start gap-4">
          <span className="bg-accent/10 text-accent inline-flex size-11 shrink-0 items-center justify-center rounded-2xl">
            <SlidersHorizontal aria-hidden="true" size={20} />
          </span>
          <div>
            <Card.Title>{t('settings.common')}</Card.Title>
            <Card.Description>{t('settings.commonDes')}</Card.Description>
          </div>
        </Card.Header>
        <Card.Content className="space-y-3">
          <ControlledNextUIFormWrapper
            control={control}
            label={t('settings.language.label')}
            name="language"
          >
            {(field) => (
              <Select
                selectedKey={field.value}
                variant="secondary"
                onSelectionChange={(key) => {
                  const language = key as SupportedLanguage
                  field.onChange(language)
                  void changeLanguage(language)
                }}
              >
                <Select.Trigger className="rounded-xl">
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {supportedLanguages.map((language) => (
                      <ListBox.Item
                        key={language}
                        id={language}
                        textValue={t(`settings.language.${language}`)}
                      >
                        {t(`settings.language.${language}`)}
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            )}
          </ControlledNextUIFormWrapper>
          <ControlledNextUIFormWrapper
            control={control}
            description={t('settings.maxCleaningRecordDesc')}
            label={t('settings.maxCleaningRecord')}
            name="recordMaxCount"
            rules={{
              validate: (value) => {
                const maxCount = Number(value)
                return (
                  (Number.isSafeInteger(maxCount) && maxCount > 0) ||
                  t('validation.gt0')
                )
              },
            }}
          >
            <Input
              disabled={isSaving}
              inputMode="numeric"
              min={1}
              step={1}
              type="number"
            />
          </ControlledNextUIFormWrapper>
        </Card.Content>
      </Card>

      <Card className="rounded-3xl">
        <Card.Header className="flex-row items-start gap-4">
          <span className="bg-accent/10 text-accent inline-flex size-11 shrink-0 items-center justify-center rounded-2xl">
            <Bot aria-hidden="true" size={20} />
          </span>
          <div>
            <Card.Title>{t('settings.llmService')}</Card.Title>
            <Card.Description>{t('settings.llmServiceDesc')}</Card.Description>
          </div>
        </Card.Header>

        <Card.Content className="grid grid-cols-2 gap-x-6 gap-y-5">
          <div className="col-span-2">
            <ControlledNextUIFormWrapper
              control={control}
              description="LLM Secret"
              label="LLM Secret"
              rules={{ required: true }}
              name="llmSecret"
            >
              <Input disabled={isSaving} type="password" placeholder="sk-xxx" />
            </ControlledNextUIFormWrapper>
          </div>
          <div className="col-span-2">
            <ControlledNextUIFormWrapper
              control={control}
              description="Model API Url"
              label="LLM URL"
              rules={{ required: true }}
              name="llmURL"
            >
              <Input
                disabled={isSaving}
                placeholder="https://api.example.com/v1"
                type="url"
              />
            </ControlledNextUIFormWrapper>
          </div>
          <ControlledNextUIFormWrapper
            control={control}
            description="Modeal name"
            label="LLM Model"
            rules={{ required: true }}
            name="llmModel"
          >
            <Input disabled={isSaving} placeholder="gpt-5" />
          </ControlledNextUIFormWrapper>
          <ControlledNextUIFormWrapper
            control={control}
            description={t('settings.maxTokenInOneConversation')}
            label="LLM Max Token"
            name="llmMaxToken"
            rules={{
              validate: (value) => {
                const maxToken = Number(value)
                return (
                  (Number.isSafeInteger(maxToken) && maxToken > 0) ||
                  t('validation.gt0')
                )
              },
            }}
          >
            <Input
              disabled={isSaving}
              inputMode="numeric"
              min={1}
              step={1}
              type="number"
            />
          </ControlledNextUIFormWrapper>
          <div className="col-span-2">
            <ControlledNextUIFormWrapper
              control={control}
              name="llmAutoContextCompress"
            >
              {(field) => (
                <div className="space-y-2">
                  <Checkbox
                    isDisabled={isSaving}
                    isSelected={field.value === 'true'}
                    onChange={(value) => field.onChange(String(value))}
                  >
                    <Checkbox.Content>
                      <Checkbox.Control>
                        <Checkbox.Indicator />
                      </Checkbox.Control>
                      <Label>{t('settings.autoContextCompress.label')}</Label>
                    </Checkbox.Content>
                  </Checkbox>
                  {field.value === 'true' && (
                    <p className="text-warning text-sm">
                      {t('settings.autoContextCompress.warning')}
                    </p>
                  )}
                </div>
              )}
            </ControlledNextUIFormWrapper>
          </div>
          <div className="col-span-2">
            <ControlledNextUIFormWrapper
              control={control}
              description={t('settings.llmExtraBody.description')}
              label={t('settings.llmExtraBody.label')}
              name="llmExtraBody"
              rules={{
                validate: (value) => {
                  if (!value.trim()) {
                    return true
                  }
                  try {
                    const parsed: unknown = JSON.parse(value)
                    return (
                      (typeof parsed === 'object' &&
                        parsed !== null &&
                        !Array.isArray(parsed)) ||
                      t('settings.llmExtraBody.invalid')
                    )
                  } catch {
                    return t('settings.llmExtraBody.invalid')
                  }
                },
              }}
            >
              <TextArea
                disabled={isSaving}
                placeholder='{"thinking": {"type": "disabled"}}'
              />
            </ControlledNextUIFormWrapper>
          </div>
        </Card.Content>
      </Card>

      <div className="flex items-center justify-between gap-4">
        <div className="text-muted flex items-center gap-2 text-sm">
          {loadError ?? (
            <span className="text-danger">加载设置失败：{loadError}</span>
          )}
        </div>
        <div className="flex items-center gap-3">
          <Button
            className="min-w-28 gap-2 rounded-xl"
            isDisabled={isSaving || Boolean(loadError) || !hasWailsRuntime()}
            variant="secondary"
            onPress={confirmTestConnection}
          >
            <PlugZap aria-hidden="true" size={16} />
            {t('settings.testConnection.button')}
          </Button>
          <Button
            className="min-w-28 gap-2 rounded-xl"
            isDisabled={isSaving || Boolean(loadError) || !hasWailsRuntime()}
            isPending={isSaving}
            variant="primary"
            onPress={() => void save()}
          >
            {isSaving ? (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin"
                size={16}
              />
            ) : (
              <Save aria-hidden="true" size={16} />
            )}
            {isSaving ? t('common.loading') : t('common.save')}
          </Button>
        </div>
      </div>

      <Card className="flex-col items-center rounded-3xl">
        <img src="/github-brands-solid-full.svg" alt="github" width={60} />
        <Link
          aria-label="在 GitHub 打开 vudsen/ai-disk-cleaner"
          className="block"
          href="https://github.com/vudsen/ai-disk-cleaner"
          rel="noreferrer"
          target="_blank"
        >
          <div className="flex items-center gap-1">
            vudsen/ai-disk-cleaner
            <ExternalLink
              aria-hidden="true"
              className="text-muted ml-auto"
              size={16}
            />
          </div>
        </Link>
        <div>v{__APP_VERSION__}</div>
      </Card>
    </div>
  )
}
