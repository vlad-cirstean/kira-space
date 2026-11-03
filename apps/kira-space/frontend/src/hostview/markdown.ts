import { badgeVariants } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import MarkdownIt, { type MarkdownIt as MarkdownItInstance, type Token } from 'markdown-it';
import alerts from 'markdown-it-github-alerts';

// Descriptions and comments come from other people. Raw HTML is never emitted: comments and tags
// drop, the text between inline tags stays, and a block's tags strip to escaped text. Images render
// as chips (a link when https, else text) so nothing loads from a remote host; the host's own
// mention and status links (#kira-mention, #kira-status-<color>) render as chips too.

const SAFE_LINK = /^(?:https?:|mailto:|#)/i;
const COMMENT = /<!--[\s\S]*?-->/g;
const TAG = /<\/?[A-Za-z][^>]*>/g;
const STATUS_HREF = /^#kira-status-(neutral|purple|blue|red|yellow|green)$/;

const STATUS_VARIANT = {
  green: 'ok',
  blue: 'info',
  red: 'error',
  yellow: 'warn',
  purple: 'default',
  neutral: 'default',
} as const;

const CHIP_MENTION = badgeVariants({ variant: 'default' });
const CHIP_IMAGE = `${badgeVariants({ variant: 'default' })} kira-prose-media`;

function chipClass(href: string): string | undefined {
  if (href === '#kira-mention') return CHIP_MENTION;
  const color = STATUS_HREF.exec(href)?.[1] as keyof typeof STATUS_VARIANT | undefined;
  return color ? badgeVariants({ variant: STATUS_VARIANT[color] }) : undefined;
}

function chipLinks(state: { tokens: Token[] }): void {
  for (const block of state.tokens) {
    if (block.type !== 'inline' || !block.children) continue;
    let open: Token | undefined;
    for (const tok of block.children) {
      if (tok.type === 'link_open') {
        const cls = chipClass(String(tok.attrGet('href') ?? ''));
        if (!cls) continue;
        tok.type = 'kira_chip_open';
        tok.tag = 'span';
        tok.attrs = [['class', cls]];
        open = tok;
      } else if (tok.type === 'link_close' && open) {
        tok.type = 'kira_chip_close';
        tok.tag = 'span';
        open = undefined;
      }
    }
  }
}

function createMarkdown(): MarkdownItInstance {
  const md = new MarkdownIt({ html: true, linkify: true, typographer: false, breaks: false });
  md.use(alerts, {
    titles: {
      note: t('space.hostView.markdown.alert.note.title'),
      tip: t('space.hostView.markdown.alert.tip.title'),
      important: t('space.hostView.markdown.alert.important.title'),
      warning: t('space.hostView.markdown.alert.warning.title'),
      caution: t('space.hostView.markdown.alert.caution.title'),
    },
  });
  md.renderer.rules.alert_open = (tokens, idx) => {
    const meta = (tokens[idx] as Token).meta as { title: string; type: string; icon: string };
    return `<div class="markdown-alert markdown-alert-${meta.type}"><p class="markdown-alert-title">${meta.icon}${md.utils.escapeHtml(meta.title)}</p>`;
  };
  md.core.ruler.after('inline', 'kira-chips', chipLinks);

  const defaultValidateLink = md.validateLink.bind(md);
  md.validateLink = (url) => defaultValidateLink(String(url)) && SAFE_LINK.test(String(url).trim());

  md.renderer.rules.html_inline = () => '';
  md.renderer.rules.html_block = (tokens, idx) => {
    const text = (tokens[idx]?.content ?? '').replace(COMMENT, '').replace(TAG, '').trim();
    return text ? `<p>${md.utils.escapeHtml(text)}</p>\n` : '';
  };

  md.renderer.rules.image = (tokens, idx) => {
    const token = tokens[idx];
    const src = String(token?.attrGet('src') ?? '');
    const alt = md.utils.escapeHtml(token?.content || 'image');
    if (!/^https:\/\//i.test(src)) {
      return `<span class="${CHIP_IMAGE}" data-kira-image>${alt}</span>`;
    }
    const href = md.utils.escapeHtml(src);
    return `<a class="${CHIP_IMAGE}" data-kira-image href="${href}" title="${href}">${alt}</a>`;
  };

  md.renderer.rules.link_open = (tokens, idx, options, _env, renderer) => {
    const href = tokens[idx]?.attrGet('href');
    if (href) tokens[idx]?.attrSet('title', href);
    return renderer.renderToken(tokens, idx, options);
  };

  md.renderer.rules.table_open = (tokens, idx, options, _env, renderer) =>
    `<div class="kira-prose-scroll">${renderer.renderToken(tokens, idx, options)}`;
  md.renderer.rules.table_close = (tokens, idx, options, _env, renderer) =>
    `${renderer.renderToken(tokens, idx, options)}</div>\n`;
  return md;
}

let md: MarkdownItInstance | undefined;

export function renderMarkdown(source: string): string {
  md ??= createMarkdown();
  return md.render(source);
}
