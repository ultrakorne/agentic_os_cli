import './styles.css'

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'

if (navigator.userAgent.includes('Mac')) {
  document.documentElement.dataset.platform = 'mac'
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>
)
