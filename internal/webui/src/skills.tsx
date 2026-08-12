import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertTitle, AlertDescription } from '@appica/ui-react/alert'
import { Badge } from '@appica/ui-react/badge'
import { ScrollArea } from '@appica/ui-react/scroll-area'
import { Spinner } from '@appica/ui-react/spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@appica/ui-react/table'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@appica/ui-react/tabs'
import { fetchSkills, fetchSkill, getErrorMessage } from './skill-api'
import type { Skill } from './skill-api'
import { SkillListPane } from './skill-list-pane'

export { SkillListPane } from './skill-list-pane'
export type { Skill, SkillBinding } from './skill-api'

function formatTime(value?: string): string {
  if (!value) return 'never'
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

function isAbortError(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { name?: unknown }).name === 'AbortError'
}

export function SkillsIndex() {
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    setLoading(true)
    setError(null)
    fetchSkills(controller.signal)
      .then((data) => {
        if (inflight.current === controller) setSkills(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(getErrorMessage(err))
      })
      .finally(() => {
        if (inflight.current === controller) setLoading(false)
      })
  }, [])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Skills</h1>
          <p className="text-foreground-subtle text-sm">Authoritative local collection of managed Agent Skills.</p>
        </div>
      </div>

      {error && (
        <Alert variant="error">
          <AlertTitle>Could not load Skills</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {loading && skills === null ? (
        <div className="flex justify-center py-12">
          <Spinner className="text-3xl text-foreground-subtle" aria-label="Loading skills" />
        </div>
      ) : skills === null ? null : skills.length === 0 ? (
        <div className="rounded-xl border border-border bg-background p-8 text-center space-y-3">
          <p className="text-foreground-subtle">No Skills in local Store yet.</p>
          <p className="text-sm text-foreground-subtle">Register a Source and import Skills to get started.</p>
          <Link
            to="/sources"
            className="inline-block text-sm font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            Go to Sources →
          </Link>
        </div>
      ) : (
        <ScrollArea className="w-full" orientation="horizontal">
          <div className="min-w-[700px]">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead>Source Binding</TableHead>
                  <TableHead>Updated</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {skills.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell className="font-medium text-foreground-strong">
                      <Link
                        to={`/skills/${encodeURIComponent(s.slug)}`}
                        className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                      >
                        {s.name}
                      </Link>
                    </TableCell>
                    <TableCell className="font-mono text-foreground-subtle">{s.slug}</TableCell>
                    <TableCell className="text-foreground-subtle">{s.description}</TableCell>
                    <TableCell>
                      {s.binding ? (
                        <div className="flex flex-col text-xs">
                          <Link
                            to={`/sources/${encodeURIComponent(s.binding.source_name)}`}
                            className="font-medium underline decoration-border underline-offset-2 hover:decoration-foreground"
                          >
                            {s.binding.source_name}
                          </Link>
                          <span className="font-mono text-foreground-subtle">{s.binding.relative_dir}</span>
                        </div>
                      ) : (
                        <Badge variant="outline" className="text-xs">
                          unbound
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-foreground-subtle text-xs">{formatTime(s.updated_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </ScrollArea>
      )}
    </div>
  )
}

export function SkillDetailPage() {
  const { slug } = useParams()
  const [skill, setSkill] = useState<Skill | null>(null)
  const [error, setError] = useState<string | null>(null)
  const inflight = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    if (!slug) return
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller

    setSkill(null)
    setError(null)

    fetchSkill(slug, controller.signal)
      .then((data) => {
        if (inflight.current === controller) setSkill(data)
      })
      .catch((err: unknown) => {
        if (isAbortError(err)) return
        if (inflight.current === controller) setError(getErrorMessage(err))
      })

    return controller
  }, [slug])

  useEffect(() => {
    load()
    return () => {
      const active = inflight.current
      inflight.current = null
      active?.abort()
    }
  }, [load])

  if (error && !skill) {
    return (
      <Alert variant="error">
        <AlertTitle>Could not load Skill</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  if (!skill) {
    return <Spinner className="text-3xl text-foreground-subtle" aria-label="Loading skill" />
  }

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/skills"
          className="text-sm text-foreground-subtle underline decoration-border underline-offset-2 hover:decoration-foreground"
        >
          ← All Skills
        </Link>
        <div className="flex items-start justify-between gap-4 mt-2">
          <div>
            <h1 className="text-2xl font-bold">{skill.name}</h1>
            <p className="font-mono text-sm text-foreground-subtle">{skill.slug}</p>
          </div>
          {skill.binding ? (
            <Badge variant="success">Bound</Badge>
          ) : (
            <Badge variant="outline">Unbound</Badge>
          )}
        </div>
        <p className="mt-2 text-foreground-subtle">{skill.description}</p>
      </div>

      <Tabs defaultValue="overview" variant="line">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="synchronization" disabled>
            Synchronization (coming soon)
          </TabsTrigger>
          <TabsTrigger value="distribution" disabled>
            Distribution (coming soon)
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="pt-4 space-y-6">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 text-sm">
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">Slug</div>
              <div className="font-mono font-medium mt-0.5">{skill.slug}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">Created</div>
              <div className="font-medium mt-0.5">{formatTime(skill.created_at)}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">Updated</div>
              <div className="font-medium mt-0.5">{formatTime(skill.updated_at)}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">Store Digest</div>
              <div className="font-mono text-xs break-all mt-0.5">{skill.store_digest || '—'}</div>
            </div>
            <div className="border border-border rounded-xl p-4 bg-background">
              <div className="text-foreground-subtle">Baseline Digest</div>
              <div className="font-mono text-xs break-all mt-0.5">{skill.baseline_digest || '—'}</div>
            </div>
          </div>

          <div className="border border-border rounded-xl p-6 bg-background space-y-4">
            <h2 className="text-lg font-semibold">Source Binding</h2>
            {skill.binding ? (
              <div className="grid gap-4 sm:grid-cols-2 text-sm">
                <div>
                  <div className="text-foreground-subtle">Source Name</div>
                  <div className="font-medium mt-0.5">
                    <Link
                      to={`/sources/${encodeURIComponent(skill.binding.source_name)}`}
                      className="underline decoration-border underline-offset-2 hover:decoration-foreground"
                    >
                      {skill.binding.source_name}
                    </Link>
                  </div>
                </div>
                <div>
                  <div className="text-foreground-subtle">Relative Directory</div>
                  <div className="font-mono font-medium mt-0.5">{skill.binding.relative_dir}</div>
                </div>
                <div>
                  <div className="text-foreground-subtle">Source Commit</div>
                  <div className="font-mono text-xs mt-0.5">{skill.binding.source_commit || '—'}</div>
                </div>
                <div>
                  <div className="text-foreground-subtle">Imported At</div>
                  <div className="font-medium mt-0.5">{formatTime(skill.binding.imported_at)}</div>
                </div>
                <div className="sm:col-span-2">
                  <div className="text-foreground-subtle">Binding Digest</div>
                  <div className="font-mono text-xs break-all mt-0.5">{skill.binding.digest}</div>
                </div>
              </div>
            ) : (
              <p className="text-sm text-foreground-subtle">
                This Skill is unbound. It was imported or created without an active Source association.
              </p>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

export function SkillExplorer() {
  const { slug } = useParams()
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)] lg:items-start">
      <div className="hidden lg:block">
        <SkillListPane />
      </div>
      <SkillDetailPage key={slug} />
    </div>
  )
}
