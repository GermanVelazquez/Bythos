# Iconos de Bythos

`icon.svg` es la fuente del icono de la extensión y el favicon web: cuadrado
negro redondeado, B blanca en negrita y ondas finas en los contadores. Los PNG
(`icon-512.png`, `icon-256.png`, `../extension/icons/`,
`../desktop/ui/public/favicon.png`) se generan desde ese SVG con
`node gen-icons.mjs` (script puro con solo stdlib, sin dependencias).

## Logo de la app (`bythos-logo.jpg` + `bythos.ico`)

`bythos-logo.jpg` es el logo oficial de la app (la B blanca sobre fondo
oscuro). Es un asset fuente: **sí se commitea**. El `.jpg` original del dueño
no se toca: solo se copió al repo.

`bythos.ico` es el icono multi-tamaño (16/32/48/256) derivado de ese JPG, también
commiteado como asset fuente. Se genera con Go puro (solo stdlib, sin
dependencias): recorte cuadrado al centro + reescala con filtro de caja + PNGs
empaquetados en contenedor ICO (válido desde Windows Vista). El script vivía en
Temp y no quedó en el repo; para regenerar basta cualquier conversor
(`magick bythos-logo.jpg -define icon:auto-resize=16,32,48,256 bythos.ico`)
manteniendo los 4 tamaños.

## Icono del .exe (`desktop/versioninfo.json` + `desktop/resource.syso`)

El binario lleva el icono EMBEBIDO con `goversioninfo` (genera `.syso` sin
pedir gcc, que no hay en esta PC):

```powershell
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
cd desktop
goversioninfo   # lee versioninfo.json, escribe resource.syso
go build -o bythos.exe .
```

`versioninfo.json` es la fuente (incluye `IconPath: ../assets/bythos.ico` más
`FileDescription: Bythos` y versiones). `resource.syso` **sí se commitea** para
que `go build` funcione sin instalar nada extra (build reproducible); al
cambiar el icono o la versión, regeneralo y commitea ambos. `go build` absorbe
solo cualquier `.syso` del paquete en Windows.

Los accesos directos del instalador (`installer/bythos.iss`) usan ese icono
embebido (`IconFilename: {app}\bythos.exe`): no se instala ningún `.ico` junto
a la app.

## Favicon .ico (opcional)

El navegador acepta el `favicon.png` ya cableado en `desktop/ui/index.html`, así
que el `.ico` web es opcional. Si lo quieres igual, abre `assets/icon-256.png`
en Paint y guárdalo como `desktop/ui/public/favicon.ico`, o con ImageMagick:
`magick icon-256.png -define icon:auto-resize=16,32,48 favicon.ico`. Después
agrega `<link rel="icon" href="/favicon.ico">` en `index.html`.
