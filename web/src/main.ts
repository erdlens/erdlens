import App from './App.svelte'
import './app.css'
import logoUrl from './assets/logo.svg'

const icon = document.createElement('link')
icon.rel = 'icon'
icon.type = 'image/svg+xml'
icon.href = logoUrl
document.head.appendChild(icon)

const app = new App({
  target: document.getElementById('app')!,
})

export default app
