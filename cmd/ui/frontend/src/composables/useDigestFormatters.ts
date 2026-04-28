import { marked } from 'marked';

const palette = [
  '#f87171', '#fb923c', '#ca9a04', '#4ade80',
  '#2dd4bf', '#60a5fa', '#c084fc', '#f472b6',
  '#a78bfa', '#34d399', '#38bdf8', '#e879f9',
];

function paletteColor(s: string): string {
  let h = 0;
  for (const c of s) h = ((h * 31) + c.charCodeAt(0)) >>> 0;
  return palette[h % palette.length];
}

export function readTag(score: number): string {
  if (score >= 90) return 'Must Read';
  if (score >= 75) return 'Should Read';
  if (score >= 60) return 'May Read';
  if (score > 0)   return 'Optional';
  return 'Unscored';
}

export function readTagStyle(tag: string): { background: string; color: string } {
  switch (tag) {
    case 'Must Read':   return { background: 'rgba(239,68,68,.12)',  color: '#f87171' };
    case 'Should Read': return { background: 'rgba(249,115,22,.12)', color: '#fb923c' };
    case 'May Read':    return { background: 'rgba(234,179,8,.1)',   color: '#ca9a04' };
    case 'Optional':    return { background: 'rgba(30,32,36,1)',     color: '#6b7080' };
    default:            return { background: 'transparent',          color: '#4a4e5c' };
  }
}

export function dupDotStyle(group: string): string {
  return `background:${paletteColor(group)}`;
}

export function sourceDotStyle(source: string): string {
  return `background:${paletteColor(source)}`;
}

export function renderMarkdown(text: string): string {
  if (!text) return '';
  return marked.parse(text) as string;
}

export function useDigestFormatters() {
  return { readTag, readTagStyle, dupDotStyle, sourceDotStyle, renderMarkdown, paletteColor };
}
