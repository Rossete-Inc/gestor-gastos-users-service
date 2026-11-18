package notificacao

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
	webpush "github.com/SherClockHolmes/webpush-go"
)

// aparelho gera chaves de verdade, como o navegador faria.
func aparelho(t *testing.T, endpoint string) push.Assinatura {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	segredo := make([]byte, 16)
	_, _ = rand.Read(segredo)
	return push.Assinatura{
		UserID: "ana", Endpoint: endpoint,
		P256dh: base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		Auth:   base64.RawURLEncoding.EncodeToString(segredo),
	}
}

func TestWebPush(t *testing.T) {
	privada, publica, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	var pedidos []*http.Request
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos = append(pedidos, r)
		switch r.URL.Path {
		case "/expirada":
			w.WriteHeader(http.StatusGone)
		case "/sumiu":
			w.WriteHeader(http.StatusNotFound)
		case "/quebrado":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	w := WebPush{ChavePublica: publica, ChavePrivada: privada, Assunto: "https://github.com/Rossete-Inc", HTTP: srv.Client()}
	m := Montar([]Alerta{{NomeCategoria: "Lazer", Mes: "2026-11", Limiar: 80, TotalCentavos: 82000, MetaCentavos: 100000}}, "2026-11")

	expirada, err := w.Enviar(context.Background(), aparelho(t, srv.URL+"/ok"), m)
	if err != nil || expirada {
		t.Fatalf("envio = %v, %v", expirada, err)
	}
	r := pedidos[0]
	if r.Method != http.MethodPost || r.Header.Get("Content-Encoding") != "aes128gcm" || r.Header.Get("TTL") != "86400" {
		t.Fatalf("cabeçalhos = %v", r.Header)
	}
	if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, "k="+publica) {
		t.Fatalf("Authorization = %q", auth)
	}

	// spec aplicativo-pwa, "Aparelho que não aceita mais notificações"
	for _, rota := range []string{"/expirada", "/sumiu"} {
		if expirada, err := w.Enviar(context.Background(), aparelho(t, srv.URL+rota), m); err != nil || !expirada {
			t.Errorf("%s = %v, %v", rota, expirada, err)
		}
	}
	if _, err := w.Enviar(context.Background(), aparelho(t, srv.URL+"/quebrado"), m); err == nil {
		t.Error("500 deveria ser erro")
	}
}
