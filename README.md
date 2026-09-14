# Bythos — tu biblioteca en tu PC (app de escritorio + extensión)

Guarda links de YouTube y artículos, organízalos por carpetas, marca tu avance y repasa.
Todo vive en tu disco (`bythos.db` en tu carpeta de configuración). Sin cuentas, sin nube:
Google solo ve lo que TÚ le pegas al repasar. Nada sale solo.

**Stack:** Go + SQLite (memoria y cerebro) · React + CSS con Vite y pnpm (cara) · Extensión Chrome MV3 (brazo)

## Mapa (dónde vive cada cosa y por qué)

```
bythos/
  desktop/                        → el programa descargable (.exe de 1 archivo)
    main.go                       → director: abre .db + incrusta UI + prende :8080
    db/                           → memoria (SQL solo aquí): db.go, folders.go, resources.go, progreso.go
    api/                          → cerebro (HTTP, sin SQL): server.go (11 rutas),
                                    metadata.go (olfato), ui.go (sirve dist), export.go (repaso)
    ui/                           → cara: package.json, vite.config.js, index.html,
                                    src/{api.js, App.jsx, main.jsx, styles.css}
  extension/                      → brazo: manifest.json + popup.html + popup.js
```

Reglas que cumplimos: `main` flaco · SQL solo en `db/` · la UI y la extensión nunca tocan el
`.db` (hablan HTTP) · la extensión manda solo `{url}` y Go huele título/imagen/tipo.

## Cómo correrlo

Requisitos: Go 1.25+ y (para la UI) Node 18+ con pnpm 9.

```powershell
# 1. Cerebro + memoria → http://localhost:8080/api/salud = {"ok":true}
cd desktop
go run .

# 2. UI en desarrollo → http://localhost:5173 (proxy a :8080, cero cambios de código)
cd desktop/ui
pnpm install
pnpm dev

# 3. Descargable de 1 archivo (UI DENTRO del .exe, una sola puerta :8080)
cd desktop/ui
pnpm build
cd ..
go build -o bythos.exe .
.\bythos.exe
# Abre http://localhost:8080/ → tu React. /api/... sigue viva.
```

Sin `pnpm build`, `/` avisa en español qué hacer (no un 404 mudo).

## Extensión (guardar sin copiar links)

1. Prende la app (`go run .` o el `.exe`): `localhost:8080` eres tú mismo, debe estar vivo.
2. `chrome://extensions` → modo desarrollador → "Cargar descomprimida" → carpeta `extension/`.
3. En cualquier video/artículo pulsa 💾 **Guardar esta página** (elige tu carpeta ID, la ves en la app).
4. Si la app está apagada, el popup te lo dice ("Abre primero tu app") en vez de fallar mudo.

Permisos mínimos a propósito: `activeTab` (solo la pestaña clicada) + `storage` (tu carpeta
favorita) + `http://localhost:8080/*` (solo tu PC, nadie más).

## API local (toda en localhost:8080, sin login: es tu PC)

| Método | Ruta | Hace |
|---|---|---|
| GET | /api/salud | ¿sigo vivo? |
| GET/POST | /api/carpetas | listar / crear `{nombre}` |
| GET | /api/carpetas/{id}/progreso | `{total, completados, pendientes, en_curso, porcentaje}` |
| GET | /api/recursos?carpeta_id= | listar (0 o vacío = todos) |
| POST | /api/recursos | guardar `{carpeta_id, url}` — Go huele título/imagen/tipo |
| PATCH | /api/recursos/{id} | estado `{estado: pendiente\|en_curso\|completado}` |
| DELETE | /api/recursos/{id} | borrar |
| GET | /api/stats | progreso general (gráfico portada) |
| GET | /api/carpetas/{id}/export?format= | `markdown` (notas) · `gemini` (pegar en la IA) · `notebooklm` (importar) · `drive` (descarga `.md`) |
| GET | / | tu React (o aviso si falta `pnpm build`) |

## Repaso (a dónde va cada export)

- **Markdown**: lista `[título](url)` para tus notas/Obsidian (botón Copiar en la UI).
- **Gemini**: tus links + 3 órdenes (resumen, 10 preguntas, plan 7 días) para pegar en `gemini.google.com`.
- **NotebookLM**: pasos + URLs una por una para `notebooklm.google.com`.
- **Drive**: mismo Markdown como archivo `bythos-<carpeta>.md` (el navegador lo descarga, tú lo subes).

## Tests

```powershell
cd desktop
go vet ./...   # compilación sana
go test ./api/ # export (puro, sin DB) en milisegundos
```

## Estado: v1 terminada ✅

Guardar con olfato + carpetas + estados + % y barra + export 4 formatos + UI + extensión +
`.exe` single-file + tests. Todo probado clase a clase.
Lujos futuros (nada bloquea usarla): tags `json:` minúsculas · borrar carpetas en UI ·
barra % por carpeta · instalador con icono.
