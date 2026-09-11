// The Skill Overview tab: the Skill's own SKILL.md. Frontmatter fields
// render in document order, then the Markdown body. The document refetches
// when the Store digest changes so a Synchronization action is reflected.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Spinner } from '@appica/ui-react/spinner'
import { Switch } from '@appica/ui-react/switch'
import { Tooltip, TooltipTrigger, TooltipContent } from '@appica/ui-react/tooltip'
import { InfoCircle } from '@appica/icons-react'
import { fetchSkillContent, setSkillModelInvocationDisabled } from './skill-api'
import type { Skill, SkillContent } from './skill-api'
import { SkillMarkdown } from './skill-markdown'
import { useLocale } from './locale-context'

// The disable-model-invocation frontmatter field is a boolean the operator
// toggles from a Switch, not a raw text field; it is lifted out of the
// Attributes list and rendered as its own control.
const DISABLE_MODEL_INVOCATION = 'disable-model-invocation'

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillContentTab({ skill }: { skill: Skill }) {
  const { t, getErrorMessage } = useLocale()
  const [content, setContent] = useState<SkillContent | null>(null)
  const [error, setError] = useState<unknown | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<unknown | null>(null)
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

  const modelInvocationField = content.frontmatter.find((f) => f.key === DISABLE_MODEL_INVOCATION)
  const otherFields = content.frontmatter.filter((f) => f.key !== DISABLE_MODEL_INVOCATION)
  const modelInvocationDisabled = modelInvocationField?.value.trim().toLowerCase() === 'true'

  const onToggleModelInvocation = (checked: boolean) => {
    setSaving(true)
    setSaveError(null)
    setSkillModelInvocationDisabled(skill.id, checked)
      .then((data) => setContent(data))
      .catch((err: unknown) => setSaveError(err))
      .finally(() => setSaving(false))
  }

  return (
    <div className="space-y-6">
      <div className="border border-border rounded-xl p-4 bg-background">
        <h2 className="text-lg font-semibold">{t('titleFrontmatter')}</h2>

        <div className="mt-3 flex items-center gap-2 text-sm">
          <label className="flex items-center gap-2 select-none">
            <Switch
              checked={modelInvocationDisabled}
              onCheckedChange={onToggleModelInvocation}
              disabled={saving}
            />
            <span>{t('labelDisableModelInvocation')}</span>
          </label>
          <Tooltip>
            <TooltipTrigger
              render={
                <span tabIndex={0} className="cursor-help text-foreground-muted">
                  <InfoCircle className="size-4" aria-label={t('labelDisableModelInvocation')} />
                </span>
              }
            />
            <TooltipContent className="max-w-xs">{t('tipDisableModelInvocation')}</TooltipContent>
          </Tooltip>
        </div>
        {saveError != null && (
          <p className="mt-2 text-sm text-error-emphasis">
            {getErrorMessage(saveError, 'errUpdatingSkillFailed')}
          </p>
        )}

        {otherFields.length > 0 && (
          <dl className="mt-3 space-y-2 text-sm">
            {otherFields.map((field) => (
              <div key={field.key}>
                <dt className="font-mono text-xs text-foreground-muted">{field.key}</dt>
                <dd className="mt-0.5 whitespace-pre-wrap break-words">{field.value}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>

      <div className="border border-border rounded-xl p-4 bg-background">
        <h2 className="text-lg font-semibold">{t('titleSkillBody')}</h2>
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
