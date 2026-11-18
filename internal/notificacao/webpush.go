package notificacao

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
	webpush "github.com/SherClockHolmes/webpush-go"
)

// ttl é quanto tempo o serviço de push guarda a notificação de um aparelho
// desligado: um dia, depois disso o alerta já perdeu o sentido.
const ttl = 24 * 60 * 60

// Enviador entrega uma mensagem a um aparelho.
type Enviador interface {
	// Enviar devolve expirada=true quando o serviço de push diz que a
	// assinatura não vale mais (404 ou 410): quem chama a apaga.
	Enviar(ctx context.Context, a push.Assinatura, m Mensagem) (expirada bool, err error)
}

// WebPush envia pelo protocolo Web Push, assinado com VAPID (RFC 8292).
type WebPush struct {
	ChavePublica string
	ChavePrivada string
	// Assunto é o "sub" do JWT do VAPID: uma URL https de contato do
	// serviço, nunca o e-mail de alguém.
	Assunto string
	// HTTP nil usa o client padrão da biblioteca.
	HTTP webpush.HTTPClient
}

// Enviar implementa Enviador.
func (w WebPush) Enviar(ctx context.Context, a push.Assinatura, m Mensagem) (bool, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return false, fmt.Errorf("serializar mensagem: %w", err)
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload,
		&webpush.Subscription{Endpoint: a.Endpoint, Keys: webpush.Keys{P256dh: a.P256dh, Auth: a.Auth}},
		&webpush.Options{
			HTTPClient: w.HTTP, Subscriber: w.Assunto, TTL: ttl, Urgency: webpush.UrgencyNormal,
			VAPIDPublicKey: w.ChavePublica, VAPIDPrivateKey: w.ChavePrivada,
		})
	if err != nil {
		return false, fmt.Errorf("enviar push: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return true, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return false, nil
	default:
		return false, fmt.Errorf("serviço de push respondeu %d", resp.StatusCode)
	}
}
