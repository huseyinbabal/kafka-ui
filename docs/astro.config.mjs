// @ts-check
import starlight from '@astrojs/starlight'
import { defineConfig } from 'astro/config'

// Published to GitHub Pages as a project site.
export default defineConfig({
  site: 'https://huseyinbabal.github.io',
  base: '/kafka-ui',
  integrations: [
    starlight({
      title: 'kafka-ui',
      description: 'A fast, single-binary web UI for Apache Kafka.',
      logo: { src: './src/assets/logo.svg' },
      favicon: '/favicon.svg',
      customCss: ['@fontsource-variable/geist', './src/styles/custom.css'],
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/huseyinbabal/kafka-ui' }],
      editLink: { baseUrl: 'https://github.com/huseyinbabal/kafka-ui/edit/main/docs/' },
      lastUpdated: true,
      sidebar: [
        {
          label: 'Getting started',
          items: [
            { label: 'Quick start', slug: 'getting-started/quick-start' },
            { label: 'Kubernetes', slug: 'getting-started/kubernetes' },
            { label: 'Configuration', slug: 'getting-started/configuration' },
          ],
        },
        {
          label: 'Features',
          items: [
            { label: 'Clusters and access', slug: 'features/clusters-and-access' },
            { label: 'Topics and messages', slug: 'features/topics-and-messages' },
            { label: 'Consumer groups', slug: 'features/consumer-groups' },
            { label: 'Schema Registry', slug: 'features/schema-registry' },
            { label: 'ACLs', slug: 'features/acls' },
            { label: 'Analytics', slug: 'features/analytics' },
            { label: 'Live topology', slug: 'features/live-topology' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Security', slug: 'guides/security' },
            { label: 'Development', slug: 'guides/development' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'HTTP API', slug: 'reference/api' },
            { label: 'Architecture', slug: 'reference/architecture' },
          ],
        },
      ],
    }),
  ],
})
