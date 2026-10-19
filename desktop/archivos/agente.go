package archivos

// agente.go — Lo que el agente recibe de un adjunto de la pizarra: una
// imagen acotada (leer_imagen) o el texto aplanado con tope (leer_archivo).
// Corre en el servidor para funcionar con la UI cerrada (design D13, D14).

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strconv"
	"strings"
)

// ladoMaximoAgente: lado más largo de la imagen que se le entrega al agente.
const ladoMaximoAgente = 1280

// maxBytesSinProcesar: una imagen que no se puede reescalar (webp, ilegible,
// demasiados píxeles) solo se entrega tal cual si pesa hasta esto.
const maxBytesSinProcesar = 5 << 20

// maxBytesLeidos: techo de lectura en memoria (el límite de un adjunto).
const maxBytesLeidos = 25 << 20

// maxRunasAgente: tope del texto que lee el agente (el visor admite más).
const maxRunasAgente = 40_000

// ImagenAgente es la imagen lista para el agente. Datos nil = no se pudo
// entregar la imagen; Aviso explica por qué y el caller devuelve texto y ruta.
type ImagenAgente struct {
	Datos []byte
	Mime  string
	Aviso string
}

// ImagenParaAgente lee el blob y lo deja en 1280px como máximo. El guardia de
// píxeles y escalar son los de miniaturas.go. PNG y GIF salen como PNG (no
// pierden transparencia), JPEG como JPEG.
func ImagenParaAgente(rutaRelativa, mime string) (ImagenAgente, error) {
	f, err := Abrir(rutaRelativa)
	if err != nil {
		return ImagenAgente{}, err
	}
	defer f.Close()
	datos, err := io.ReadAll(io.LimitReader(f, maxBytesLeidos+1))
	if err != nil {
		return ImagenAgente{}, err
	}
	tal := func() (ImagenAgente, error) {
		if len(datos) > maxBytesSinProcesar {
			return ImagenAgente{Mime: mime, Aviso: "La imagen no se puede reducir y supera los 5 MiB: léela desde la ruta."}, nil
		}
		return ImagenAgente{Datos: datos, Mime: mime}, nil
	}
	if !mimeSoportaMiniatura(mime) || len(datos) > maxBytesLeidos {
		return tal()
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || int64(cfg.Width)*int64(cfg.Height) > pixelesMaximosMiniatura {
		return tal()
	}
	if max(cfg.Width, cfg.Height) <= ladoMaximoAgente {
		return tal()
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return tal()
	}
	var salida bytes.Buffer
	pequena := escalar(img, ladoMaximoAgente)
	if mime == "image/jpeg" {
		err = jpeg.Encode(&salida, pequena, &jpeg.Options{Quality: 85})
	} else {
		mime = "image/png"
		err = png.Encode(&salida, pequena)
	}
	if err != nil {
		return tal()
	}
	return ImagenAgente{Datos: salida.Bytes(), Mime: mime}, nil
}

// TextoPlano es el texto aplanado: Texto ya recortado, CaracteresTotales el
// largo completo (en runas) antes del recorte.
type TextoPlano struct {
	Texto             string
	Truncado          bool
	CaracteresTotales int
}

// Plano aplana el texto extraído para el agente: títulos con #, lista con "- ",
// "Diapositiva N" por lámina y filas de hoja separadas por tabulador. Tope de
// 40.000 runas; Truncado también conserva el corte del propio extractor.
func (t TextoExtraido) Plano() TextoPlano {
	var lineas []string
	for _, b := range t.Bloques {
		switch b.Tipo {
		case "titulo":
			lineas = append(lineas, strings.Repeat("#", max(1, b.Nivel))+" "+b.Texto)
		case "lista":
			lineas = append(lineas, "- "+b.Texto)
		default:
			lineas = append(lineas, b.Texto)
		}
	}
	for _, d := range t.Diapositivas {
		lineas = append(lineas, "Diapositiva "+strconv.Itoa(d.Numero))
		lineas = append(lineas, d.Parrafos...)
	}
	for _, fila := range t.Filas {
		lineas = append(lineas, strings.Join(fila, "\t"))
	}
	if t.Texto != "" {
		lineas = append(lineas, t.Texto)
	}
	runas := []rune(strings.Join(lineas, "\n"))
	p := TextoPlano{Texto: string(runas), Truncado: t.Truncado, CaracteresTotales: len(runas)}
	if len(runas) > maxRunasAgente {
		p.Texto, p.Truncado = string(runas[:maxRunasAgente]), true
	}
	return p
}
