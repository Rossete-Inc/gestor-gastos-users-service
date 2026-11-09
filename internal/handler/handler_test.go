package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

type memStore struct {
	mu     sync.Mutex
	perfis map[string]perfil.Perfil
}

func (m *memStore) Criar(_ context.Context, p perfil.Perfil) (perfil.Perfil, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atual, ok := m.perfis[p.ID]; ok {
		return atual, false, nil
	}
	m.perfis[p.ID] = p
	return p, true, nil
}

func (m *memStore) Obter(_ context.Context, id string) (perfil.Perfil, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.perfis[id]; ok {
		return p, nil
	}
	return perfil.Perfil{}, errs.NotFound("perfil_nao_encontrado", "Perfil não encontrado.")
}

func (m *memStore) Listar(context.Context, int, page.Key) ([]perfil.Perfil, page.Key, error) {
	return nil, nil, nil
}

// transporte entrega o envelope direto ao router, passando por JSON como os
// transportes reais.
type transporte struct{ r *invoke.Router }

func (t transporte) Send(ctx context.Context, req invoke.Request) (invoke.Response, error) {
	b, _ := json.Marshal(req)
	var dec invoke.Request
	_ = json.Unmarshal(b, &dec)
	return t.r.Dispatch(ctx, dec), nil
}

func montar() (*invoke.Client, *events.Router) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := app.New(&memStore{perfis: map[string]perfil.Perfil{}}, log)
	return invoke.NewClient(transporte{Invoke(svc, log)}), Events(svc, log)
}

func TestObterPerfilInexistente(t *testing.T) {
	c, _ := montar()
	err := c.Call(context.Background(), contract.OpObterPerfil, "u-1", nil, &contract.Perfil{})
	if e := errs.As(err); e.Kind != errs.KindNotFound || e.Code != "perfil_nao_encontrado" {
		t.Errorf("erro = %+v", e)
	}
}

func TestCriarEObterPeloContrato(t *testing.T) {
	ctx := context.Background()
	c, _ := montar()

	var criado contract.Perfil
	in := contract.CriarPerfilIn{Email: "Ana@X.com", Nome: "Ana", Papel: contract.PapelCliente}
	if err := c.Call(ctx, contract.OpCriarPerfil, "u-1", in, &criado); err != nil {
		t.Fatal(err)
	}
	if criado.ID != "u-1" || criado.Email != "ana@x.com" || criado.Papel != "cliente" || criado.Status != "ATIVO" || criado.CriadoEm.IsZero() {
		t.Errorf("criado = %+v", criado)
	}

	var obtido contract.Perfil
	if err := c.Call(ctx, contract.OpObterPerfil, "u-1", nil, &obtido); err != nil {
		t.Fatal(err)
	}
	if obtido != criado {
		t.Errorf("obtido = %+v, quer %+v", obtido, criado)
	}
}

func TestCriarPerfilInvalido(t *testing.T) {
	c, _ := montar()
	err := c.Call(context.Background(), contract.OpCriarPerfil, "u-1", contract.CriarPerfilIn{Email: "x", Nome: "Ana", Papel: "root"}, nil)
	e := errs.As(err)
	if e.Kind != errs.KindValidation || len(e.Fields) != 2 {
		t.Errorf("erro = %+v", e)
	}
}

func TestSemUsuario(t *testing.T) {
	c, _ := montar()
	for _, op := range []string{contract.OpObterPerfil, contract.OpCriarPerfil} {
		err := c.Call(context.Background(), op, "", contract.CriarPerfilIn{}, nil)
		if e := errs.As(err); e.Code != "usuario_ausente" {
			t.Errorf("%s: erro = %+v", op, e)
		}
	}
}

func TestEventoUserConfirmed(t *testing.T) {
	ctx := context.Background()
	c, ev := montar()

	detail, _ := json.Marshal(events.UserConfirmedDetail{UserID: "u-7", Email: "bia@x.com", Nome: "Bia"})
	e := events.Event{ID: "e-1", DetailType: events.UserConfirmed, Source: "gestor-gastos.auth", Time: time.Now(), Detail: detail}
	if err := ev.LambdaHandler()(ctx, e); err != nil {
		t.Fatal(err)
	}
	var p contract.Perfil
	if err := c.Call(ctx, contract.OpObterPerfil, "u-7", nil, &p); err != nil {
		t.Fatal(err)
	}
	if p.Papel != contract.PapelCliente || p.Nome != "Bia" {
		t.Errorf("perfil = %+v", p)
	}
}

func TestEventoInvalido(t *testing.T) {
	_, ev := montar()
	// Detail sem e-mail: falha de validação vira erro, e na AWS o evento vai
	// para a DLQ depois das retentativas.
	detail, _ := json.Marshal(events.UserConfirmedDetail{UserID: "u-8", Nome: "Sem Email"})
	err := ev.Dispatch(context.Background(), events.Event{DetailType: events.UserConfirmed, Detail: detail})
	if errs.KindOf(err) != errs.KindValidation {
		t.Errorf("erro = %v", err)
	}
}
