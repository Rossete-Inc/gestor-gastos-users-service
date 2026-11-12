package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/contas"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/memstore"
)

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
	svc := app.New(memstore.New(), log)
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

// semEventos aceita qualquer publicação.
type semEventos struct{ n int }

func (s *semEventos) Publish(context.Context, string, any) error { s.n++; return nil }

func montarAdmin(t *testing.T) (*invoke.Client, *memstore.Store, *contas.Fake, *semEventos) {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	st, fake, pub := memstore.New(), contas.NewFake(), &semEventos{}
	svc := app.New(st, log).ComAdmin(fake, pub)
	c := invoke.NewClient(transporte{Invoke(svc, log)})
	ctx := context.Background()
	for _, u := range []struct{ id, papel string }{{"adm", "admin"}, {"ana", "cliente"}} {
		in := contract.CriarPerfilIn{Email: u.id + "@x.com", Nome: u.id, Papel: u.papel}
		if err := c.Call(ctx, contract.OpCriarPerfil, u.id, in, &contract.Perfil{}); err != nil {
			t.Fatal(err)
		}
	}
	return c, st, fake, pub
}

func TestAdminPeloContrato(t *testing.T) {
	ctx := context.Background()
	c, _, fake, pub := montarAdmin(t)

	var pg contract.PaginaUsuarios
	if err := c.Call(ctx, contract.OpListarUsuarios, "adm", contract.ListarUsuariosIn{Limit: 10}, &pg); err != nil || len(pg.Items) != 2 {
		t.Fatalf("listar = %+v, %v", pg, err)
	}

	var p contract.Perfil
	in := contract.AtualizarUsuarioIn{ID: "ana", Status: "BLOQUEADO", Papel: "admin"}
	if err := c.Call(ctx, contract.OpAtualizarUsuario, "adm", in, &p); err != nil {
		t.Fatal(err)
	}
	if p.Status != "BLOQUEADO" || p.Papel != "admin" || !fake.Bloqueada("ana") {
		t.Errorf("atualizado = %+v", p)
	}

	err := c.Call(ctx, contract.OpAtualizarUsuario, "adm", contract.AtualizarUsuarioIn{ID: "adm", Status: "BLOQUEADO"}, &p)
	if e := errs.As(err); e == nil || e.Kind != errs.KindConflict || e.Code != contract.CodigoBloquearASiMesmo {
		t.Errorf("bloquear a si mesmo = %v", err)
	}
	err = c.Call(ctx, contract.OpAtualizarUsuario, "adm", contract.AtualizarUsuarioIn{ID: "ana", Status: "EXCLUIDO"}, &p)
	if e := errs.As(err); e == nil || e.Kind != errs.KindValidation {
		t.Errorf("status inválido = %v", err)
	}

	if err := c.Call(ctx, contract.OpExcluirUsuario, "adm", contract.ExcluirUsuarioIn{ID: "ana"}, nil); err != nil {
		t.Fatal(err)
	}
	if !fake.Excluida("ana") || pub.n != 1 {
		t.Errorf("excluir: conta excluída %v, eventos %d", fake.Excluida("ana"), pub.n)
	}
	err = c.Call(ctx, contract.OpExcluirUsuario, "adm", contract.ExcluirUsuarioIn{ID: "nao-existe"}, nil)
	if e := errs.As(err); e == nil || e.Code != contract.CodigoUsuarioNaoEncontrado {
		t.Errorf("excluir inexistente = %v", err)
	}
	for _, op := range []string{contract.OpListarUsuarios, contract.OpAtualizarUsuario, contract.OpExcluirUsuario} {
		err := c.Call(ctx, op, "", map[string]string{"id": "ana"}, nil)
		if e := errs.As(err); e == nil || e.Code != "usuario_ausente" {
			t.Errorf("%s sem usuário = %v", op, err)
		}
	}
}

func TestListarUsuariosFormatoJSON(t *testing.T) {
	// Lista vazia: items sai como [] e nextCursor some (openapi: TelaAdminUsuarios).
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := Invoke(app.New(memstore.New(), log), log)
	resp := r.Dispatch(context.Background(), invoke.Request{Operation: contract.OpListarUsuarios, UserID: "adm", Payload: json.RawMessage(`{}`)})
	if string(resp.Payload) != `{"items":[]}` {
		t.Errorf("payload = %s", resp.Payload)
	}
	resp = r.Dispatch(context.Background(), invoke.Request{Operation: contract.OpListarUsuarios, UserID: "adm", Payload: json.RawMessage(`{"cursor":"!!"}`)})
	if p := errs.FromProblem(*resp.Error); p.Code != contract.CodigoCursorInvalido {
		t.Errorf("cursor inválido = %+v", resp.Error)
	}
}

func TestAdminDesligadoNaoExpoeAsOperacoes(t *testing.T) {
	// Lambda antes da 10.4: só a listagem; o resto é operação desconhecida.
	c, _ := montar()
	for _, op := range []string{contract.OpAtualizarUsuario, contract.OpExcluirUsuario} {
		err := c.Call(context.Background(), op, "adm", contract.ExcluirUsuarioIn{ID: "ana"}, nil)
		if e := errs.As(err); e == nil || e.Code != "operacao_desconhecida" {
			t.Errorf("%s = %v", op, err)
		}
	}
	if err := c.Call(context.Background(), contract.OpListarUsuarios, "adm", contract.ListarUsuariosIn{}, &contract.PaginaUsuarios{}); err != nil {
		t.Errorf("listar = %v", err)
	}
}
