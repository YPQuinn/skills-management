import { Routes, Route, useParams } from 'react-router-dom'
import { GroupsIndex } from './groups-index'
import { GroupDetailPage } from './group-detail'

export { GroupsIndex }
export { GroupDetailPage } from './group-detail'

export function GroupExplorer() {
  const { name } = useParams()
  return <GroupDetailPage key={name} />
}

export default function Groups() {
  return (
    <Routes>
      <Route path="/" element={<GroupsIndex />} />
      <Route path="/:name" element={<GroupExplorer />} />
    </Routes>
  )
}
