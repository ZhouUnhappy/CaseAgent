import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    // Auto-import Element Plus APIs (ElMessage, ElMessageBox, etc.) so call
    // sites don't need `import { ElMessage } from 'element-plus'` boilerplate.
    AutoImport({
      resolvers: [ElementPlusResolver()],
      // include only vue files; we still want explicit imports in plain .js
      // (api/, stores/) so that grep for an import still works.
      include: [/\.vue$/, /\.vue\?vue/],
    }),
    // Auto-import Element Plus components on first use. Tree-shaken: only
    // components actually referenced in templates end up in the bundle, which
    // is what fixes the 966 kB vendor chunk seen in prior builds.
    Components({
      resolvers: [ElementPlusResolver()],
    }),
  ],
  server: {
    port: 40002,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:40003',
        changeOrigin: true,
      },
    },
  },
})
