package terminal

import (
	"bytes"
	"testing"
)

func TestAnilloDebajoDeLaCapacidadDevuelveTodo(t *testing.T) {
	a := nuevoAnillo(16)
	a.Escribir([]byte("hola "))
	a.Escribir([]byte("mundo"))
	if got := string(a.Snapshot()); got != "hola mundo" {
		t.Fatalf("snapshot = %q", got)
	}
}

func TestAnilloSobreescribeLoMasViejo(t *testing.T) {
	a := nuevoAnillo(8)
	a.Escribir([]byte("abcdef"))
	a.Escribir([]byte("ghij")) // desborda y da la vuelta
	if got := string(a.Snapshot()); got != "cdefghij" {
		t.Fatalf("snapshot = %q", got)
	}
	a.Escribir([]byte("0123456789XY")) // más grande que el anillo
	if got := string(a.Snapshot()); got != "456789XY" {
		t.Fatalf("snapshot = %q", got)
	}
}

func TestAnilloReplayEsLosUltimos256KiB(t *testing.T) {
	a := nuevoAnillo(tamAnillo)
	var todo []byte
	for i := 0; i < 700; i++ {
		trozo := bytes.Repeat([]byte{byte(i)}, 1000)
		todo = append(todo, trozo...)
		a.Escribir(trozo)
	}
	if got := a.Snapshot(); !bytes.Equal(got, todo[len(todo)-tamAnillo:]) {
		t.Fatalf("el replay no es la cola de %d bytes (len=%d)", tamAnillo, len(got))
	}
}

func TestAnilloSnapshotEsCopia(t *testing.T) {
	a := nuevoAnillo(4)
	a.Escribir([]byte("abcd"))
	s := a.Snapshot()
	a.Escribir([]byte("wxyz"))
	if string(s) != "abcd" {
		t.Fatalf("el snapshot cambió: %q", s)
	}
}
