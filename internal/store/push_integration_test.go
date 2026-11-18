//go:build integration

package store

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
)

func assinatura(t *testing.T, userID, endpoint string) push.Assinatura {
	t.Helper()
	a, err := push.Nova(userID, endpoint,
		base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...)),
		base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
		time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAssinaturasPushNoDynamo(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	if _, _, err := d.Criar(ctx, novo(t, "ana", base)); err != nil {
		t.Fatal(err)
	}

	celular := assinatura(t, "ana", "https://fcm.googleapis.com/fcm/send/celular")
	notebook := assinatura(t, "ana", "https://updates.push.services.mozilla.com/wpush/v2/notebook")
	for _, a := range []push.Assinatura{celular, notebook, celular, assinatura(t, "bruno", celular.Endpoint)} {
		if err := d.SalvarAssinatura(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	lista, err := d.AssinaturasDe(ctx, "ana")
	if err != nil || len(lista) != 2 {
		t.Fatalf("aparelhos da ana = %+v, %v", lista, err)
	}
	for _, a := range lista {
		if a.UserID != "ana" || a.P256dh != celular.P256dh || !a.CriadoEm.Equal(celular.CriadoEm) {
			t.Fatalf("assinatura lida = %+v", a)
		}
	}

	// as assinaturas não entram na listagem do admin nem atrapalham o perfil
	perfis, _, err := d.Listar(ctx, 10, nil)
	if err != nil || len(perfis) != 1 {
		t.Fatalf("listagem do admin = %+v, %v", perfis, err)
	}
	if p, err := d.Obter(ctx, "ana"); err != nil || p.ID != "ana" {
		t.Fatalf("perfil = %+v, %v", p, err)
	}

	if err := d.ExcluirAssinatura(ctx, "ana", celular.ID()); err != nil {
		t.Fatal(err)
	}
	if err := d.ExcluirAssinatura(ctx, "ana", celular.ID()); err != nil {
		t.Fatalf("excluir de novo = %v", err)
	}
	if n, err := d.ExcluirAssinaturasDe(ctx, "ana"); err != nil || n != 1 {
		t.Fatalf("excluir todas = %d, %v", n, err)
	}
	if lista, _ := d.AssinaturasDe(ctx, "ana"); len(lista) != 0 {
		t.Fatalf("restaram = %+v", lista)
	}
	if lista, _ := d.AssinaturasDe(ctx, "bruno"); len(lista) != 1 {
		t.Fatalf("bruno = %+v", lista)
	}
}
