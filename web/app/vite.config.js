// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The SPA of Go Uptime.
// - build → ../static, embedded in the Go binary by web/static.go (//go:embed static). The names are stable and have no
//   hash: the static middleware sends no Cache-Control and the fonts carry their version in their names.
// - dev   → port 8081, with /api (and the custom CSS) proxied to the Go server on localhost:8080
//
// index.html is a Go html/template: the server fills the theme, window.config, the favicons and the title. Its links to
// a placeholder ({{ ... }}), to the custom CSS of ui.custom-css and to the fonts only exist at runtime, so Vite must not
// resolve them: they are swapped for markers before Vite reads the HTML and put back after.
const keptLink = /<link\b[^>]*\bhref="(?:\{\{[^"]*\}\}|\/css\/custom\.css|\/fonts\/[^"]*)"[^>]*>/g

function keepGoTemplateLinks() {
  const kept = []
  return [
    {
      name: 'go-uptime-keep-template-links',
      apply: 'build',
      transformIndexHtml: {
        order: 'pre',
        handler(html) {
          kept.length = 0
          return html.replace(keptLink, (link) => `<!--go-uptime-kept-${kept.push(link) - 1}-->`)
        }
      }
    },
    {
      // The markers are restored on the final HTML, after Vite resolved the assets
      name: 'go-uptime-restore-template-links',
      apply: 'build',
      transformIndexHtml: {
        order: 'post',
        handler(html) {
          return html.replace(/<!--go-uptime-kept-(\d+)-->/g, (_, i) => kept[Number(i)])
        }
      }
    }
  ]
}

export default defineConfig({
  plugins: [vue(), ...keepGoTemplateLinks()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    // The sources import components and modules without extension, as Vue CLI allowed
    extensions: ['.mjs', '.js', '.json', '.vue']
  },
  publicDir: 'public',
  build: {
    outDir: '../static',
    emptyOutDir: true,
    assetsDir: '',
    cssCodeSplit: false,
    sourcemap: false,
    rollupOptions: {
      output: {
        entryFileNames: 'js/app.js',
        chunkFileNames: 'js/[name].js',
        assetFileNames: (asset) => {
          const name = asset.names?.[0] || asset.name || ''
          if (name.endsWith('.css')) return 'css/app.css'
          if (/\.(woff2?|ttf|otf)$/.test(name)) return 'fonts/[name][extname]'
          return 'img/[name][extname]'
        },
        // The libraries in js/chunk-vendors.js, like the build of Vue CLI did
        manualChunks: (id) => (id.includes('node_modules') ? 'chunk-vendors' : undefined)
      }
    }
  },
  server: {
    port: 8081,
    strictPort: true,
    proxy: {
      '^/(api|css|oidc)': { target: 'http://localhost:8080', changeOrigin: true, secure: false }
    }
  }
})
