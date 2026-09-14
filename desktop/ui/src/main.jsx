import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App.jsx'
import './styles.css' // la ropa global: se importa UNA vez aquí, no en cada componente

// main.jsx — El INTERRUPTOR. 5 líneas, ni una más.
// ¿Por qué existe si App.jsx ya tiene todo?
// Porque React necesita 2 cosas separadas: QUÉ mostrar (App)
// y DÓNDE enchufarlo (el <div id="root"> del index.html).
// Este archivo es el enchufe.

// createRoot = API de React 18 (la vieja ReactDOM.render está jubilada).
// StrictMode = modo profesor: en dev te avisa de malas prácticas
// (efectos dobles, APIs viejas). En el .exe final no pesa nada.
createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
