import { Badge } from '@appica/ui-react/badge'

// SourceStatusBadge is the operator-facing availability label shared by the
// index table, the master–detail list pane, and the detail page.
export function SourceStatusBadge({ available, stale }: { available: boolean; stale: boolean }) {
  if (available) {
    return <Badge variant="success">Available</Badge>
  }
  return (
    <>
      <Badge variant="error">Unavailable</Badge>
      {stale && <Badge variant="warning">Stale</Badge>}
    </>
  )
}
