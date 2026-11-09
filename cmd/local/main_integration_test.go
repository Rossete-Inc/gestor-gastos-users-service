//go:build integration

package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/dynamotest"
)

func portaLivre(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	_, porta, _ := net.SplitHostPort(l.Addr().String())
	return porta
}

// TestPontaAPonta sobe o cmd/local de verdade, contra uma tabela temporária
// no DynamoDB Local, e o usa como o BFF e o publisher de eventos usarão.
func TestPontaAPonta(t *testing.T) {
	_, tabela := dynamotest.UsersTable(t, "gestor-gastos-users")
	porta := portaLivre(t)
	base := "http://127.0.0.1:" + porta

	ctx, cancel := context.WithCancel(context.Background())
	terminou := make(chan error, 1)
	go func() {
		terminou <- run(ctx, env(map[string]string{
			"RUNTIME": "local", "PORT": porta, "LOG_LEVEL": "error",
			"DYNAMODB_ENDPOINT": dynamotest.Endpoint(), "USERS_TABLE": tabela,
		}))
	}()

	// Espera o servidor responder.
	pronto := false
	for range 50 {
		if resp, err := http.Get(base + "/healthz"); err == nil {
			_ = resp.Body.Close()
			pronto = resp.StatusCode == http.StatusOK
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !pronto {
		cancel()
		t.Fatalf("cmd/local não subiu: %v", <-terminou)
	}

	bff := invoke.NewClient(invoke.NewHTTP(base, nil))

	t.Run("perfil inexistente", func(t *testing.T) {
		err := bff.Call(ctx, contract.OpObterPerfil, "u-1", nil, nil)
		if e := errs.As(err); e == nil || e.Code != "perfil_nao_encontrado" {
			t.Errorf("erro = %v", err)
		}
	})

	t.Run("CriarPerfil e ObterPerfil", func(t *testing.T) {
		var criado, obtido contract.Perfil
		in := contract.CriarPerfilIn{Email: "admin@local.test", Nome: "Admin Local", Papel: contract.PapelAdmin}
		if err := bff.Call(ctx, contract.OpCriarPerfil, "u-1", in, &criado); err != nil {
			t.Fatal(err)
		}
		if err := bff.Call(ctx, contract.OpObterPerfil, "u-1", nil, &obtido); err != nil {
			t.Fatal(err)
		}
		if obtido != criado || obtido.Papel != contract.PapelAdmin {
			t.Errorf("obtido = %+v, criado = %+v", obtido, criado)
		}
	})

	t.Run("UserConfirmed pelo publisher local", func(t *testing.T) {
		pub := events.NewLocal([]string{base + events.Path}, "gestor-gastos.teste", nil, nil)
		d := events.UserConfirmedDetail{UserID: "u-2", Email: "cliente@local.test", Nome: "Cliente Local"}
		if err := pub.Publish(ctx, events.UserConfirmed, d); err != nil {
			t.Fatal(err)
		}
		var p contract.Perfil
		if err := bff.Call(ctx, contract.OpObterPerfil, "u-2", nil, &p); err != nil {
			t.Fatal(err)
		}
		if p.Papel != contract.PapelCliente || p.Status != contract.StatusAtivo {
			t.Errorf("perfil = %+v", p)
		}
	})

	t.Run("validação chega ao BFF", func(t *testing.T) {
		err := bff.Call(ctx, contract.OpCriarPerfil, "u-3", contract.CriarPerfilIn{Email: "x", Nome: "Ana", Papel: "cliente"}, nil)
		if e := errs.As(err); e == nil || e.Kind != errs.KindValidation || e.Fields[0].Field != "email" {
			t.Errorf("erro = %v", err)
		}
	})

	cancel()
	select {
	case err := <-terminou:
		if err != nil {
			t.Errorf("encerramento: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("cmd/local não encerrou")
	}
}
