import { useParams } from 'react-router-dom'
import { TargetDetailPage } from './target-detail'

export { TargetsIndex } from './targets-index'
export { TargetDetailPage } from './target-detail'

export function TargetExplorer() {
  const { name } = useParams()
  return <TargetDetailPage key={name} />
}
