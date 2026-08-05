import { mount } from 'svelte'
import './style.css'
import { appearance } from './lib/stores/appearance.svelte'
import App from './App.svelte'

appearance.init()

const mountTarget = document.getElementById('app')
if (!mountTarget) {
  throw new Error('missing #app mount target')
}

const app = mount(App, {
  target: mountTarget,
})

export default app
