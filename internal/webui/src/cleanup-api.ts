import { responseError } from './skill-api'
import type { Skill } from './skill-api'
import type { Assignment } from './target-api'
import type { SkillRef, TargetRef } from './group-api'
import type { GroupRef } from './target-api'

export interface CleanupLink {
  target_id: number
  target_name: string
  skill_id: number
  slug: string
  result?: string
  warning?: string
}

export interface TargetImpact {
  id: number
  name: string
  direct: boolean
  groups?: GroupRef[]
}

export interface SourceDeletePreview {
  source_id: number
  name: string
  bound_skills: SkillRef[]
  requires_detach: boolean
}

export interface SkillDeletePreview {
  skill: SkillRef
  groups: GroupRef[]
  targets: TargetImpact[]
  links: CleanupLink[]
  referenced: boolean
}

export interface TargetDeletePreview {
  target: TargetRef
  assignments: Assignment[]
  links: CleanupLink[]
}

export interface GroupDeletePreview {
  group: GroupRef
  targets: TargetRef[]
  assigned: boolean
}

export interface SourceDeleteResult {
  source_id: number
  name: string
  detached: SkillRef[]
}

export interface SkillDeleteResult {
  skill: SkillRef
  removed_assignments: number
  removed_memberships: number
  links: CleanupLink[]
}

export interface TargetDeleteResult {
  target: TargetRef
  links: CleanupLink[]
}

export interface GroupDeleteResult {
  group: GroupRef
  unassigned: TargetRef[]
}

export interface RebindResult {
  skill: Skill
  identical: boolean
}

function mutate<T>(url: string, method: string, body: unknown, fallback: 'errCleanupFailed'): Promise<T> {
  return fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, fallback)
    return (await res.json()) as T
  })
}

export function fetchSourceDeletion(id: number): Promise<SourceDeletePreview> {
  return fetch(`/api/v1/sources/${id}/deletion`).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errCleanupFailed')
    return (await res.json()) as SourceDeletePreview
  })
}

export function deleteSource(id: number, detachSkills: boolean): Promise<SourceDeleteResult> {
  return mutate(`/api/v1/sources/${id}`, 'DELETE', { detach_skills: detachSkills }, 'errCleanupFailed')
}

export function fetchSkillDeletion(id: number): Promise<SkillDeletePreview> {
  return fetch(`/api/v1/skills/${id}/deletion`).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errCleanupFailed')
    return (await res.json()) as SkillDeletePreview
  })
}

export function deleteSkill(id: number, cleanup: boolean): Promise<SkillDeleteResult> {
  return mutate(`/api/v1/skills/${id}`, 'DELETE', { cleanup }, 'errCleanupFailed')
}

export function detachSkill(id: number): Promise<Skill> {
  return mutate(`/api/v1/skills/${id}/detach`, 'POST', {}, 'errCleanupFailed')
}

export function rebindSkill(id: number, sourceId: number, relativeDir: string): Promise<RebindResult> {
  return mutate(`/api/v1/skills/${id}/rebind`, 'POST', { source_id: sourceId, relative_dir: relativeDir }, 'errCleanupFailed')
}

export function fetchTargetDeletion(id: number): Promise<TargetDeletePreview> {
  return fetch(`/api/v1/targets/${id}/deletion`).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errCleanupFailed')
    return (await res.json()) as TargetDeletePreview
  })
}

export function deleteTarget(id: number): Promise<TargetDeleteResult> {
  return mutate(`/api/v1/targets/${id}`, 'DELETE', {}, 'errCleanupFailed')
}

export function fetchGroupDeletion(id: number): Promise<GroupDeletePreview> {
  return fetch(`/api/v1/groups/${id}/deletion`).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errCleanupFailed')
    return (await res.json()) as GroupDeletePreview
  })
}

export function deleteGroup(id: number, unassign: boolean): Promise<GroupDeleteResult> {
  return mutate(`/api/v1/groups/${id}`, 'DELETE', { unassign }, 'errCleanupFailed')
}

export function namesList(items: { name?: string; slug?: string }[]): string {
  return items
    .map((i) => i.name || i.slug || '')
    .filter(Boolean)
    .join(', ')
}
