import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import { initializeTheme } from './composables/useTheme'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import ContentState from './components/ContentState.vue'
import TableSkeleton from './components/TableSkeleton.vue'
import router from './router'

initializeTheme()

const app = createApp(App)
app.component('ContentState', ContentState)
app.component('TableSkeleton', TableSkeleton)

// Register Element Plus icons
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

app.mount('#app')
