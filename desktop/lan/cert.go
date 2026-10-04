package lan

// cert.go — Identidad TLS del receptor: clave ECDSA P-256 persistida en
// disco (mismo par entre arranques) y certificado autofirmado reemitido
// en cada Start con las IPs actuales como SAN. El celular no valida
// contra una CA: fija el SPKI (FingerprintSPKI) que viaja en el QR, así
// el fingerprint queda estable aunque el certificado cambie de IP.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const nombreArchivoClave = "clave.pem"

// CarpetaLAN es %APPDATA%\Bythos\lan (o ~/.config/bythos/lan).
func CarpetaLAN() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "lan")
	}
	return filepath.Join(base, "Bythos", "lan")
}

// clavePersistente carga la clave de carpeta, o genera y guarda una nueva
// si no existe o está corrupta. Reusar la misma clave entre arranques es
// lo que mantiene el fingerprint SPKI estable: una clave nueva en cada
// Start invalidaría a todos los celulares ya emparejados.
func clavePersistente(carpeta string) (*ecdsa.PrivateKey, error) {
	ruta := filepath.Join(carpeta, nombreArchivoClave)
	if datos, err := os.ReadFile(ruta); err == nil {
		if bloque, _ := pem.Decode(datos); bloque != nil {
			if clave, err := x509.ParseECPrivateKey(bloque.Bytes); err == nil {
				return clave, nil
			}
		}
	}
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	crudo, err := x509.MarshalECPrivateKey(clave)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(carpeta, 0700); err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: crudo})
	return clave, os.WriteFile(ruta, pemBytes, 0600)
}

// certificadoParaIPs genera (sin persistir) un cert autofirmado válido 10
// años con ips como SAN, firmado con la clave persistente de carpeta.
// Se llama en cada Start: barato de generar, refleja la IP del arranque.
func certificadoParaIPs(carpeta string, ips []net.IP) (tls.Certificate, error) {
	clave, err := clavePersistente(carpeta)
	if err != nil {
		return tls.Certificate{}, err
	}
	serie, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	plantilla := &x509.Certificate{
		SerialNumber: serie,
		Subject:      pkix.Name{CommonName: "bythos-lan"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		return tls.Certificate{}, err
	}
	clavePKCS8, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	clavePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clavePKCS8})
	return tls.X509KeyPair(certPEM, clavePEM)
}

// FingerprintSPKI es el SHA-256 en base64url (43 chars) de la
// SubjectPublicKeyInfo: lo que va en el QR (?k=...) y lo que el celular
// fija como pin. Depende solo de la clave, no cambia si el cert se
// reemite con otro SAN o serie.
func FingerprintSPKI(cert tls.Certificate) (string, error) {
	hoja := cert.Leaf
	if hoja == nil {
		var err error
		hoja, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return "", err
		}
	}
	suma := sha256.Sum256(hoja.RawSubjectPublicKeyInfo)
	return base64.RawURLEncoding.EncodeToString(suma[:]), nil
}
