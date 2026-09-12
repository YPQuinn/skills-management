import { WorldCode } from '@appica/icons-react'
import claudeCodeLogo from '@lobehub/icons-static-svg/icons/claudecode-color.svg'
import codexLogo from '@lobehub/icons-static-svg/icons/codex-color.svg'
import cursorLogo from '@lobehub/icons-static-svg/icons/cursor.svg'
import geminiCliLogo from '@lobehub/icons-static-svg/icons/geminicli-color.svg'
import githubCopilotLogo from '@lobehub/icons-static-svg/icons/githubcopilot.svg'
import openCodeLogo from '@lobehub/icons-static-svg/icons/opencode.svg'
import piLogo from '@lobehub/icons-static-svg/icons/pi.svg'

const agentLogos: Record<string, { src: string; monochrome?: boolean }> = {
  'claude-code': { src: claudeCodeLogo },
  codex: { src: codexLogo },
  cursor: { src: cursorLogo, monochrome: true },
  'gemini-cli': { src: geminiCliLogo },
  opencode: { src: openCodeLogo, monochrome: true },
  pi: { src: piLogo, monochrome: true },
  'github-copilot': { src: githubCopilotLogo, monochrome: true },
}

interface AgentLogoProps {
  adapterKey: string
  className?: string
}

export function AgentLogo({ adapterKey, className = 'size-4.5' }: AgentLogoProps) {
  if (adapterKey === 'universal') {
    return (
      <WorldCode
        aria-hidden
        data-agent-logo={adapterKey}
        data-icon="start"
        className={`${className} shrink-0`}
      />
    )
  }

  const logo = agentLogos[adapterKey]
  if (!logo) return null

  return (
    <img
      src={logo.src}
      alt=""
      aria-hidden
      data-agent-logo={adapterKey}
      data-icon="start"
      className={`${className} shrink-0 object-contain${logo.monochrome ? ' dark:invert' : ''}`}
    />
  )
}
