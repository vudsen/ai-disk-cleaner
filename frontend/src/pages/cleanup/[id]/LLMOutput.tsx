import { Card } from '@heroui/react'
import { LoaderCircle } from 'lucide-react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useTranslation } from 'react-i18next'
import { memo } from 'react'

type LLMOutputProps = {
  isRunning: boolean
  output: string
}

function LLMOutput({ isRunning, output }: LLMOutputProps) {
  const { t } = useTranslation()
  return (
    <Card className="rounded-3xl">
      <Card.Header className="flex-row items-center justify-between">
        <div>
          <Card.Title>{t('cleanup.detail.llmAnalyze')}</Card.Title>
          <Card.Description>
            {t('cleanup.detail.llmAnalyzeDesc')}
          </Card.Description>
        </div>
        {isRunning && (
          <span className="text-accent flex items-center gap-1 text-xs">
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin"
              size={12}
            />
            {t('cleanup.detail.generating')}
          </span>
        )}
      </Card.Header>
      <Card.Content>
        <div className="bg-default/30 text-foreground max-h-128 min-h-56 overflow-y-auto rounded-2xl p-5 text-sm leading-6">
          {isRunning ? (
            <div className="break-words whitespace-pre-wrap">
              {output || t('cleanup.detail.prepareLLMAnalyze')}
            </div>
          ) : (
            <Markdown
              remarkPlugins={[remarkGfm]}
              components={{
                h1: ({ children }) => (
                  <h1 className="mt-6 mb-3 text-2xl font-semibold first:mt-0">
                    {children}
                  </h1>
                ),
                h2: ({ children }) => (
                  <h2 className="mt-5 mb-2 text-xl font-semibold first:mt-0">
                    {children}
                  </h2>
                ),
                h3: ({ children }) => (
                  <h3 className="mt-4 mb-2 text-base font-semibold first:mt-0">
                    {children}
                  </h3>
                ),
                p: ({ children }) => (
                  <p className="my-2 first:mt-0 last:mb-0">{children}</p>
                ),
                a: ({ children, ...props }) => (
                  <a
                    {...props}
                    className="text-accent underline decoration-current/40 underline-offset-4 transition-opacity hover:opacity-80"
                    rel="noreferrer"
                    target="_blank"
                  >
                    {children}
                  </a>
                ),
                ul: ({ children }) => (
                  <ul className="my-2 list-disc space-y-1 pl-6">{children}</ul>
                ),
                ol: ({ children }) => (
                  <ol className="my-2 list-decimal space-y-1 pl-6">
                    {children}
                  </ol>
                ),
                blockquote: ({ children }) => (
                  <blockquote className="border-accent bg-accent/5 text-muted my-3 rounded-r-xl border-l-3 px-4 py-2">
                    {children}
                  </blockquote>
                ),
                code: ({ children, className, ...props }) => (
                  <code
                    {...props}
                    className={`${className ?? ''} bg-default/60 rounded-md px-1.5 py-0.5 font-mono text-[0.9em]`}
                  >
                    {children}
                  </code>
                ),
                pre: ({ children }) => (
                  <pre className="bg-default/60 my-3 overflow-x-auto rounded-xl p-4 font-mono text-xs leading-5 [&>code]:bg-transparent [&>code]:p-0">
                    {children}
                  </pre>
                ),
                hr: () => <hr className="border-divider my-5" />,
                table: ({ children }) => (
                  <div className="border-divider my-3 overflow-x-auto rounded-xl border">
                    <table className="w-full border-collapse text-left">
                      {children}
                    </table>
                  </div>
                ),
                th: ({ children }) => (
                  <th className="bg-default/50 border-divider border-b px-3 py-2 font-medium">
                    {children}
                  </th>
                ),
                td: ({ children }) => (
                  <td className="border-divider border-t px-3 py-2">
                    {children}
                  </td>
                ),
              }}
            >
              {output || t('cleanup.detail.noOutput')}
            </Markdown>
          )}
        </div>
      </Card.Content>
    </Card>
  )
}

export default memo(LLMOutput)
