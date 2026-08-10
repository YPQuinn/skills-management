import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@appica/ui-react/button'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Input } from '@appica/ui-react/input'
import { Field, FieldLabel } from '@appica/ui-react/field'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@appica/ui-react/select'
import { Spinner } from '@appica/ui-react/spinner'
import { createSource, getErrorMessage } from './source-api'

// AddSourceForm registers one Source through the API and opens its detail;
// registration scans the Inventory but never imports Skills.
export function AddSourceForm() {
  const navigate = useNavigate()
  const [kind, setKind] = useState<'local' | 'git'>('local')
  const [location, setLocation] = useState('')
  const [name, setName] = useState('')
  const [ref, setRef] = useState('')
  const [subpath, setSubpath] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const inflight = useRef<AbortController | null>(null)

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
      const body: Record<string, string> = { kind, location: location.trim() }
      if (name.trim()) body.name = name.trim()
      if (kind === 'git' && ref.trim()) body.ref = ref.trim()
      if (subpath.trim()) body.subpath = subpath.trim()
      const created = await createSource(body, controller.signal)
      if (inflight.current !== controller) return

      setLocation('')
      setName('')
      setRef('')
      setSubpath('')
      // open the created Source detail; the list refreshes on return
      navigate(`/sources/${encodeURIComponent(created.name)}`)
    } catch (err: unknown) {
      if (typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError') return
      if (inflight.current === controller) setError(getErrorMessage(err))
    } finally {
      if (inflight.current === controller) setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4 border border-border rounded-xl p-6 bg-background shadow-sm">
      <div>
        <h2 className="text-lg font-semibold">Add a Source</h2>
        <p className="text-sm text-foreground-subtle">Registration scans the Inventory but never imports Skills.</p>
      </div>

      {error && (
        <Alert variant="error">
          <AlertTitle>Registration failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field name="kind">
          <FieldLabel>Kind</FieldLabel>
          <Select value={kind} onValueChange={(v) => { setKind(v as 'local' | 'git'); setError(null) }}>
            <SelectTrigger aria-label="Source kind">
              <SelectValue placeholder="local" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="local">Local directory</SelectItem>
              <SelectItem value="git">Git repository</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field name="name">
          <FieldLabel>Name (optional)</FieldLabel>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Defaults to the location name" />
        </Field>
        <Field name="location" className="sm:col-span-2">
          <FieldLabel>Location</FieldLabel>
          <Input
            value={location}
            onChange={(e) => { setLocation(e.target.value); setError(null) }}
            placeholder={kind === 'local' ? '/absolute/path/to/skills' : 'owner/repo or https://…'}
            required
          />
        </Field>
        {kind === 'git' && (
          <Field name="ref">
            <FieldLabel>Ref (optional)</FieldLabel>
            <Input value={ref} onChange={(e) => setRef(e.target.value)} placeholder="Branch, tag, or commit SHA" />
          </Field>
        )}
        <Field name="subpath">
          <FieldLabel>Subpath (optional)</FieldLabel>
          <Input value={subpath} onChange={(e) => setSubpath(e.target.value)} placeholder="skills" />
        </Field>
      </div>

      <Button type="submit" disabled={loading} focusableWhenDisabled>
        {loading && <Spinner data-icon="start" currentColor />}
        {loading ? 'Scanning…' : 'Register and scan'}
      </Button>
    </form>
  )
}
