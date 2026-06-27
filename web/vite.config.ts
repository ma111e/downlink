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
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: '[name].js',
        assetFileNames: '[name].[ext]',
      },
    },
  },
})
