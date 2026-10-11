import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { consolePreset } from './styles/tokens'
import './styles/theme.css'
import './styles/admin.css'
import '@fontsource-variable/dm-sans'
import '@fontsource/ibm-plex-mono/latin-400.css'

createApp(App)
  .use(createPinia())
  .use(router)
  .use(PrimeVue, {
    theme: { preset: consolePreset, options: { darkModeSelector: '[data-theme="dark"]' } },
  })
  .mount('#app')
