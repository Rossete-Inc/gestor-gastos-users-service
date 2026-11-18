package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/memstore"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/notificacao"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
)

func TestPushPeloContrato(t *testing.T) {
	ctx := context.Background()
	st := memstore.New()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	c := invoke.NewClient(transporte{Invoke(app.New(st, log), log)})
	in := contract.RegistrarAssinaturaPushIn{
		Endpoint: "https://fcm.googleapis.com/fcm/send/celular",
		P256dh:   base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...)),
		Auth:     base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
	}
	if err := c.Call(ctx, contract.OpRegistrarAssinaturaPush, "ana", in, nil); err != nil {
		t.Fatal(err)
	}
	if lista, _ := st.AssinaturasDe(ctx, "ana"); len(lista) != 1 || lista[0].Endpoint != in.Endpoint {
		t.Fatalf("assinaturas = %+v", lista)
	}
	if err := c.Call(ctx, contract.OpRemoverAssinaturaPush, "ana", contract.RemoverAssinaturaPushIn{Endpoint: in.Endpoint}, nil); err != nil {
		t.Fatal(err)
	}
	if lista, _ := st.AssinaturasDe(ctx, "ana"); len(lista) != 0 {
		t.Fatalf("depois de remover = %+v", lista)
	}

	in.Auth = "curta"
	err := c.Call(ctx, contract.OpRegistrarAssinaturaPush, "ana", in, nil)
	if e := errs.As(err); e == nil || e.Kind != errs.KindValidation || e.Fields[0].Field != "auth" {
		t.Fatalf("auth inválida = %v", err)
	}
	// sem userId no envelope
	if err := c.Call(ctx, contract.OpRemoverAssinaturaPush, "", contract.RemoverAssinaturaPushIn{Endpoint: in.Endpoint}, nil); err == nil {
		t.Fatal("sem usuário deveria falhar")
	}
}

type enviadorDeTeste struct{ mensagens []notificacao.Mensagem }

func (e *enviadorDeTeste) Enviar(_ context.Context, _ push.Assinatura, m notificacao.Mensagem) (bool, error) {
	e.mensagens = append(e.mensagens, m)
	return false, nil
}

func TestEventoMetasAtingidas(t *testing.T) {
	ctx := context.Background()
	st := memstore.New()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	env := &enviadorDeTeste{}
	svc := app.New(st, log).ComPush(env)
	c := invoke.NewClient(transporte{Invoke(svc, log)})
	in := contract.RegistrarAssinaturaPushIn{
		Endpoint: "https://fcm.googleapis.com/fcm/send/celular",
		P256dh:   base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...)),
		Auth:     base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
	}
	if err := c.Call(ctx, contract.OpRegistrarAssinaturaPush, "ana", in, nil); err != nil {
		t.Fatal(err)
	}

	detail, _ := json.Marshal(events.MetasAtingidasDetail{DonoID: "ana", AutorID: "ana", Alertas: []events.AlertaDeMeta{
		{CategoriaID: "lazer", NomeCategoria: "Lazer", Mes: "2099-01", Limiar: 100, TotalCentavos: 110000, MetaCentavos: 100000},
	}})
	if err := Events(svc, log).Dispatch(ctx, events.Event{DetailType: events.MetasAtingidas, Detail: detail}); err != nil {
		t.Fatal(err)
	}
	if len(env.mensagens) != 1 || !strings.Contains(env.mensagens[0].Corpo, "Meta de Lazer estourou em janeiro de 2099 (R$ 1.100,00 de R$ 1.000,00).") {
		t.Fatalf("mensagens = %+v", env.mensagens)
	}
}
