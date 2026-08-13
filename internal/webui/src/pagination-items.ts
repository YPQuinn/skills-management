export function paginationItems(current: number, total: number): (number | 'gap')[] {
  const pages = Array.from(new Set([1, total, current - 1, current, current + 1]))
    .filter((page) => page >= 1 && page <= total)
    .sort((a, b) => a - b)
  const items: (number | 'gap')[] = []
  let previous = 0
  for (const page of pages) {
    if (page - previous > 1) items.push('gap')
    items.push(page)
    previous = page
  }
  return items
}
