package app

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/contas"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/memstore"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// publicador guarda os eventos publicados.
type publicador struct {
	mu      sync.Mutex
	eventos []string
	err     error
}

func (p *publicador) Publish(_ context.Context, tipo string, detail any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	d := detail.(events.UserDeletedDetail)
	p.eventos = append(p.eventos, tipo+" "+d.UserID)
	return nil
}

type cenario struct {
	svc    *Service
	st     *memstore.Store
	contas *contas.Fake
	pub    *publicador
}

var ctx = context.Background()

// montarAdmin cria a admin "adm" e os clientes "ana" e "bia", todos ativos.
func montarAdmin(t *testing.T) cenario {
	t.Helper()
	c := cenario{st: memstore.New(), contas: contas.NewFake(), pub: &publicador{}}
	c.svc = novoService(c.st).ComAdmin(c.contas, c.pub)
	for i, p := range []struct {
		id    string
		papel perfil.Papel
	}{{"adm", perfil.Admin}, {"ana", perfil.Cliente}, {"bia", perfil.Cliente}} {
		novo, err := perfil.Novo(p.id, p.id+"@x.com", strings.ToUpper(p.id), p.papel, time.Date(2026, 11, 1+i, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.st.Criar(ctx, novo); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func (c cenario) perfil(t *testing.T, id string) perfil.Perfil {
	t.Helper()
	p, err := c.st.Obter(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func conflito(t *testing.T, nome string, err error, codigo string) {
	t.Helper()
	if e := errs.As(err); e == nil || e.Kind != errs.KindConflict || e.Code != codigo {
		t.Errorf("%s = %v, quero 409 %s", nome, err, codigo)
	}
}

func TestBloquearEReativar(t *testing.T) {
	// Cenários "Admin bloqueia usuário" e "Admin reativa usuário".
	c := montarAdmin(t)
	p, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Bloqueado})
	if err != nil || p.Status != perfil.Bloqueado || c.perfil(t, "ana").Status != perfil.Bloqueado {
		t.Fatalf("bloquear = %+v, %v", p, err)
	}
	if !c.contas.Bloqueada("ana") {
		t.Error("a conta não foi bloqueada no Cognito")
	}

	p, err = c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Ativo})
	if err != nil || p.Status != perfil.Ativo || c.contas.Bloqueada("ana") {
		t.Fatalf("reativar = %+v, %v", p, err)
	}
	if !slices.Equal(c.contas.Chamadas, []string{"Bloquear ana", "Reativar ana"}) {
		t.Errorf("chamadas = %v", c.contas.Chamadas)
	}
}

func TestPedirOEstadoAtualNaoMudaNada(t *testing.T) {
	c := montarAdmin(t)
	antes := c.perfil(t, "ana")
	p, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Ativo, Papel: perfil.Cliente})
	if err != nil || p != antes || len(c.contas.Chamadas) != 0 {
		t.Errorf("sem mudança = %+v, %v, chamadas %v", p, err, c.contas.Chamadas)
	}
}

func TestBloquearASiMesmo(t *testing.T) {
	c := montarAdmin(t)
	_, err := c.svc.AtualizarUsuario(ctx, "adm", "adm", Mudanca{Status: perfil.Bloqueado})
	conflito(t, "bloquear a si mesmo", err, "bloquear_a_si_mesmo")
	if len(c.contas.Chamadas) != 0 || c.perfil(t, "adm").Status != perfil.Ativo {
		t.Error("o bloqueio recusado mexeu na conta")
	}
}

func TestPromoverERebaixar(t *testing.T) {
	// Cenário "Promover a admin": o grupo muda no Cognito e no perfil.
	c := montarAdmin(t)
	p, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Papel: perfil.Admin})
	if err != nil || p.Papel != perfil.Admin || c.contas.Papel("ana") != perfil.Admin {
		t.Fatalf("promover = %+v, %v", p, err)
	}
	// Com dois admins, o primeiro pode se rebaixar.
	p, err = c.svc.AtualizarUsuario(ctx, "adm", "adm", Mudanca{Papel: perfil.Cliente})
	if err != nil || p.Papel != perfil.Cliente || c.contas.Papel("adm") != perfil.Cliente {
		t.Fatalf("rebaixar = %+v, %v", p, err)
	}
}

func TestUltimoAdmin(t *testing.T) {
	// Cenário "Último admin": o único admin ativo não pode se rebaixar.
	c := montarAdmin(t)
	_, err := c.svc.AtualizarUsuario(ctx, "adm", "adm", Mudanca{Papel: perfil.Cliente})
	conflito(t, "rebaixar o último admin", err, "ultimo_admin")
	if len(c.contas.Chamadas) != 0 || c.perfil(t, "adm").Papel != perfil.Admin {
		t.Error("a troca recusada mexeu na conta")
	}

	// Um segundo admin bloqueado não conta como ativo.
	if _, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Papel: perfil.Admin, Status: perfil.Bloqueado}); err != nil {
		t.Fatal(err)
	}
	_, err = c.svc.AtualizarUsuario(ctx, "adm", "adm", Mudanca{Papel: perfil.Cliente})
	conflito(t, "rebaixar com o outro admin bloqueado", err, "ultimo_admin")

	// Outro admin ativo libera o rebaixamento.
	if _, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Ativo}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.svc.AtualizarUsuario(ctx, "adm", "adm", Mudanca{Papel: perfil.Cliente}); err != nil {
		t.Errorf("rebaixar com outro admin ativo = %v", err)
	}
}

func TestUltimoAdminVisaoDeOutroAdmin(t *testing.T) {
	// O último admin ativo também não pode ser bloqueado nem excluído por
	// alguém cujo perfil já não conta como admin ativo.
	c := montarAdmin(t)
	_, err := c.svc.AtualizarUsuario(ctx, "ana", "adm", Mudanca{Status: perfil.Bloqueado})
	conflito(t, "bloquear o último admin", err, "ultimo_admin")
	conflito(t, "excluir o último admin", c.svc.ExcluirUsuario(ctx, "ana", "adm"), "ultimo_admin")
	if len(c.contas.Chamadas) != 0 || len(c.pub.eventos) != 0 {
		t.Errorf("chamadas %v, eventos %v", c.contas.Chamadas, c.pub.eventos)
	}
}

func TestTransicaoInvalida(t *testing.T) {
	// Cenário "Transição inválida": conta EXCLUIDO não volta a ATIVO, nem
	// muda de papel.
	c := montarAdmin(t)
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); err != nil {
		t.Fatal(err)
	}
	_, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Ativo})
	conflito(t, "reativar excluída", err, "transicao_invalida")
	_, err = c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Papel: perfil.Admin})
	conflito(t, "promover excluída", err, "transicao_invalida")
}

func TestAtualizarInvalido(t *testing.T) {
	c := montarAdmin(t)
	casos := map[string]Mudanca{
		"vazia":            {},
		"status EXCLUIDO":  {Status: perfil.Excluido},
		"status pendente":  {Status: perfil.PendenteConfirmacao},
		"status inventado": {Status: "OUTRO"},
		"papel inventado":  {Papel: "dono"},
	}
	for nome, m := range casos {
		_, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", m)
		if errs.KindOf(err) != errs.KindValidation {
			t.Errorf("%s = %v, quero 422", nome, err)
		}
	}
	for _, id := range []string{"nao-existe", "", "  "} {
		_, err := c.svc.AtualizarUsuario(ctx, "adm", id, Mudanca{Status: perfil.Bloqueado})
		if e := errs.As(err); e == nil || e.Kind != errs.KindNotFound || e.Code != "usuario_nao_encontrado" {
			t.Errorf("id %q = %v", id, err)
		}
	}
	if len(c.contas.Chamadas) != 0 {
		t.Errorf("chamadas = %v", c.contas.Chamadas)
	}
}

func TestExcluirUsuario(t *testing.T) {
	// Cenário "Admin exclui usuário": acesso removido e UserDeleted publicado.
	c := montarAdmin(t)
	if err := c.svc.ExcluirUsuario(ctx, "adm", " ana "); err != nil {
		t.Fatal(err)
	}
	if !c.contas.Excluida("ana") || c.perfil(t, "ana").Status != perfil.Excluido {
		t.Error("a conta não foi excluída")
	}
	if !slices.Equal(c.pub.eventos, []string{"UserDeleted ana"}) {
		t.Errorf("eventos = %v", c.pub.eventos)
	}

	// Repetir republica o evento sem tocar de novo no Cognito.
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); err != nil {
		t.Fatal(err)
	}
	if len(c.pub.eventos) != 2 || slices.Index(c.contas.Chamadas, "Excluir ana") != len(c.contas.Chamadas)-1 {
		t.Errorf("repetição: eventos %v, chamadas %v", c.pub.eventos, c.contas.Chamadas)
	}
}

func TestExcluirASiMesmoEInexistente(t *testing.T) {
	c := montarAdmin(t)
	conflito(t, "excluir a si mesmo", c.svc.ExcluirUsuario(ctx, "adm", "adm"), "excluir_a_si_mesmo")
	err := c.svc.ExcluirUsuario(ctx, "adm", "nao-existe")
	if e := errs.As(err); e == nil || e.Code != "usuario_nao_encontrado" {
		t.Errorf("inexistente = %v", err)
	}
}

func TestExcluirComFalhaNaPublicacao(t *testing.T) {
	// A conta já saiu do Cognito e o perfil já é EXCLUIDO; a próxima
	// tentativa só republica.
	c := montarAdmin(t)
	c.pub.err = errors.New("eventbridge fora")
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); errs.KindOf(err) != errs.KindInternal {
		t.Fatalf("erro = %v", err)
	}
	if c.perfil(t, "ana").Status != perfil.Excluido {
		t.Error("perfil não ficou EXCLUIDO")
	}
	c.pub.err = nil
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); err != nil || len(c.pub.eventos) != 1 {
		t.Errorf("retentativa = %v, eventos %v", err, c.pub.eventos)
	}
}

func TestFalhaNoCognitoNaoMexeNoPerfil(t *testing.T) {
	c := montarAdmin(t)
	falha := errs.Internal(errors.New("cognito fora"))
	c.contas.Err = falha
	if _, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Bloqueado}); !errors.Is(err, falha) {
		t.Errorf("bloquear = %v", err)
	}
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); !errors.Is(err, falha) {
		t.Errorf("excluir = %v", err)
	}
	if p := c.perfil(t, "ana"); p.Status != perfil.Ativo || len(c.pub.eventos) != 0 {
		t.Errorf("perfil = %+v, eventos %v", p, c.pub.eventos)
	}
}

func TestErroDoStoreNoAdmin(t *testing.T) {
	c := montarAdmin(t)
	falha := errs.Internal(errors.New("dynamodb fora"))
	c.st.Err = falha
	if _, err := c.svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Bloqueado}); !errors.Is(err, falha) {
		t.Errorf("atualizar = %v", err)
	}
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); !errors.Is(err, falha) {
		t.Errorf("excluir = %v", err)
	}
	if _, err := c.svc.ListarUsuarios(ctx, "", 0); !errors.Is(err, falha) {
		t.Errorf("listar = %v", err)
	}
}

func TestAdminDesligado(t *testing.T) {
	svc := novoService(memstore.New())
	if svc.AdminHabilitado() {
		t.Fatal("admin ligado sem ComAdmin")
	}
	if _, err := svc.AtualizarUsuario(ctx, "adm", "ana", Mudanca{Status: perfil.Bloqueado}); errs.KindOf(err) != errs.KindInternal {
		t.Errorf("atualizar = %v", err)
	}
	if err := svc.ExcluirUsuario(ctx, "adm", "ana"); errs.KindOf(err) != errs.KindInternal {
		t.Errorf("excluir = %v", err)
	}
}

func TestListarUsuariosPaginado(t *testing.T) {
	// Cenário "Admin lista usuários": do mais novo para o mais antigo.
	c := montarAdmin(t)
	pg, err := c.svc.ListarUsuarios(ctx, "", 2)
	if err != nil || len(pg.Items) != 2 || pg.Items[0].ID != "bia" || pg.Items[1].ID != "ana" || pg.NextCursor == "" {
		t.Fatalf("página 1 = %+v, %v", pg, err)
	}
	pg, err = c.svc.ListarUsuarios(ctx, pg.NextCursor, 2)
	if err != nil || len(pg.Items) != 1 || pg.Items[0].ID != "adm" || pg.NextCursor != "" {
		t.Fatalf("página 2 = %+v, %v", pg, err)
	}
}

func TestCursorInvalido(t *testing.T) {
	c := montarAdmin(t)
	bom, _ := c.svc.ListarUsuarios(ctx, "", 1)
	casos := map[string]string{
		"lixo":          "!!!",
		"não é JSON":    codificarTexto("oi"),
		"chave a mais":  codificarTexto(`{"PK":"USER#a","SK":"PROFILE","GSI1PK":"USERS","GSI1SK":"2026-11-01T00:00:00.000Z#a","X":"y"}`),
		"outro índice":  codificarTexto(`{"PK":"USER#a","SK":"PROFILE","GSI1PK":"OUTRO","GSI1SK":"2026-11-01T00:00:00.000Z#a"}`),
		"outro item":    codificarTexto(`{"PK":"USER#a","SK":"CAT#x","GSI1PK":"USERS","GSI1SK":"2026-11-01T00:00:00.000Z#a"}`),
		"id divergente": codificarTexto(`{"PK":"USER#a","SK":"PROFILE","GSI1PK":"USERS","GSI1SK":"2026-11-01T00:00:00.000Z#b"}`),
		"data errada":   codificarTexto(`{"PK":"USER#a","SK":"PROFILE","GSI1PK":"USERS","GSI1SK":"ontem#a"}`),
		"longo":         strings.Repeat("a", maxCursor+1),
	}
	for nome, cur := range casos {
		_, err := c.svc.ListarUsuarios(ctx, cur, 1)
		if e := errs.As(err); e == nil || e.Kind != errs.KindBadRequest || e.Code != "cursor_invalido" {
			t.Errorf("%s = %v", nome, err)
		}
	}
	if _, err := c.svc.ListarUsuarios(ctx, bom.NextCursor, 1); err != nil {
		t.Errorf("cursor bom = %v", err)
	}
}

func codificarTexto(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
