// The Skill Overview tab: the Skill's own SKILL.md. Frontmatter fields
// render in document order, then the Markdown body. The document refetches
// when the Store digest changes so a Synchronization action is reflected.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { fetchSkillContent } from './skill-api'
import type { Skill, SkillContent } from './skill-api'
import { SkillMarkdown } from './skill-markdown'
import { useLocale } from './locale-context'

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillContentTab({ skill }: { skill: Skill }) {
  const { t, getErrorMessage } = useLocale()
  const [content, setContent] = useState<SkillContent | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    setContent(null)
    setError(null)

    fetchSkillContent(skill.id, controller.signal)
      .then((data) => {
        if (inflight.current === controller) setContent(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(err)
      })
  }, [skill.id])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load, skill.store_digest])

  if (error && !content) {
    return (
      <Alert variant="error">
        <AlertTitle>{t('alertCouldNotLoadContent')}</AlertTitle>
        <AlertDescription>{getErrorMessage(error, 'errLoadingSkillContentFailed')}</AlertDescription>
      </Alert>
    )
  }

  if (!content) {
    return <Spinner className="text-3xl text-foreground-muted" aria-label={t('ariaLoadingSkillContent')} />
  }

  return (
    <div className="space-y-6">
      {content.frontmatter.length > 0 && (
        <div className="border border-border rounded-xl p-4 bg-background">
          <h2 className="text-lg font-semibold">{t('titleFrontmatter')}</h2>
          <dl className="mt-3 space-y-2 text-sm">
            {content.frontmatter.map((field) => (
              <div key={field.key}>
                <dt className="font-mono text-xs text-foreground-muted">{field.key}</dt>
                <dd className="mt-0.5 whitespace-pre-wrap break-words">{field.value}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}

      <div className="border border-border rounded-xl p-4 bg-background">
        <div className="font-mono text-sm text-foreground-muted">{content.path}</div>
        {content.body.trim() === '' ? (
          <p className="mt-3 text-sm text-foreground-muted">{t('emptySkillBody')}</p>
        ) : (
          <div className="mt-3">
            <SkillMarkdown body={content.body} />
          </div>
        )}
      </div>
    </div>
  )
}
