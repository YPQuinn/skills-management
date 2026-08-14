// The Skill Overview tab: identity facts, Store/Baseline digests, and the
// Source Binding section.
import { Link } from 'react-router-dom'
import type { Skill } from './skill-api'
import { useLocale } from './locale-context'

export function SkillOverview({ skill }: { skill: Skill }) {
  const { t, formatTime } = useLocale()

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 text-sm">
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">{t('labelSlug')}</div>
          <div className="font-mono font-medium mt-0.5">{skill.slug}</div>
        </div>
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">{t('labelCreated')}</div>
          <div className="font-medium mt-0.5">{formatTime(skill.created_at)}</div>
        </div>
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">{t('labelUpdated')}</div>
          <div className="font-medium mt-0.5">{formatTime(skill.updated_at)}</div>
        </div>
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">{t('labelStoreDigest')}</div>
          <div className="font-mono text-xs break-all mt-0.5">{skill.store_digest || '—'}</div>
        </div>
        <div className="border border-border rounded-xl p-4 bg-background">
          <div className="text-foreground-subtle">{t('labelBaselineDigest')}</div>
          <div className="font-mono text-xs break-all mt-0.5">{skill.baseline_digest || '—'}</div>
        </div>
      </div>

      <div className="border border-border rounded-xl p-6 bg-background space-y-4">
        <h2 className="text-lg font-semibold">{t('titleSourceBinding')}</h2>
        {skill.binding ? (
          <div className="grid gap-4 sm:grid-cols-2 text-sm">
            <div>
              <div className="text-foreground-subtle">{t('labelSourceName')}</div>
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
              <div className="text-foreground-subtle">{t('labelRelativeDir')}</div>
              <div className="font-mono font-medium mt-0.5">{skill.binding.relative_dir}</div>
            </div>
            <div>
              <div className="text-foreground-subtle">{t('labelSourceCommit')}</div>
              <div className="font-mono text-xs mt-0.5">{skill.binding.source_commit || '—'}</div>
            </div>
            <div>
              <div className="text-foreground-subtle">{t('labelImportedAt')}</div>
              <div className="font-medium mt-0.5">{formatTime(skill.binding.imported_at)}</div>
            </div>
            <div className="sm:col-span-2">
              <div className="text-foreground-subtle">{t('labelBindingDigest')}</div>
              <div className="font-mono text-xs break-all mt-0.5">{skill.binding.digest}</div>
            </div>
          </div>
        ) : (
          <p className="text-sm text-foreground-subtle">{t('unboundSkillText')}</p>
        )}
      </div>
    </div>
  )
}
