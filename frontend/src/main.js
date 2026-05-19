import { createApp } from 'vue'
import { createPinia } from 'pinia'

// Element Plus is registered on-demand by unplugin-vue-components +
// unplugin-auto-import (see vite.config.js). We deliberately do NOT call
// app.use(ElementPlus) — that would re-introduce the full vendor chunk.
// Imperative services (ElMessage, ElMessageBox, ElNotification, ElLoading)
// are function calls — the resolver only auto-imports styles for tags it
// finds in templates, so we explicitly import their CSS here.
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/notification/style/css'
import 'element-plus/es/components/loading/style/css'
import './style.css'
import App from './App.vue'
import router from './router'

const app = createApp(App)
app.use(createPinia())
app.use(router)

app.config.errorHandler = (err, _instance, info) => {
  // 兜底：未被 try/catch 捕获的渲染/生命周期错误
  // eslint-disable-next-line no-console
  console.error('[app-error]', info, err)
}

app.mount('#app')
