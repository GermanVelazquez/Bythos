# Registro de cambios

Todos los cambios notables de este proyecto se documentan en este archivo.

El formato sigue [Keep a Changelog](https://keepachangelog.com/es/1.0.0/),
y este proyecto adhiere a [Versionado Semántico](https://semver.org/lang/es/).

## [Unreleased]

Sin cambios pendientes.

## [1.0.3] - 2026-09-18

### Added

- El instalador deja la carpeta `Bythos-Extension` en el Escritorio con
  guía paso a paso (`installer/LEEME-Extension.txt`): activar modo
  desarrollador en `chrome://extensions`, cargar descomprimida y fijar
  el icono B.
- El desinstalador pregunta antes de borrar la carpeta de la extensión,
  para no perderla por accidente.

### Changed

- `README.md`: la extensión ahora se instala desde el Escritorio
  (flujo de 2 minutos actualizado).
- Versión 1.0.3 en `desktop/api/server.go`, `desktop/ui`,
  `extension/manifest.json`, `installer/bythos.iss` y
  `desktop/versioninfo.json`.

## [1.0.2] - 2026-09-18

### Added

- Arranque con espera activa: `desktop/arranque.go` abre la ventana
  apenas el servidor responde en `/api/salud`, con pruebas
  (`desktop/arranque_test.go`).
- Identidad visual en el binario: `desktop/versioninfo.json` +
  `desktop/resource.syso` embeben el logo en `bythos.exe`;
  `installer/bythos.iss` usa `assets/bythos.ico` en el instalador.
- Recursos `assets/bythos-logo.jpg` y `assets/bythos.ico`, con
  `assets/README.md` actualizado.

### Fixed

- Los errores fatales de arranque muestran un diálogo nativo de Windows
  en vez de una consola: si algo falla al iniciar, el mensaje se entiende.
- La ventana se abre en cuanto el servidor está listo (antes podía
  intentarse demasiado pronto).

### Changed

- `desktop/main.go`: arranque delega en el nuevo mecanismo de espera
  y el manejo de errores fatales con diálogo.
- `desktop/ventana/ventana.go`: aviso nativo reutilizable.
- Versión 1.0.2 en `desktop/api/server.go`, `desktop/ui`,
  `extension/manifest.json`, `installer/bythos.iss` y documentación.

## [1.0.1] - 2026-09-18

### Fixed

- Regla de oro de la ventana: la app instalada nunca abre el navegador.
  Si no hay Edge/Chrome, muestra un aviso nativo (`MessageBoxW`, Go puro
  sin CGO) y devuelve error para que `main` lo registre.
- `desktop/ventana/ventana.go` separado en versión inyectable (`abrir`,
  `candidatos`, `textoAviso`) con pruebas en
  `desktop/ventana/ventana_test.go`: el respaldo se prueba sin abrir
  ventanas reales.

### Changed

- `README.md`: botón visible de descarga
  (`Bythos-Setup.exe` desde la última release) con nota de SmartScreen
  e instalación sin permisos de administrador.
- Versión 1.0.1 en `desktop/api/server.go`, `desktop/ui/src/App.jsx`,
  `desktop/ui/package.json`, `extension/manifest.json` e
  `installer/bythos.iss`.

## [1.0.0] - 2026-09-18

Primera versión pública: biblioteca personal 100 % local en tu PC.

### Added

- App de escritorio en Go de un solo `.exe` con la UI embarcada:
  una sola puerta en `http://localhost:8080`, base SQLite local
  (`bythos.db` se crea en `%APPDATA%/Bythos`, no se empaqueta).
- Biblioteca por carpetas temáticas con filtro Todas / En curso /
  Completadas, progreso por recurso (0–100), promedio por carpeta
  y dashboard global al abrir cada carpeta.
- Agenda mensual con importación Markdown, cargas por lotes y
  organización en carpetas por bloque.
- Metadata automática al guardar URLs (título, imagen, tipo:
  video, artículo, otro).
- Repaso con IA por exportación en 4 formatos (notas, Gemini,
  NotebookLM, Drive) e importación de avance de vuelta.
- Extensión de Chrome de 1 clic (permisos mínimos: solo la pestaña
  activa + `http://localhost:8080`) con popup para elegir carpeta.
- Instalador profesional con Inno Setup en español (bienvenida,
  licencia, destino, icono en Escritorio), instalación por usuario
  en `{localappdata}\Bythos` sin administrador, acceso directo en
  menú inicio y desinstalador en Panel de control. Se conserva el
  instalador Go mínimo como respaldo.
- Licencia MIT y CI en GitHub Actions con paso SignPath listo.
- `README.md` presentable con logo, problema que resuelve, guía de
  uso y tabla de API (`/api/salud`, `/api/carpetas`, progreso...).

[Unreleased]: https://github.com/GermanVelazquez/Bythos/compare/v1.0.3...HEAD
[1.0.3]: https://github.com/GermanVelazquez/Bythos/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/GermanVelazquez/Bythos/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/GermanVelazquez/Bythos/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/GermanVelazquez/Bythos/releases/tag/v1.0.0
