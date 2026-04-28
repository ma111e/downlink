import {fileURLToPath, URL} from 'node:url'
import vue from '@vitejs/plugin-vue'
// import autoprefixer from 'autoprefixer'
// import tailwind from 'tailwindcss'
import tailwindcss from '@tailwindcss/vite'
import {defineConfig} from 'vite'

// https://vite.dev/config/
export default defineConfig({
    // css: {
    //     postcss: {
    //         plugins: [],
    //     },
    // },
    plugins: [
        vue(),
        tailwindcss(),
    ],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url))
        }
    }
})
