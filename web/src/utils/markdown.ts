import { marked, type Tokens } from 'marked'

const SAFE_SCHEME = /^(https?:|mailto:)/i
const BLOCKED_SCHEME = /^(javascript|vbscript|data|file):/i

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  })[char] || char)
}

function isSafeURL(value: string): boolean {
  const href = value.trim()
  if (!href || href.startsWith('#') || href.startsWith('?')) return true
  if (href.startsWith('//')) return false
  if (href.startsWith('/')) return true
  if (BLOCKED_SCHEME.test(href)) return false
  return SAFE_SCHEME.test(href)
}

const safeRenderer = new marked.Renderer()
// Marked parses Markdown but does not sanitize raw HTML.
safeRenderer.html = ({ text }: Tokens.HTML | Tokens.Tag) => escapeHtml(text)
safeRenderer.link = function ({ href, title, tokens }: Tokens.Link) {
  const label = this.parser.parseInline(tokens)
  if (!isSafeURL(href)) return label
  const titleAttr = title ? ` title="${escapeHtml(title)}"` : ''
  return `<a href="${escapeHtml(href)}"${titleAttr} target="_blank" rel="noopener noreferrer">${label}</a>`
}
safeRenderer.image = ({ href, title, text }: Tokens.Image) => {
  if (!isSafeURL(href)) return escapeHtml(text)
  const titleAttr = title ? ` title="${escapeHtml(title)}"` : ''
  return `<img src="${escapeHtml(href)}" alt="${escapeHtml(text)}"${titleAttr} loading="lazy" referrerpolicy="no-referrer">`
}

export function renderMarkdown(value: string): string {
  return marked.parse(value || '', { breaks: true, gfm: true, renderer: safeRenderer }) as string
}
