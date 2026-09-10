import { Link } from 'react-router-dom'
import { Badge } from '@appica/ui-react/badge'
import { Button } from '@appica/ui-react/button'
import { Checkbox } from '@appica/ui-react/checkbox'
import { Spinner } from '@appica/ui-react/spinner'
import { Tooltip, TooltipTrigger, TooltipContent } from '@appica/ui-react/tooltip'
import { Sparkles, Folder, Link as LinkIcon, Unlink, Trash, Check, AlertTriangle } from '@appica/icons-react'
import { StatusLabel } from './status-label'
import { useLocale } from './locale-context'
import type { SkillRow } from './target-skills-model'

export type RowAction = 'link' | 'unlink' | 'adopt' | 'remove'

interface TargetSkillRowProps {
  row: SkillRow
  selected: boolean
  disabled: boolean
  busyAction: RowAction | null
  onSelect: (checked: boolean) => void
  onLink: () => void
  onUnlink: () => void
  onAdopt: () => void
  onRemove: () => void
}

function OriginChips({ row }: { row: SkillRow }) {
  const { t } = useLocale()
  return (
    <span className="flex flex-wrap items-center gap-1">
      {row.direct && (
        <Badge variant="soft">
          <Sparkles className="size-3 mr-1 inline" />
          {t('originDirect')}
        </Badge>
      )}
      {row.groups.map((g) => (
        <Link key={g} to={`/groups/${encodeURIComponent(g)}`}>
          <Badge variant="outline">
            <Folder className="size-3 mr-1 inline" />
            {g}
          </Badge>
        </Link>
      ))}
    </span>
  )
}

export function TargetSkillRow({
  row,
  selected,
  disabled,
  busyAction,
  onSelect,
  onLink,
  onUnlink,
  onAdopt,
  onRemove,
}: TargetSkillRowProps) {
  const { t } = useLocale()
  const canAdopt = !row.linked && row.observed === 'conflict' && row.adoptable

  return (
    <div className="flex items-center gap-3 rounded-lg border border-border px-3 py-2">
      <Checkbox checked={selected} disabled={disabled} onCheckedChange={onSelect} aria-label={t('ariaSelectSkill', { name: row.name })} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="flex flex-wrap items-center gap-2">
          <Link
            to={`/skills/${encodeURIComponent(row.slug)}`}
            className="font-medium text-foreground-strong underline decoration-border underline-offset-2 hover:decoration-foreground"
          >
            {row.name}
          </Link>
          {row.observed === 'conflict' && (
            <StatusLabel icon={AlertTriangle} tone="error">
              {t('observedConflict')}
            </StatusLabel>
          )}
          {row.observed === 'broken_link' && (
            <StatusLabel icon={Unlink} tone="warning">
              {t('observedBrokenLink')}
            </StatusLabel>
          )}
        </span>
        <OriginChips row={row} />
      </span>

      {row.linked ? (
        <Button variant="outline" size="sm" disabled={disabled} focusableWhenDisabled onClick={onUnlink}>
          {busyAction === 'unlink' ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <Unlink data-icon="start" />}
          {busyAction === 'unlink' ? t('btnUnlinking') : t('btnUnlink')}
        </Button>
      ) : canAdopt ? (
        <Button variant="soft" size="sm" disabled={disabled} focusableWhenDisabled onClick={onAdopt}>
          {busyAction === 'adopt' ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <Check data-icon="start" />}
          {busyAction === 'adopt' ? t('btnAdopting') : t('btnAdopt')}
        </Button>
      ) : (
        <Button variant="soft" size="sm" disabled={disabled} focusableWhenDisabled onClick={onLink}>
          {busyAction === 'link' ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <LinkIcon data-icon="start" />}
          {busyAction === 'link' ? t('btnLinking') : t('btnLink')}
        </Button>
      )}

      {row.groups.length > 0 ? (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="ghost"
                size="sm"
                disabled={disabled}
                focusableWhenDisabled
                onClick={onRemove}
                aria-label={t('ariaRemoveSkill', { name: row.name })}
                className="text-error-emphasis hover:text-error-emphasis"
              >
                <Trash className="size-4" />
              </Button>
            }
          />
          <TooltipContent>{t('removeGroupDialogTitle')}</TooltipContent>
        </Tooltip>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled}
          focusableWhenDisabled
          onClick={onRemove}
          aria-label={t('ariaRemoveSkill', { name: row.name })}
          className="text-error-emphasis hover:text-error-emphasis"
        >
          <Trash className="size-4" />
        </Button>
      )}
    </div>
  )
}
