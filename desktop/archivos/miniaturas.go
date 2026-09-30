package archivos

// miniaturas.go — Preview chico (JPEG) para las tarjetas de recurso, SOLO
// para imágenes (png/jpeg/gif). Video usa el propio <video> con #t=0.1 en
// el frontend (sin trabajo de servidor, ver Archivos.jsx); PDF y
// documentos muestran solo el ícono de tipo. WEBP queda afuera a
// propósito: la librería estándar de Go no trae decoder (image/webp no
// existe en el std lib, y el enunciado pide "no new third-party
// dependencies"), así que una miniatura de WEBP simplemente no es
// posible sin agregar una dependencia — el caller (api/) lo traduce a
// 404 y la UI cae sola al ícono de tipo.
//
// Downscale hecho a mano (promedio de área, ver escalar): nada de
// x/image/draw ni otra librería de resize, todo con image/color del std
// lib. Cacheado en Base()/miniaturas/<sha256>.jpg, mismo criterio de
// "content-addressed" que el resto del almacén: se genera una sola vez
// por archivo, nunca por request.

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // registra el decoder GIF (init() de este paquete); no se llama gif.Decode directo, image.Decode lo despacha solo
	"image/jpeg"  // encoder de salida (jpeg.Encode) + registra el decoder JPEG
	_ "image/png" // registra el decoder PNG, mismo criterio que gif arriba
	"io"
	"os"
	"path/filepath"
)

// ladoMaximoMiniatura es el techo del lado más largo de la miniatura
// generada: 320px alcanza de sobra para una tarjeta de ~200px de ancho
// incluso en pantallas de alta densidad, y mantiene el JPEG resultante
// chico (unos pocos KB).
const ladoMaximoMiniatura = 320

// pixelesMaximosMiniatura es el guardia contra "decompression bombs":
// una imagen que declara dimensiones absurdas en su cabecera (ej. un PNG
// de 1KB que dice ser de 50000x50000 píxeles) reventaría la memoria al
// decodificarse por completo. image.DecodeConfig lee SOLO la cabecera
// (no decodifica los píxeles), así que este chequeo es barato y corre
// ANTES de tocar image.Decode. 50 megapíxeles (ej. ~7000x7000) es más
// que cualquier foto real que alguien suba a Bythos.
const pixelesMaximosMiniatura = 50_000_000

// ErrMiniaturaNoDisponible es lo que ve el handler HTTP cuando no
// corresponde (o no se pudo) generar una miniatura: mime sin soporte
// (webp y todo lo que no sea imagen), decodificación fallida, o imagen
// demasiado grande. El caller responde 404 SIEMPRE con el mismo mensaje
// genérico sin importar la causa puntual — la UI no necesita distinguir
// el motivo, solo cae al ícono de tipo.
var ErrMiniaturaNoDisponible = fmt.Errorf("no se pudo generar una miniatura para este archivo")

// carpetaMiniaturas es Base()/miniaturas: al lado del almacén de blobs,
// nunca mezclado con las subcarpetas <sha[:2]>/ de Guardar.
func carpetaMiniaturas() string {
	return filepath.Join(Base(), "miniaturas")
}

func rutaMiniatura(sha256 string) string {
	return filepath.Join(carpetaMiniaturas(), sha256+".jpg")
}

// mimeSoportaMiniatura es la lista blanca de formatos que Go sabe
// decodificar con su std lib (ver los imports de arriba). Cualquier otra
// cosa (webp, pdf, video, office, texto) nunca llega a intentar abrir el
// archivo.
func mimeSoportaMiniatura(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif":
		return true
	default:
		return false
	}
}

// Miniatura devuelve la ruta absoluta en disco de la miniatura JPEG del
// archivo (sha256/rutaRelativa/mime, los tres datos que ya tiene
// db.Archivo). La genera y cachea la primera vez; llamadas siguientes
// para el mismo sha256 solo hacen un os.Stat. Devuelve
// ErrMiniaturaNoDisponible para todo caso en que no corresponda mostrar
// miniatura — el caller (api/) lo traduce a 404.
func Miniatura(sha256, rutaRelativa, mime string) (string, error) {
	destino := rutaMiniatura(sha256)
	if _, err := os.Stat(destino); err == nil {
		return destino, nil
	}
	if !mimeSoportaMiniatura(mime) {
		return "", ErrMiniaturaNoDisponible
	}

	f, err := Abrir(rutaRelativa)
	if err != nil {
		return "", ErrMiniaturaNoDisponible
	}
	defer f.Close()

	// Paso 1: SOLO la cabecera, para el guardia de decompression bomb.
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return "", ErrMiniaturaNoDisponible
	}
	if int64(cfg.Width)*int64(cfg.Height) > pixelesMaximosMiniatura {
		return "", ErrMiniaturaNoDisponible
	}

	// Paso 2: recién acá decodificamos los píxeles completos, ya sabiendo
	// que el tamaño declarado es razonable.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", ErrMiniaturaNoDisponible
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return "", ErrMiniaturaNoDisponible
	}

	pequena := escalar(img, ladoMaximoMiniatura)

	if err := os.MkdirAll(carpetaMiniaturas(), 0755); err != nil {
		return "", fmt.Errorf("no se pudo preparar la carpeta de miniaturas: %w", err)
	}
	tmp, err := os.CreateTemp(carpetaMiniaturas(), "tmp-miniatura-*")
	if err != nil {
		return "", fmt.Errorf("no se pudo crear el temporal de la miniatura: %w", err)
	}
	tmpPath := tmp.Name()
	huboExito := false
	defer func() {
		if huboExito {
			return
		}
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "archivos: no se pudo limpiar el temporal de miniatura", tmpPath, "-", err)
		}
	}()

	if err := jpeg.Encode(tmp, pequena, &jpeg.Options{Quality: 85}); err != nil {
		tmp.Close()
		return "", fmt.Errorf("no se pudo codificar la miniatura: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("no se pudo cerrar el temporal de la miniatura: %w", err)
	}
	// Mismo patrón de rename atómico que Guardar: tmp nace en la misma
	// carpeta que el destino, así el rename es atómico (mismo volumen) y
	// dos requests concurrentes generando la MISMA miniatura por primera
	// vez no dejan un archivo a medio escribir — la segunda simplemente
	// pisa a la primera con bytes idénticos.
	if err := os.Rename(tmpPath, destino); err != nil {
		return "", fmt.Errorf("no se pudo guardar la miniatura: %w", err)
	}
	huboExito = true
	return destino, nil
}

// EliminarMiniatura borra la miniatura cacheada de un archivo, si existe.
// La llama api/ desde limpiarArchivoSiHuerfano, en el mismo momento en
// que se borra el blob (refcount llegó a cero). Idempotente, mismo
// criterio que Eliminar: si ya no está, no es error.
func EliminarMiniatura(sha256 string) error {
	if sha256 == "" {
		return nil
	}
	err := os.Remove(rutaMiniatura(sha256))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// escalar reduce img a que su lado más largo mida como máximo maxLado,
// con downscale por promedio de área hecho a mano: para cada píxel de
// destino, promedia el bloque rectangular de píxeles de origen que le
// corresponde (mejor calidad que nearest-neighbor para fotos, sin usar
// ninguna librería de resize). Si la imagen ya es igual o más chica que
// maxLado en su lado más largo, se devuelve tal cual — agrandarla no
// suma nitidez, solo trabajo.
func escalar(img image.Image, maxLado int) image.Image {
	b := img.Bounds()
	anchoOrig, altoOrig := b.Dx(), b.Dy()
	if anchoOrig <= 0 || altoOrig <= 0 {
		return img
	}
	ladoMayor := max(anchoOrig, altoOrig)
	if ladoMayor <= maxLado {
		return img
	}

	factor := float64(maxLado) / float64(ladoMayor)
	anchoDst := max(1, int(float64(anchoOrig)*factor))
	altoDst := max(1, int(float64(altoOrig)*factor))

	dst := image.NewRGBA(image.Rect(0, 0, anchoDst, altoDst))
	for y := 0; y < altoDst; y++ {
		y0 := b.Min.Y + y*altoOrig/altoDst
		y1 := b.Min.Y + (y+1)*altoOrig/altoDst
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < anchoDst; x++ {
			x0 := b.Min.X + x*anchoOrig/anchoDst
			x1 := b.Min.X + (x+1)*anchoOrig/anchoDst
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var rSum, gSum, bSum, aSum, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					r, g, bb, a := img.At(xx, yy).RGBA()
					rSum += uint64(r)
					gSum += uint64(g)
					bSum += uint64(bb)
					aSum += uint64(a)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA64(x, y, color.RGBA64{
				R: uint16(rSum / n),
				G: uint16(gSum / n),
				B: uint16(bSum / n),
				A: uint16(aSum / n),
			})
		}
	}
	return dst
}
