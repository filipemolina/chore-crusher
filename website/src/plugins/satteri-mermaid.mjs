import { renderMermaidSVG } from 'beautiful-mermaid';

/**
 * Diagram colors mapped to Starlight's theme tokens so a single SVG
 * adapts to light/dark mode via CSS custom properties.
 */
const DIAGRAM_COLORS = {
  transparent: true,
  fg: 'var(--sl-color-text)',
  line: 'var(--sl-color-gray-4)',
  accent: 'var(--sl-color-accent)',
  muted: 'var(--sl-color-gray-3)',
  surface: 'var(--sl-color-gray-6)',
  border: 'var(--sl-color-gray-5)',
  font: "ui-monospace, 'JetBrains Mono', 'Fira Code', 'Fira Mono', 'Source Code Pro', Menlo, Consolas, monospace",
};

/**
 * Sätteri mdast plugin that renders ```mermaid code blocks to inline SVG
 * at build time, so no mermaid JavaScript ships to the browser.
 */
export function satteriMermaid() {
  return {
    name: 'satteri-mermaid',
    code(node, context) {
      if (node.lang !== 'mermaid') return;

      const file = context?.fileURL?.pathname || 'unknown file';
      try {
        const svg = renderMermaidSVG(node.value, DIAGRAM_COLORS);
        return { type: 'html', value: `<div class="mermaid-diagram">${svg}</div>` };
      } catch (error) {
        throw new Error(
          `satteri-mermaid: failed to render diagram in ${file}: ${error.message}`
        );
      }
    },
  };
}
