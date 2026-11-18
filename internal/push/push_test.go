package push

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
)

var (
	agora  = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	p256dh = base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	auth   = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
)

const endpoint = "https://fcm.googleapis.com/fcm/send/abc123"

func TestNova(t *testing.T) {
	a, err := Nova("u-1", " "+endpoint+" ", p256dh, auth+"==", agora)
	if err != nil {
		t.Fatal(err)
	}
	if a.Endpoint != endpoint || a.UserID != "u-1" || !a.CriadoEm.Equal(agora) {
		t.Fatalf("assinatura = %+v", a)
	}
	if a.ID() != ID(endpoint) || len(a.ID()) != 64 {
		t.Fatalf("ID = %q", a.ID())
	}
	if ID(endpoint+"x") == a.ID() {
		t.Fatal("endpoints diferentes com o mesmo ID")
	}
}

func TestNovaInvalida(t *testing.T) {
	casos := []struct {
		nome, endpoint, p256dh, auth, campo string
	}{
		{"http sem s", "http://push.example/abc", p256dh, auth, "endpoint"},
		{"endpoint relativo", "/push/abc", p256dh, auth, "endpoint"},
		{"endpoint com usuário", "https://x:y@push.example/abc", p256dh, auth, "endpoint"},
		{"endpoint enorme", "https://push.example/" + strings.Repeat("a", MaxEndpoint), p256dh, auth, "endpoint"},
		{"p256dh vazio", endpoint, "", auth, "p256dh"},
		{"p256dh curto", endpoint, auth, auth, "p256dh"},
		{"auth não base64", endpoint, p256dh, "***", "auth"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Nova("u-1", c.endpoint, c.p256dh, c.auth, agora)
			e := errs.As(err)
			if e == nil || e.Kind != errs.KindValidation || len(e.Fields) != 1 || e.Fields[0].Field != c.campo {
				t.Fatalf("erro = %v, want campo %s", err, c.campo)
			}
		})
	}
}
