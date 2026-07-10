import { defineConfig } from 'vite'
import { fileURLToPath, URL } from 'node:url'

const r = (p: string) => fileURLToPath(new URL(p, import.meta.url))

// One entry per page. The entry KEY becomes the emitted basename, so it must
// match the filenames the Go render code loads (digest.css/js, archive-index.*,
// reports.*, sources.*, swipe.*). Each entry imports its own CSS so Rollup
// co-emits a minified [name].css next to [name].js.
//
// Output lands in the Go package's assets/ dir, which is embedded via
// //go:embed. emptyOutDir is false so the committed PLACEHOLDER survives.
export default defineConfig({
  // Classic JSX transform for the swipe bundle: the source uses React.createElement
  // semantics via namespaced React.* hooks, so no automatic runtime is needed.
  esbuild: {
    jsx: 'transform',
    jsxFactory: 'React.createElement',
    jsxFragment: 'React.Fragment',
  },
  build: {
    outDir: r('../cmd/server/internal/notification/assets'),
    emptyOutDir: false,
    cssCodeSplit: true,
    assetsInlineLimit: 0,
    modulePreload: false,
    minify: 'esbuild',
    rollupOptions: {
      input: {
        'digest': r('./src/digest/main.ts'),
        'archive-index': r('./src/archive/main.ts'),
        'reports': r('./src/reports/main.ts'),
        'sources': r('./src/sources/main.ts'),
        'swipe': r('./src/swipe/main.tsx'),
        // v2 layout. Entry keys nest under v2/ so Rollup emits assets/v2/<page>.{js,css},
        // which the layout-aware Go asset loader resolves for `layout: v2`. Only digest
        // and archive-index are genuinely redesigned (master-detail reader + redesigned
        // archive). reports/sources/swipe have no redesign and share the same per-theme
        // CSS variables, so they intentionally fall back to the default assets (the Go
        // loader resolves assets/v2/<name> then assets/<name>).
        'v2/digest': r('./src/v2/digest/main.ts'),
        'v2/archive-index': r('./src/v2/archive/main.ts'),
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: '[name].js',
        // Route a v2 entry's co-emitted CSS under assets/v2/ so it doesn't collide with
        // the default page CSS of the same basename (digest.css, archive-index.css). The
        // css module's source path disambiguates the two.
        // A v2 entry's co-emitted CSS is attributed to the entry module (e.g.
        // src/v2/digest/main.ts). Route those under assets/v2/ so they don't collide
        // with the default page CSS of the same basename (digest.css, archive-index.css).
        assetFileNames: (info) => {
          const orig = (info as any).originalFileName || (info.originalFileNames && info.originalFileNames[0]) || ''
          if (orig.includes('src/v2/')) return 'v2/[name].[ext]'
          return '[name].[ext]'
        },
      },
    },
  },
})
