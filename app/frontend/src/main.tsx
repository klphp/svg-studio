import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { initLocale } from './i18n'

// 必须在首次渲染之前读 localStorage 里的语言，
// 否则会先闪一下默认语言再切换。
initLocale()

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
