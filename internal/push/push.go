// Package push é a assinatura Web Push de um aparelho (spec aplicativo-pwa,
// "Ativar notificações no aparelho"; ADR 0011). O navegador gera o endpoint
// e as chaves ao assinar; o users-service guarda uma assinatura por
// aparelho e pessoa e as usa para enviar os alertas de meta.
package push

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
)

const (
	// MaxEndpoint limita o tamanho do endpoint (os navegadores usam bem
	// menos de 1 KB).
	MaxEndpoint = 2048
	// Tamanhos das chaves decodificadas: p256dh é um ponto P-256 não
	// comprimido (65 bytes) e auth é um segredo de 16 bytes (RFC 8291).
	tamanhoP256dh = 65
	tamanhoAuth   = 16
)

// Assinatura de um aparelho.
type Assinatura struct {
	UserID   string
	Endpoint string
	P256dh   string // base64url, como o navegador entrega
	Auth     string // base64url
	CriadoEm time.Time
}

// ID identifica a assinatura pelo endpoint: registrar o mesmo aparelho de
// novo substitui a assinatura em vez de duplicar.
func ID(endpoint string) string {
	h := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(h[:])
}

// ID da assinatura.
func (a Assinatura) ID() string { return ID(a.Endpoint) }

// Nova valida os dados do navegador e cria a assinatura do usuário.
func Nova(userID, endpoint, p256dh, auth string, agora time.Time) (Assinatura, error) {
	endpoint, p256dh, auth = strings.TrimSpace(endpoint), strings.TrimSpace(p256dh), strings.TrimSpace(auth)
	var campos []errs.FieldError
	if err := ValidarEndpoint(endpoint); err != nil {
		campos = append(campos, errs.FieldError{Field: "endpoint", Message: "deve ser uma URL https do serviço de push"})
	}
	if !chaveValida(p256dh, tamanhoP256dh) {
		campos = append(campos, errs.FieldError{Field: "p256dh", Message: "chave inválida"})
	}
	if !chaveValida(auth, tamanhoAuth) {
		campos = append(campos, errs.FieldError{Field: "auth", Message: "chave inválida"})
	}
	if len(campos) > 0 {
		return Assinatura{}, errs.Validation("Assinatura de notificações inválida.", campos...)
	}
	return Assinatura{UserID: userID, Endpoint: endpoint, P256dh: p256dh, Auth: auth, CriadoEm: agora.UTC().Truncate(time.Millisecond)}, nil
}

// ValidarEndpoint aceita só URL https absoluta, de até MaxEndpoint
// caracteres: é para onde o users-service vai mandar requisições.
func ValidarEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > MaxEndpoint || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errs.Validation("Endpoint inválido.", errs.FieldError{Field: "endpoint", Message: "deve ser uma URL https do serviço de push"})
	}
	return nil
}

// chaveValida confere que a chave é base64url (com ou sem padding) do
// tamanho esperado.
func chaveValida(chave string, tamanho int) bool {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(chave, "="))
	return err == nil && len(b) == tamanho
}
