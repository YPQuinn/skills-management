import { ApiError, type DictionaryKey } from './locale-dictionary'
import type { SkillRef } from './group-api'

export interface AdapterDetection {
  status: 'detected' | 'not_detected' | 'unknown' | 'not_applicable' | string
  evidence: string[]
  detected_at: string
}

export interface TargetAdapter {
  key: string
  name: string
  detection: AdapterDetection
}

export interface TargetAdapterListResponse {
  items: TargetAdapter[]
  total: number
}

export interface TargetSummary {
  id: number
  name: string
  path: string
  adapter: string
  scope: string
  project_root?: string
  created_at: string
  updated_at: string
}

export interface TargetListResponse {
  items: TargetSummary[]
  total: number
}

export interface GroupRef {
  id: number
  name: string
}

export interface Assignment {
  id: number
  kind: 'skill' | 'group'
  skill?: SkillRef
  group?: GroupRef
  created_at: string
}

export interface DesireReason {
  assignment_id: number
  kind: 'skill' | 'group'
  group_id?: number
  group_name?: string
}

export interface DesiredSkill {
  id: number
  slug: string
  name: string
  reasons: DesireReason[]
}

export interface TargetView extends TargetSummary {
  compatible_adapters?: string[]
  direct_skills: Assignment[]
  groups: Assignment[]
  desired_skills: DesiredSkill[]
}

export interface CreateTargetInput {
  name: string
  adapter?: string
  scope?: string
  project_root?: string
  path?: string
}

export interface CreateAssignmentInput {
  kind: 'skill' | 'group'
  skill_id?: number
  group_id?: number
}

interface ErrorEnvelope {
  error?: { message?: string; code?: string }
}

async function responseError(
  res: Response,
  fallbackKey: DictionaryKey,
  params?: Record<string, string | number>,
): Promise<ApiError> {
  let serverMessage: string | undefined
  try {
    const data = (await res.json()) as ErrorEnvelope
    if (data?.error?.message) {
      serverMessage = data.error.message
    }
  } catch {}
  return new ApiError(serverMessage, fallbackKey, params || { status: res.status })
}

export function fetchTargetAdapters(signal?: AbortSignal): Promise<TargetAdapter[]> {
  return fetch('/api/v1/targets/adapters', { signal })
    .then(async (res) => {
      if (!res.ok) throw await responseError(res, 'errServerResponded', { status: res.status })
      return (await res.json()) as TargetAdapterListResponse
    })
    .then((data) => data.items || [])
}

export function fetchTargets(signal?: AbortSignal): Promise<TargetSummary[]> {
  return fetch('/api/v1/targets', { signal })
    .then(async (res) => {
      if (!res.ok) throw await responseError(res, 'errServerResponded', { status: res.status })
      return (await res.json()) as TargetListResponse
    })
    .then((data) => data.items || [])
}

export function registerTarget(input: CreateTargetInput, signal?: AbortSignal): Promise<TargetView> {
  return fetch('/api/v1/targets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errRegisteringTargetFailed')
    return (await res.json()) as TargetView
  })
}

export function fetchTarget(id: number | string, signal?: AbortSignal): Promise<TargetView> {
  return fetch(`/api/v1/targets/${id}`, { signal }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errTargetNotFound')
    return (await res.json()) as TargetView
  })
}

export function createAssignment(targetId: number | string, input: CreateAssignmentInput, signal?: AbortSignal): Promise<Assignment> {
  return fetch(`/api/v1/targets/${targetId}/assignments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errAddingAssignmentFailed')
    return (await res.json()) as Assignment
  })
}

export function deleteAssignment(targetId: number | string, assignmentId: number, signal?: AbortSignal): Promise<TargetView> {
  return fetch(`/api/v1/targets/${targetId}/assignments/${assignmentId}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errDeletingAssignmentFailed')
    return (await res.json()) as TargetView
  })
}
