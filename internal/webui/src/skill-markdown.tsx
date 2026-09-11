// Renders a Skill's Markdown body with GFM. Body headings shift one level
// down so the page keeps the single h1 rendered for the Skill name. Raw
// HTML stays inert (no rehype-raw) and react-markdown's default
// urlTransform strips unsafe hrefs; links open externally like the other
// outbound links in this UI.
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'

const components: Components = {
  h1: ({ children }) => <h2 className="mt-6 text-xl font-semibold">{children}</h2>,
  h2: ({ children }) => <h3 className="mt-5 text-lg font-semibold">{children}</h3>,
  h3: ({ children }) => <h4 className="mt-4 text-base font-semibold">{children}</h4>,
  h4: ({ children }) => <h5 className="mt-3 text-sm font-semibold">{children}</h5>,
  h5: ({ children }) => <h6 className="mt-3 text-sm font-semibold">{children}</h6>,
  h6: ({ children }) => <h6 className="mt-3 text-sm font-semibold text-foreground-muted">{children}</h6>,
  p: ({ children }) => <p className="mt-3">{children}</p>,
  ul: ({ children }) => <ul className="mt-3 list-disc ps-6">{children}</ul>,
  ol: ({ children }) => <ol className="mt-3 list-decimal ps-6">{children}</ol>,
  li: ({ children }) => <li className="mt-1">{children}</li>,
  a: ({ children, href }) => (
    <a
      href={href}
      target="_blank"
      rel="noreferrer noopener"
      className="underline decoration-border underline-offset-2 hover:decoration-foreground"
    >
      {children}
    </a>
  ),
  code: ({ children, className }) => (
    <code
      className={`rounded bg-background-subtle px-1 py-0.5 font-mono text-xs${className ? ` ${className}` : ''}`}
    >
      {children}
    </code>
  ),
  pre: ({ children }) => (
    <pre className="mt-3 overflow-x-auto rounded-md bg-background-subtle p-3 font-mono text-xs leading-5 [&_code]:bg-transparent [&_code]:p-0">
      {children}
    </pre>
  ),
  blockquote: ({ children }) => (
    <blockquote className="mt-3 border-s-2 border-border ps-4 text-foreground-muted">{children}</blockquote>
  ),
  hr: () => <hr className="my-6 border-border" />,
  table: ({ children }) => <table className="mt-3 w-full border-collapse text-sm">{children}</table>,
  th: ({ children, style }) => (
    <th style={style} className="border border-border bg-background-subtle px-3 py-1.5 text-start font-semibold">
      {children}
    </th>
  ),
  td: ({ children, style }) => (
    <td style={style} className="border border-border px-3 py-1.5 align-top">
      {children}
    </td>
  ),
}

export function SkillMarkdown({ body }: { body: string }) {
  return (
    <div className="text-sm [&>:first-child]:mt-0">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {body}
      </ReactMarkdown>
    </div>
  )
}
