package lan

// cert_test.go — clavePersistente da la misma clave entre "arranques"
// (mismo fingerprint SPKI, distinto SAN), y persiste en disco.

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintEstableEntreCertificados(t *testing.T) {
	carpeta := t.TempDir() // nunca %APPDATA% real
	cert1, err := certificadoParaIPs(carpeta, []net.IP{net.ParseIP("192.168.1.10")})
	if err != nil {
		t.Fatalf("certificadoParaIPs #1: %v", err)
	}
	huella1, err := FingerprintSPKI(cert1)
	if err != nil || huella1 == "" {
		t.Fatalf("FingerprintSPKI #1: huella=%q err=%v", huella1, err)
	}

	cert2, err := certificadoParaIPs(carpeta, []net.IP{net.ParseIP("192.168.2.20")})
	if err != nil {
		t.Fatalf("certificadoParaIPs #2: %v", err)
	}
	huella2, err := FingerprintSPKI(cert2)
	if err != nil {
		t.Fatalf("FingerprintSPKI #2: %v", err)
	}
	if huella1 != huella2 {
		t.Fatalf("fingerprint cambió entre arranques con la misma clave: %s != %s", huella1, huella2)
	}
	if _, err := os.Stat(filepath.Join(carpeta, nombreArchivoClave)); err != nil {
		t.Fatalf("clave.pem no quedó persistida: %v", err)
	}
}
