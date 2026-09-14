# Iconos de Bythos

`icon.svg` es la fuente: cuadrado negro redondeado, B blanca en negrita y ondas
finas en los contadores. Los PNG (`icon-512.png`, `icon-256.png`,
`../extension/icons/`, `../desktop/ui/public/favicon.png`) se generan desde ese
SVG con `node gen-icons.mjs` (script puro con solo stdlib, sin dependencias).

## Favicon .ico (paso manual)

El navegador acepta el `favicon.png` ya cableado en `desktop/ui/index.html`, así
que el `.ico` es opcional. Si lo quieres igual, abre `assets/icon-256.png` en
Paint y guárdalo como `desktop/ui/public/favicon.ico`, o con ImageMagick:
`magick icon-256.png -define icon:auto-resize=16,32,48 favicon.ico`. Después
agrega `<link rel="icon" href="/favicon.ico">` en `index.html`.

## Icono del .exe (pendiente, sin romper `go build`)

El binario sigue sin icono propio a propósito: incrustarlo pide `rsrc` o
`goversioninfo` (herramienta externa + archivos `.syso`, rompe el build puro sin
CGO). Cuando quieras hacerlo: `go install github.com/akavel/rsrc@latest`,
`rsrc -ico assets/icon-256.png -o desktop/bythos.syso` y `go build` lo absorbe
solo en Windows. Nada de esto está instalado ni cableado hoy.
