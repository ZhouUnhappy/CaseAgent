import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'

import './style.css'
import App from './App.vue'
import router from './router'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus)

app.config.errorHandler = (err, _instance, info) => {
  // 兜底：未被 try/catch 捕获的渲染/生命周期错误
  // eslint-disable-next-line no-console
  console.error('[app-error]', info, err)
}

app.mount('#app')
