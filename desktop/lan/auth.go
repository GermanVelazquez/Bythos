package lan

// auth.go — Los tokens por dispositivo: 32 bytes de crypto/rand, solo el
// SHA-256 hex vive en la base (db.dispositivos), y el middleware Bearer que
// protege toda ruta autenticada. Un token revocado falla igual que uno
// desconocido (db.DispositivoPorTokenHash excluye revocados).

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"bythos-desktop/db"
)

// GenerarToken devuelve el token en claro (se entrega UNA vez al celular)
// y su hash, lo único que se persiste.
func GenerarToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken es el SHA-256 hex del token. Buscar por hash en la base (alta
// entropía, UNIQUE) evita comparar el secreto en claro byte a byte.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ConToken envuelve un handler autenticado: exige "Authorization: Bearer
// <token>" de un dispositivo no revocado, o responde 401 token_invalido
// sin llamar a siguiente.
func ConToken(base *sql.DB, siguiente func(http.ResponseWriter, *http.Request, db.Dispositivo)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			responderError(w, ErrTokenInvalido)
			return
		}
		d, existe, err := db.DispositivoPorTokenHash(base, HashToken(token))
		if err != nil || !existe {
			responderError(w, ErrTokenInvalido)
			return
		}
		_ = db.TocarDispositivo(base, d.ID) // mejor esfuerzo: no tumba la request
		siguiente(w, r, d)
	})
}
