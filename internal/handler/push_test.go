package handler

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/memstore"
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
