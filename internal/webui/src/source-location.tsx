import { DeviceDesktop, BrandGithub, BrandGitlab, BrandGit } from '@appica/icons-react'
import type { SourceSummary } from './source-api'

type SourceLocationProvider = 'github' | 'gitlab' | 'git'

const iconClass = 'mt-px size-4 shrink-0'

// sourceLocationProvider labels a Git location by its exact, case-insensitive
// host. It parses the two location forms Source registration accepts — URLs
// with a scheme (https://, http://, ssh://, …) and scp-like identities
// (git@host:path) — so look-alike hosts (github.com.evil) and paths that only
// mention a host name (example.com/gitlab/repo) are never mislabelled.
function sourceLocationProvider(location: string): SourceLocationProvider {
  const trimmed = location.trim()
  let host: string | null = null
  if (trimmed.includes('://')) {
    try {
      host = new URL(trimmed).hostname
    } catch {
      host = null
    }
  } else {
    const at = trimmed.indexOf('@')
    const colon = trimmed.indexOf(':')
    if (at >= 0 && colon > at) host = trimmed.slice(at + 1, colon)
  }
  if (host === null) return 'git'
  const lower = host.toLowerCase()
  if (lower === 'github.com') return 'github'
  if (lower === 'gitlab.com') return 'gitlab'
  return 'git'
}

export function SourceLocationIcon({ source }: { source: Pick<SourceSummary, 'kind' | 'location'> }) {
  if (source.kind === 'local') {
    return <DeviceDesktop className={iconClass} data-source-provider="local" />
  }
  const provider = sourceLocationProvider(source.location)
  const Icon = provider === 'github' ? BrandGithub : provider === 'gitlab' ? BrandGitlab : BrandGit
  return <Icon className={iconClass} data-source-provider={provider} />
}
