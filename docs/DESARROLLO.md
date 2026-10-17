# Desarrollo

Guía técnica de Bythos: cómo correrlo en desarrollo, compilarlo, empaquetarlo
y su arquitectura interna. Si buscas cómo *usar* la app, ve al
[README](../README.md). Para la integración MCP (agentes de IA), ve a
[AGENTES.md](AGENTES.md).

## Requisitos

- **Go 1.25+** (backend + servidor MCP + instalador fallback)
- **Node 18+** con **pnpm 9** (o npm) para la UI
- **Inno Setup 6** solo si vas a compilar el instalador con asistente gráfico
  (`winget install -e --id JRSoftware.InnoSetup`)

## Correr en desarrollo

```powershell
# 1. App completa → http://localhost:8080 (API en /api/salud)
cd desktop
go run .

# 2. UI en modo desarrollo → http://localhost:5173 (proxy a :8080, cero cambios de código)
#    El Go de arriba tiene que arrancar con $env:BYTHOS_DEV="1": sin eso,
#    la guardia rechaza :5173 (en un release es el puerto de cualquier Vite).
cd desktop/ui
pnpm install
pnpm dev

# 3. Ejecutable de 1 archivo (la UI va DENTRO del .exe, una sola puerta :8080)
cd desktop/ui
pnpm build
cd ..
go build -o bythos.exe .
.\bythos.exe
```

> `go run .` y `go build` sin flags dejan la consola visible: ves los logs en
> la terminal. El release se compila con `-ldflags "-H=windowsgui"` (sin
> consola); ahí todo error de arranque muestra un diálogo nativo de Windows
> en vez de morir en silencio (ver `desktop/ventana/ventana.go`: `Error`).

Sin `pnpm build`, la raíz `/` avisa en español qué hacer (no un 404 mudo).

### Nota sobre `BYTHOS_DEV=1`

`conGuardia` (el guardia de Host+Origin en `desktop/api/server.go`) solo
acepta peticiones desde `localhost:5173` cuando esta variable está a `1`.
En un release, `:5173` es el puerto por defecto de cualquier proyecto Vite:
aceptarlo sin esta bandera dejaría que una página ajena corriendo ahí lea tu
biblioteca o abra la terminal del agente.

## Compilar el ejecutable

```powershell
cd desktop/ui
pnpm build
cd ..
go build -o bythos.exe .              # consola visible (debug)
go build -ldflags "-H=windowsgui" -o bythos.exe .   # release, sin consola
```

## Instalador Windows (Inno Setup, recomendado)

El setup con asistente gráfico se compila con Inno Setup 6: bienvenida,
licencia, carpeta destino (`%LocalAppData%\Bythos`, sin admin), icono de
Escritorio opcional, grupo "Bythos" en el menú inicio (app + "Extensión para
Chrome", que abre su carpeta) y desinstalador registrado en el Panel de
control. Además deja la extensión en `Bythos-Extension` en el Escritorio del
usuario, con su guía (`LEEME.txt`): al desinstalar pregunta si borrarla. La
base de datos del usuario NO viaja: se crea sola en `%APPDATA%\Bythos\bythos.db`
al abrir la app por primera vez.

```powershell
# 1. App fresca (la UI va DENTRO del .exe)
cd desktop/ui
npm run build
cd ..
go build -ldflags "-H=windowsgui" -o bythos.exe .
# 2. Compilar el setup (requiere Inno Setup 6)
cd ..\installer
& "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe" bythos.iss
# Sale en installer\Output\Bythos-Setup.exe (pesado, no se commitea, ver .gitignore)
```

Detalles de layout (ver comentarios en `installer/bythos.iss`):
- Instalación por usuario (`PrivilegesRequired=lowest`), sin admin.
- `{userdesktop}` (nunca `{commondesktop}`) resuelve el Escritorio real
  incluso con redirección a OneDrive.
- El icono del `.exe` va embebido (`assets/bythos.ico`); los accesos
  directos lo usan directo, sin instalar un archivo de icono aparte.
- La extensión se instala como archivos sueltos en `Bythos-Extension`
  (lista explícita de archivos en `[Files]`: un `.pem`/`.crx` suelto en
  `extension/` nunca se empaqueta, por construcción).

Instalación silenciosa:

```powershell
Bythos-Setup.exe /SILENT /DIR="C:\Ruta\Bythos"
# sin el icono de Escritorio:
Bythos-Setup.exe /SILENT /DIR="C:\Ruta\Bythos" /MERGETASKS=!desktopicon
```

### Instalador Go (fallback sin dependencias)

Si no puedes instalar Inno Setup, queda `installer/main.go`: Go puro, sin
dependencias (accesos directos vía WSH, que resuelve el Escritorio real
aunque esté redirigido a OneDrive).

```powershell
cd desktop/ui
npm run build
cd ..
go build -ldflags "-H=windowsgui" -o bythos.exe .
Copy-Item bythos.exe ..\installer\payload\bythos.exe -Force
cd ..\installer
go build -o Bythos-Setup.exe .
```

Instalar: doble clic en `Bythos-Setup.exe` (o `Bythos-Setup.exe /S` en
silencio). Probar sin tocar el Escritorio real:
`Bythos-Setup.exe -dir <carpeta-temp> -no-shortcuts -silent`.

## Extensión — carga en desarrollo

1. Abre `chrome://extensions` → activa **modo desarrollador** → **"Cargar
   descomprimida"** → elige la carpeta `extension/` de este repo (en vez de
   la copia de `Bythos-Extension` del Escritorio; son idénticas byte por
   byte).
2. Fija el icono en la barra para tenerlo a mano.
3. Si mueves la carpeta, repite el paso 1 apuntando a la nueva ubicación:
   nada se desengancha solo, solo hay que recargarla.

Permisos mínimos a propósito: `activeTab` (solo la pestaña que clicas) +
`storage` (carpeta favorita) + `http://localhost:8080/*` (solo la PC del
usuario, nadie más).

## Tests

```powershell
cd desktop
go vet ./...  # compilación sana
go test ./... # API + base de datos + agentes + workspace, todo en segundos
```

## Repaso — a dónde va cada export

- **Markdown**: lista `[título](url)` con % para notas/Obsidian (botón
  Copiar en la UI).
- **Gemini**: links + órdenes (resumen, preguntas, plan de 7 días) para
  pegar en `gemini.google.com`.
- **NotebookLM**: pasos + URLs para importar en `notebooklm.google.com`.
- **Drive**: el mismo Markdown como archivo `bythos-<carpeta>.md` (el
  navegador lo descarga, el usuario lo sube).

## API local

Toda en `localhost:8080`, sin login (es la PC del usuario, protegida por el
guardia de Host+Origin, ver Seguridad más abajo).

| Método | Ruta | Hace |
|---|---|---|
| GET | /api/salud | ¿sigo vivo? `{"ok":true,"version":"1.1.0"}` |
| GET/POST | /api/carpetas | listar / crear `{nombre}` |
| DELETE | /api/carpetas/{id} | borrar (con sus recursos) |
| GET | /api/carpetas/{id}/progreso | `{total, completados, porcentaje, promedio}` |
| GET/POST | /api/recursos | listar (`?carpeta_id=`) / guardar `{carpeta_id, url}` |
| PATCH | /api/recursos/{id} | avance `{progreso: 0-100}` o `{estado}` |
| DELETE | /api/recursos/{id} | borrar (y el archivo si quedó sin dueño, ver Archivos abajo) |
| POST | /api/archivos | sube un archivo (multipart: `carpeta_id` + `archivo`) y crea el recurso |
| GET | /api/archivos/{id}/contenido | sirve el archivo (Range/206 para seek de video) |
| GET | /api/archivos/{id}/miniatura | JPEG chico (≤320px) para la tarjeta; 404 si no hay preview |
| GET | /api/archivos/{id}/texto | vista de texto de docx/xlsx/pptx/txt/md; 422 si no se pudo generar |
| GET | /api/stats | avance global (dashboard) |
| GET | /api/carpetas/{id}/export?format= | `markdown` · `gemini` · `notebooklm` · `drive` |
| POST | /api/carpetas/{id}/import-avance | trae el % de vuelta desde el texto repasado |
| GET/POST | /api/agenda | notas del calendario (rango) / crear nota |
| DELETE | /api/agenda/{id} | borrar nota |
| GET | /api/actividad | historial de creación por día (puntos del calendario) |
| POST | /api/agenda/import | importa compromisos desde un bloque Markdown |
| GET | /api/agenda/imports | lista los lotes de MD importados |
| GET/PUT/DELETE | /api/agenda/imports/{id} | ver / editar / borrar un lote importado |
| GET | /api/eventos?limite=&origen= | historial: quién tocó los datos y qué cambió |
| POST | /api/agente/terminal | abre una terminal en el workspace del agente MCP |
| GET | / | la biblioteca (o aviso si falta `pnpm build`) |

Cabeceras de auditoría (opcionales, las usan la UI/extensión/agente): cada
mutación puede mandar `X-Bythos-Origen` (`app` · `extension` · `agente`) y
`X-Bythos-Actor` (nombre libre, p.ej. el cliente MCP conectado) para que
quede etiquetada en `/api/eventos`. Ver `desktop/api/origen.go`.

## Archivos (Paso 0)

Bythos guarda archivos enteros (PDF, video, imagen, documento) como recursos,
no solo un link a ellos — base para una futura app móvil. Ver
`desktop/archivos` (almacén en disco, sin SQL) y `desktop/db/archivos.go`
(metadatos, tabla `archivos`).

- **Dónde viven**: `%APPDATA%\Bythos\archivos\<sha256[:2]>\<sha256>.<ext>`
  (misma base que `bythos.db`, ver `db.CarpetaDatos`). Direccionado por
  contenido: si subes el mismo archivo dos veces (mismo hash), Bythos
  reusa el blob — no lo duplica en disco.
- **Nunca el nombre del cliente**: el nombre que mandó el navegador se
  guarda solo como texto de display (`nombre_original`, sanitizado y
  acotado a 150 caracteres), jamás se usa para construir una ruta. Cierra
  cualquier path traversal de raíz.
- **Streaming**: `POST /api/archivos` lee el multipart con
  `r.MultipartReader()` y escribe a un temporal DENTRO del almacén
  mientras calcula el sha256 al vuelo (nunca vuelca el archivo entero a
  memoria); al terminar, un `os.Rename` atómico (mismo volumen) lo deja
  en su ruta final.
- **Tipo por contenido, lista blanca** (`desktop/archivos`, ver
  `detectarTipo`): PDF (`%PDF-`), MP4/M4V/MOV (caja `ftyp`), WebM/MKV
  (magic EBML), PNG/JPEG/GIF/WEBP, DOCX/XLSX/PPTX (ZIP con
  `[Content_Types].xml` + `word/`/`xl/`/`ppt/`), TXT/MD (UTF-8 válido,
  sin NUL, y que NO empiece pareciendo markup). Todo lo demás se
  rechaza — **HTML y SVG incluidos a propósito**: si Bythos los sirviera
  tal cual, correrían script en el origen de la app. El mime guardado
  sale de la detección, nunca del `Content-Type` que mandó el cliente.
- **Límite**: 2 GiB por archivo (`archivos.TamanoMaximo`), aplicado con
  `http.MaxBytesReader` antes de tocar disco → 413 si se excede.
- **`url` de un recurso de archivo**: vale `"archivo:<id>"` (una forma
  interna, nunca una URL real) — UI/export/MCP la tratan como marcador;
  el contenido real se pide por `GET /api/archivos/{id}/contenido`.
- **Borrado con refcount**: dos recursos pueden compartir un mismo
  archivo (dedupe). Borrar un recurso (o su carpeta, que hace cascade en
  SQL) solo borra el blob + su fila de metadatos si, después, ningún
  otro recurso lo sigue referenciando (`db.ContarRecursosPorArchivo`).

### Previews: miniatura y vista de texto

Extensión del Paso 0 para que la tarjeta y el visor muestren algo más que
un ícono (`desktop/archivos/miniaturas.go` y `desktop/archivos/texto.go`).

- **`GET /archivos/{id}/miniatura`**: JPEG (máx. 320px en el lado largo)
  generado a mano con la std lib (`image`, `image/png`, `image/jpeg`,
  `image/gif`; downscale por promedio de área, sin `x/image`), cacheado
  en `<base>/miniaturas/<sha256>.jpg` (se genera una sola vez) y borrado
  junto con el blob cuando el refcount llega a cero. Solo PNG/JPEG/GIF
  (WEBP no tiene decoder en la std lib de Go); guardia contra
  decompression bombs vía `image.DecodeConfig` (rechaza > 50 megapíxeles
  declarados, sin decodificar los píxeles). Video usa el primer cuadro
  del propio `<video preload="metadata" muted src="...#t=0.1">` en la
  tarjeta, sin trabajo de servidor. Cualquier caso sin preview (tipo sin
  soporte, imagen gigante, decodificación fallida) responde 404 y la UI
  cae al ícono de tipo.
- **`GET /archivos/{id}/texto`**: vista de texto de docx (bloques
  título/párrafo/lista, vía `word/document.xml`), pptx (diapositivas en
  orden numérico, `ppt/slides/slideN.xml`, párrafos `a:t`), xlsx (primera
  hoja como filas de texto, resolviendo `sharedStrings.xml`, tope 200
  filas × 30 columnas) y txt/md (texto plano, tope 300.000 caracteres).
  Lee cada parte del ZIP con `archive/zip` + `encoding/xml` streaming
  (nunca carga el árbol completo), acotado a 20 MiB sin comprimir por
  parte (`io.LimitReader`) como defensa contra zip bombs; al llegar a
  cualquier techo (bytes, bloques, filas) corta y responde
  `truncado: true` en vez de seguir. `encoding/xml` nunca resuelve
  entidades externas (XXE cerrado por diseño de la std lib). Mime no
  soportado o nada extraíble → 422 en español; el visor cae al botón
  Descargar de siempre.

## Seguridad

- **Guardia de Host+Origin, no CORS abierto** (`conGuardia` en
  `desktop/api/server.go`): rechaza cualquier `Host:` que no sea
  `localhost:8080`/`127.0.0.1:8080` (evita DNS rebinding) y cualquier
  `Origin:` que no esté en la lista permitida (evita CSRF desde una pestaña
  cualquiera). Sin `Origin` (curl, navegación same-origin) se deja pasar sin
  cabeceras CORS.
- **`:5173` solo en modo dev**: aceptado únicamente con `BYTHOS_DEV=1` (ver
  arriba). En release, ese puerto es el default de cualquier proyecto Vite
  ajeno.
- **Anti-clickjacking en toda respuesta**: `X-Frame-Options: DENY` +
  `Content-Security-Policy: frame-ancestors 'none'`, incluidos los 403/429.
  Nadie puede meter Bythos en un `<iframe>` ajeno.
- **`POST /api/agente/terminal` con Origin EXACTO**: a diferencia del resto
  de la API (que acepta el prefijo `chrome-extension://` de la extensión),
  esta ruta lanza un proceso en la PC del usuario, así que exige
  `http://localhost:8080` exacto — la extensión nunca puede pedirla. Ver
  `desktop/api/agente.go`.
- **Guardia SSRF en metadata** (`desktop/api/metadata.go`): al guardar una
  URL, Bythos descarga la página para adivinar título/imagen/descripción.
  El `Dialer` personalizado bloquea, por la IP ya resuelta (no por el
  hostname, para cerrar el TOCTOU de DNS rebinding): loopback, redes
  privadas RFC1918 + ULA IPv6, link-local (incluido el endpoint de metadata
  de nube `169.254.169.254`), CGNAT `100.64.0.0/10`, multicast y sin
  especificar.
- **Límite de body JSON de 1 MB** (`limiteBodyJSON` en `server.go`): todo
  body que la API decodifica pasa por `http.MaxBytesReader`, para que un
  cliente hostil o con un bug no pueda tirar el proceso mandando gigabytes
  antes de que el JSON siquiera se valide.
- **Archivos: whitelist por contenido + headers al servir** (ver sección
  Archivos arriba): HTML/SVG nunca se guardan (correrían script en el
  origen de la app), el mime guardado sale de la detección por bytes
  (nunca del `Content-Type` del cliente), y `GET /api/archivos/{id}/contenido`
  responde con `X-Content-Type-Options: nosniff` y
  `Content-Security-Policy: sandbox; default-src 'none'` — el header más
  restrictivo que no rompe el visor nativo de PDF de Edge.

## Mapa (dónde vive cada cosa y por qué)

```
bythos/
  assets/                         → logo e iconos (icon.svg fuente, PNGs, favicon)
  docs/                           → esta guía + AGENTES.md
  desktop/                        → el programa descargable (.exe de 1 archivo)
    main.go                       → director: abre .db + incrusta UI + prende :8080
    db/                           → memoria (SQL solo aquí) + tests
    api/                          → cerebro (HTTP, sin SQL) + tests
    archivos/                     → almacén de archivos en disco (content-addressed, sin SQL) + tests
    agentes/                      → servidor MCP (`bythos.exe mcp`), cliente HTTP delgado, sin SQL
    workspace/                    → prepara %APPDATA%\Bythos\agente (instrucciones + config MCP por cliente)
    terminal/                     → abre wt.exe/cmd.exe en el workspace del agente (sin SQL, sin HTTP)
    ui/                           → cara: React + Vite (src/{api.js, App.jsx, ...})
    ventana/                      → abre la app en modo ventana (Edge --app)
  extension/                      → brazo: manifest.json + popup + content script + iconos
  installer/                      → instalador Inno Setup + fallback en Go puro
```

Reglas que cumplimos: `main` flaco · SQL solo en `db/` · la UI y la
extensión nunca tocan el `.db` (hablan HTTP) · la extensión manda solo
`{url}` y Go detecta título/imagen/tipo · el agente MCP también habla HTTP
con `localhost:8080`, igual que la ventana y la extensión.

## Roadmap

- [x] Guardar con metadata + carpetas + progreso + export 4 formatos + UI + extensión + `.exe` + tests + iconos
- [x] Instalador con icono en el `.exe` (logo Bythos embebido, ver `assets/README.md`)
- [x] Historial de cambios (app/extensión/agente) + conexión de agentes de IA por MCP
- [x] Paso 0: guardar archivos (PDF/video/imagen/documento) como recursos, no solo links — base para la futura app móvil
- [ ] Chequeo de actualizaciones desde la app
- [ ] Tests de UI (vitest)
