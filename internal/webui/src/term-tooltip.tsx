import { Tooltip, TooltipTrigger, TooltipContent } from '@appica/ui-react/tooltip'

export function TermTooltip({ term, tip }: { term: string; tip: string }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span tabIndex={0} className="cursor-help underline decoration-dotted underline-offset-4">
            {term}
          </span>
        }
      />
      <TooltipContent className="max-w-xs">{tip}</TooltipContent>
    </Tooltip>
  )
}
