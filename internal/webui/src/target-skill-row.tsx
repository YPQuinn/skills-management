import { Button } from '@appica/ui-react/button'
import { Checkbox } from '@appica/ui-react/checkbox'
import { Spinner } from '@appica/ui-react/spinner'
import { Tooltip, TooltipTrigger, TooltipContent } from '@appica/ui-react/tooltip'
import { Link as LinkIcon, Unlink, Trash, Check, AlertTriangle } from '@appica/icons-react'
import { StatusLabel } from './status-label'
import { SkillCardIdentity, type SkillCardSource } from './skill-identity'
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

function rowSources(row: SkillRow, originDirect: string): SkillCardSource[] {
  const sources: SkillCardSource[] = []
  if (row.direct) sources.push({ label: originDirect })
  for (const group of row.groups) {
    sources.push({ label: group, href: `/groups/${encodeURIComponent(group)}` })
  }
  return sources
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
    <div className="flex items-center gap-2.5 rounded-lg border border-border px-3 py-2.5">
      <Checkbox checked={selected} disabled={disabled} onCheckedChange={onSelect} aria-label={t('ariaSelectSkill', { name: row.name })} />
      <SkillCardIdentity
        name={row.name}
        slug={row.slug}
        sources={rowSources(row, t('originDirect'))}
        trailing={
          row.observed === 'conflict' || row.observed === 'broken_link' ? (
            <span className="ml-1.5 inline-flex items-center">
              {row.observed === 'conflict' ? (
                <StatusLabel icon={AlertTriangle} tone="error">
                  {t('observedConflict')}
                </StatusLabel>
              ) : (
                <StatusLabel icon={Unlink} tone="warning">
                  {t('observedBrokenLink')}
                </StatusLabel>
              )}
            </span>
          ) : null
        }
      />

      {row.linked ? (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="ghost"
                size="sm"
                disabled={disabled}
                focusableWhenDisabled
                onClick={onUnlink}
                aria-label={busyAction === 'unlink' ? t('btnUnlinking') : t('ariaUnlinkSkill', { name: row.name })}
              >
                {busyAction === 'unlink' ? <Spinner currentColor className="size-4" /> : <Unlink className="size-4" />}
              </Button>
            }
          />
          <TooltipContent>{busyAction === 'unlink' ? t('btnUnlinking') : t('btnUnlink')}</TooltipContent>
        </Tooltip>
      ) : canAdopt ? (
        <Button variant="soft" size="sm" disabled={disabled} focusableWhenDisabled onClick={onAdopt}>
          {busyAction === 'adopt' ? <Spinner data-icon="start" currentColor className="text-[1.2em]" /> : <Check data-icon="start" />}
          {busyAction === 'adopt' ? t('btnAdopting') : t('btnAdopt')}
        </Button>
      ) : (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="ghost"
                size="sm"
                disabled={disabled}
                focusableWhenDisabled
                onClick={onLink}
                aria-label={busyAction === 'link' ? t('btnLinking') : t('ariaLinkSkill', { name: row.name })}
              >
                {busyAction === 'link' ? <Spinner currentColor className="size-4" /> : <LinkIcon className="size-4" />}
              </Button>
            }
          />
          <TooltipContent>{busyAction === 'link' ? t('btnLinking') : t('btnLink')}</TooltipContent>
        </Tooltip>
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
