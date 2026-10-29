import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './fonts.css'
import './theme.css'
import App from './App.tsx'
import I18nProvider from './I18nProvider.tsx'
import { trackHistory } from './shell/nav.ts'

trackHistory()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </StrictMode>,
)
