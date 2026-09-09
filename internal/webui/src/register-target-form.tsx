import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Spinner } from '@appica/ui-react/spinner'
import { Plus } from '@appica/icons-react'
import { registerTarget, type TargetAdapter, type CreateTargetInput } from './target-api'
import { useLocale } from './locale-context'

interface RegisterTargetFormProps {
  adapters: TargetAdapter[]
  onRegistered?: () => void
}

export function RegisterTargetForm({ adapters, onRegistered }: RegisterTargetFormProps) {
  const navigate = useNavigate()
  const { t, getErrorMessage } = useLocale()

  const [mode, setMode] = useState<'builtin' | 'custom'>('builtin')
  const [name, setName] = useState('')
  const [adapter, setAdapter] = useState(adapters.length > 0 ? adapters[0].key : '')
  const [scope, setScope] = useState<'user' | 'project'>('user')
  const [projectRoot, setProjectRoot] = useState('')
  const [customPath, setCustomPath] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<unknown | null>(null)

  const inflight = useRef<AbortController | null>(null)

  useEffect(() => {
    if (!adapter && adapters.length > 0) setAdapter(adapters[0].key)
  }, [adapters, adapter])

  useEffect(() => {
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    setLoading(true)
    setError(null)

    try {
      let input: CreateTargetInput
      if (mode === 'builtin') {
        input = {
          name: name.trim(),
          adapter: adapter || (adapters[0]?.key ?? ''),
          scope,
          project_root: scope === 'project' ? projectRoot.trim() : undefined,
        }
      } else {
        input = {
          name: name.trim(),
          path: customPath.trim(),
        }
      }

      const created = await registerTarget(input, controller.signal)
      if (inflight.current !== controller) return

      setName('')
      setProjectRoot('')
      setCustomPath('')

      if (onRegistered) {
        onRegistered()
      } else {
        navigate(`/targets/${encodeURIComponent(created.name)}`)
      }
    } catch (err: unknown) {
      if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
      if (inflight.current === controller) setError(err)
    } finally {
      if (inflight.current === controller) setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error !== null && (
        <Alert variant="error">
          <AlertTitle>{t('alertAddTargetFailed')}</AlertTitle>
          <AlertDescription>{getErrorMessage(error, 'errAddingTargetFailed')}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field name="mode">
          <FieldLabel>{t('labelTargetKind')}</FieldLabel>
          <Select
            value={mode}
            onValueChange={(v) => setMode(v as 'builtin' | 'custom')}
            items={{ builtin: t('optBuiltinAdapter'), custom: t('optCustomTarget') }}
          >
            <SelectTrigger aria-label={t('labelTargetKind')}>
              <SelectValue placeholder={t('optBuiltinAdapter')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="builtin">{t('optBuiltinAdapter')}</SelectItem>
              <SelectItem value="custom">{t('optCustomTarget')}</SelectItem>
            </SelectContent>
          </Select>
        </Field>

        <Field name="name">
          <FieldLabel>{t('labelTargetName')}</FieldLabel>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              setError(null)
            }}
            placeholder={t('phTargetName')}
            required
          />
        </Field>

        {mode === 'builtin' ? (
          <>
            <Field name="adapter">
              <FieldLabel>{t('labelSelectAdapter')}</FieldLabel>
              <Select
                value={adapter}
                onValueChange={(val) => setAdapter(val as string)}
                items={Object.fromEntries(adapters.map((a) => [a.key, a.name]))}
              >
                <SelectTrigger aria-label={t('labelSelectAdapter')}>
                  <SelectValue placeholder={t('labelSelectAdapter')} />
                </SelectTrigger>
                <SelectContent>
                  {adapters.map((a) => (
                    // The item's text slot only sizes to its content, so grow it to let the badge sit flush right.
                    <SelectItem key={a.key} value={a.key} className="[&>*:first-child]:grow">
                      <span className="grow">{a.name}</span>
                      {a.detection.status === 'detected' && (
                        <Badge variant="success" size="sm" className="shrink-0">
                          {t('statusInstalled')}
                        </Badge>
                      )}
                    </SelectItem>
                  ))}
                  {adapters.length === 0 && <SelectItem value="">—</SelectItem>}
                </SelectContent>
              </Select>
            </Field>

            <Field name="scope">
              <FieldLabel>{t('labelScope')}</FieldLabel>
              <Select
                value={scope}
                onValueChange={(v) => setScope(v as 'user' | 'project')}
                items={{ user: t('optScopeUser'), project: t('optScopeProject') }}
              >
                <SelectTrigger aria-label={t('labelScope')}>
                  <SelectValue placeholder={t('optScopeUser')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">{t('optScopeUser')}</SelectItem>
                  <SelectItem value="project">{t('optScopeProject')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>

            {scope === 'project' && (
              <Field name="projectRoot" className="sm:col-span-2">
                <FieldLabel>{t('labelProjectRoot')}</FieldLabel>
                <Input
                  value={projectRoot}
                  onChange={(e) => setProjectRoot(e.target.value)}
                  placeholder={t('phProjectRoot')}
                  required
                />
              </Field>
            )}
          </>
        ) : (
          <Field name="customPath" className="sm:col-span-2">
            <FieldLabel>{t('labelCustomPath')}</FieldLabel>
            <Input
              value={customPath}
              onChange={(e) => setCustomPath(e.target.value)}
              placeholder={t('phCustomPath')}
              required
            />
          </Field>
        )}
      </div>

      <Button type="submit" disabled={loading} focusableWhenDisabled>
        {loading ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <Plus data-icon="start" />}
        {loading ? t('btnAddingTarget') : t('btnSubmitTarget')}
      </Button>
    </form>
  )
}
