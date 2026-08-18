import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { satteri } from '@astrojs/markdown-satteri';
import { satteriMermaid } from './src/plugins/satteri-mermaid.mjs';

export default defineConfig({
  site: 'https://farol.dev',
  markdown: {
    processor: satteri({ mdastPlugins: [satteriMermaid()] }),
  },
  integrations: [
    starlight({
      title: 'farol',
      description: 'A keyboard-first task manager for humans and AI agents — TUI, CLI, and one SQLite store.',
      defaultLocale: 'en',
      logo: {
        src: './src/assets/farol-icon.svg',
        alt: 'farol',
        replacesTitle: false,
      },
      plugins: [],
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        {
          label: 'User Guide',
          items: [{ autogenerate: { directory: 'users' } }],
        },
        {
          label: 'Contributor Guide',
          items: [{ autogenerate: { directory: 'contributors' } }],
        },
      ],
    }),
  ],
});
