import { useParams } from 'react-router-dom'
import { SkillDetailPage } from './skill-detail'
import { SkillListPane } from './skill-list-pane'

export { SkillsIndex } from './skills-index'
export { SkillDetailPage } from './skill-detail'
export { SkillListPane } from './skill-list-pane'
export type { Skill, SkillBinding } from './skill-api'

export function SkillExplorer() {
  const { slug } = useParams()
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)] lg:items-start">
      <div className="hidden lg:block">
        <SkillListPane />
      </div>
      <SkillDetailPage key={slug} />
    </div>
  )
}
