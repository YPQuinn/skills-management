import { ApiError, type DictionaryKey } from './locale-dictionary'

export interface SkillRef {
  id: number
  slug: string
  name: string
}

export interface TargetRef {
  id: number
  name: string
}

export interface GroupTarget {
  id: number
  name: string
  path: string
  adapter: string
  scope: string
  project_root?: string
  last_result?: string
  stale?: boolean
}

export interface GroupSummary {
  id: number
  name: string
  member_count: number
  created_at: string
  updated_at: string
}

export interface GroupView {
  id: number
  name: string
  created_at: string
  updated_at: string
  members: SkillRef[]
  targets: GroupTarget[]
}

export interface GroupListResponse {
  items: GroupSummary[]
  total: number
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

export function fetchGroups(signal?: AbortSignal): Promise<GroupSummary[]> {
  return fetch('/api/v1/groups', { signal })
    .then(async (res) => {
      if (!res.ok) throw await responseError(res, 'errServerResponded', { status: res.status })
      return (await res.json()) as GroupListResponse
    })
    .then((data) => data.items || [])
}

export function createGroup(name: string, signal?: AbortSignal): Promise<GroupSummary> {
  return fetch('/api/v1/groups', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errCreatingGroupFailed')
    return (await res.json()) as GroupSummary
  })
}

export function fetchGroup(id: number | string, signal?: AbortSignal): Promise<GroupView> {
  return fetch(`/api/v1/groups/${id}`, { signal }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errGroupNotFound')
    return (await res.json()) as GroupView
  })
}

export function addGroupMembers(groupId: number | string, skillIds: number[], signal?: AbortSignal): Promise<GroupView> {
  return fetch(`/api/v1/groups/${groupId}/members`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ skill_ids: skillIds }),
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errAddingMemberFailed')
    return (await res.json()) as GroupView
  })
}

export function removeGroupMember(groupId: number | string, skillId: number, signal?: AbortSignal): Promise<GroupView> {
  return fetch(`/api/v1/groups/${groupId}/members/${skillId}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    signal,
  }).then(async (res) => {
    if (!res.ok) throw await responseError(res, 'errRemovingMemberFailed')
    return (await res.json()) as GroupView
  })
}
