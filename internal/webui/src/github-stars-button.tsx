/*
 * Adapted from GAIA UI's GitHub Stars Button.
 * See ../THIRD_PARTY_NOTICES.md for attribution and license terms.
 */
import { useEffect, useState } from 'react'
import { buttonVariants } from '@appica/ui-react/button'
import { BrandGithub, StarFilled } from '@appica/icons-react'

interface GitHubRepository {
  stargazers_count: number
}

interface GitHubStarsButtonProps {
  repo: string
  accessibleLabel: string
  className?: string
}

export function GitHubStarsButton({ repo, accessibleLabel, className }: GitHubStarsButtonProps) {
  const [starCount, setStarCount] = useState<number | null>(null)

  useEffect(() => {
    let isMounted = true

    async function fetchStars() {
      try {
        const response = await fetch(`https://api.github.com/repos/${repo}`)
        if (!response.ok) {
          throw new Error('Failed to fetch repository data')
        }

        const repository: GitHubRepository = await response.json()
        if (isMounted) {
          setStarCount(repository.stargazers_count)
        }
      } catch {
        if (isMounted) {
          setStarCount(0)
        }
      }
    }

    void fetchStars()

    return () => {
      isMounted = false
    }
  }, [repo])

  const classes = [
    buttonVariants({ variant: 'primary', size: 'md' }),
    'group gap-3 rounded-xl px-3.5 shadow-md shadow-primary/20',
    className,
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <a
      href={`https://github.com/${repo}`}
      target="_blank"
      rel="noopener noreferrer"
      className={classes}
      aria-label={accessibleLabel}
      title={accessibleLabel}
    >
      <span className="flex items-center gap-1.5">
        <BrandGithub data-icon="start" />
        <span>GitHub</span>
      </span>
      <span className="flex items-center gap-1 text-sm" aria-hidden="true">
        <StarFilled className="size-4 text-current transition-colors group-hover:text-yellow-500" />
        <span className="min-w-2 font-medium tabular-nums">{starCount ?? '…'}</span>
      </span>
    </a>
  )
}
