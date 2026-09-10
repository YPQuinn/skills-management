// The merged Target Skills view is skill-level: each desired Skill is one
// row, carrying where it came from (a direct Assignment and/or Groups) and
// its current Managed Link state. Group Assignments are expanded into their
// member Skills here rather than shown as separate rows.
import type { DesiredSkill } from './target-api'
import type { DistributionStatus } from './distribution-api'

export interface SkillRow {
  id: number
  name: string
  slug: string
  direct: boolean
  groups: string[] // distinct Group names that contribute this Skill
  assignmentIds: number[] // every Assignment backing this Skill (direct + groups)
  linked: boolean
  observed: string // 'linked' | 'missing' | 'conflict' | 'broken_link' | ''
  adoptable: boolean
}

export function buildSkillRows(desired: DesiredSkill[], status: DistributionStatus | null): SkillRow[] {
  const bySkill = new Map(status?.items?.map((it) => [it.skill_id, it]) ?? [])
  const rows = desired.map((d) => {
    const groups: string[] = []
    const assignmentIds: number[] = []
    let direct = false
    for (const r of d.reasons) {
      if (!assignmentIds.includes(r.assignment_id)) assignmentIds.push(r.assignment_id)
      if (r.kind === 'skill') direct = true
      else if (r.group_name && !groups.includes(r.group_name)) groups.push(r.group_name)
    }
    const item = bySkill.get(d.id)
    return {
      id: d.id,
      name: d.name,
      slug: d.slug,
      direct,
      groups,
      assignmentIds,
      linked: item?.observed === 'linked',
      observed: item?.observed ?? '',
      adoptable: item?.adoptable ?? false,
    }
  })
  return rows.sort((a, b) => a.name.localeCompare(b.name))
}
