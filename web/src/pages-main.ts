import App from './StaticApp.svelte'
import './app.css'

const app = new App({
  target: document.getElementById('app')!,
})

export default app
