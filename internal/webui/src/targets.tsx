import { Routes, Route, useParams } from 'react-router-dom'
import { TargetsIndex } from './targets-index'
import { TargetDetailPage } from './target-detail'

export { TargetsIndex }
export { TargetDetailPage } from './target-detail'

export function TargetExplorer() {
  const { name } = useParams()
  return <TargetDetailPage key={name} />
}

export default function Targets() {
  return (
    <Routes>
      <Route path="/" element={<TargetsIndex />} />
      <Route path="/:name" element={<TargetExplorer />} />
    </Routes>
  )
}
