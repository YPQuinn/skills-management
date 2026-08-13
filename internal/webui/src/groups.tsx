import { useParams } from 'react-router-dom'
import { GroupDetailPage } from './group-detail'

export { GroupsIndex } from './groups-index'
export { GroupDetailPage } from './group-detail'

export function GroupExplorer() {
  const { name } = useParams()
  return <GroupDetailPage key={name} />
}
