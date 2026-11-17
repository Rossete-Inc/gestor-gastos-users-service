package app

import (
	"context"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/memstore"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// Spec colaboracao, no users-service.

func comPessoas(t *testing.T) (*Service, *memstore.Store) {
	t.Helper()
	st := newMem()
	s := novoService(st)
	for _, p := range [][3]string{{"ana", "ana@exemplo.com", "Ana"}, {"bruno", "bruno@exemplo.com", "Bruno"}, {"carla", "carla@exemplo.com", "Carla"}} {
		if _, err := s.CriarPerfil(context.Background(), p[0], p[1], p[2], perfil.Cliente); err != nil {
			t.Fatal(err)
		}
	}
	return s, st
}

func codigoDe(err error) string {
	if e := errs.As(err); e != nil {
		return e.Code
	}
	return ""
}

func TestConvidarEAceitar(t *testing.T) {
	ctx := context.Background()
	s, _ := comPessoas(t)

	// Cenário "Convite válido".
	c, err := s.Convidar(ctx, "ana", " Bruno@Exemplo.com ", colaboracao.Editor)
	if err != nil || c.Email != "bruno@exemplo.com" || c.NomeDono != "Ana" {
		t.Fatalf("convite = %+v, %v", c, err)
	}
	recebidos, _ := s.ConvitesRecebidos(ctx, "BRUNO@exemplo.com")
	if len(recebidos) != 1 || recebidos[0].DonoID != "ana" {
		t.Fatalf("recebidos = %+v", recebidos)
	}
	// Antes de aceitar, não há acesso (cenário "Conta alheia sem convite").
	if _, err := s.ObterAcesso(ctx, "bruno", "ana"); codigoDe(err) != contract.CodigoContaNaoEncontrada {
		t.Errorf("acesso antes do aceite = %v", err)
	}

	// Cenário "Convite de outra pessoa": Carla não aceita o convite do Bruno.
	if _, err := s.AceitarConvite(ctx, "carla", "carla@exemplo.com", "ana"); codigoDe(err) != contract.CodigoConviteNaoEncontrado {
		t.Errorf("aceite alheio = %v", err)
	}

	// Cenário "Aceitar".
	m, err := s.AceitarConvite(ctx, "bruno", "bruno@exemplo.com", "ana")
	if err != nil || m.Papel != colaboracao.Editor || m.Nome != "Bruno" || m.NomeDono != "Ana" {
		t.Fatalf("aceite = %+v, %v", m, err)
	}
	if papel, err := s.ObterAcesso(ctx, "bruno", "ana"); err != nil || papel != contract.PapelEditor {
		t.Errorf("acesso = %q, %v", papel, err)
	}
	if papel, _ := s.ObterAcesso(ctx, "ana", "ana"); papel != contract.PapelDono {
		t.Errorf("dono = %q", papel)
	}
	if contas, _ := s.MinhasContas(ctx, "bruno"); len(contas) != 1 || contas[0].DonoID != "ana" {
		t.Errorf("minhas contas = %+v", contas)
	}
	if r, _ := s.ConvitesRecebidos(ctx, "bruno@exemplo.com"); len(r) != 0 {
		t.Errorf("o convite aceito sumiu? %+v", r)
	}
	membros, convites, _ := s.ListarColaboradores(ctx, "ana")
	if len(membros) != 1 || len(convites) != 0 {
		t.Errorf("colaboradores = %+v %+v", membros, convites)
	}

	// Convidar quem já colabora.
	if _, err := s.Convidar(ctx, "ana", "bruno@exemplo.com", colaboracao.Leitor); codigoDe(err) != contract.CodigoJaColaborador {
		t.Errorf("já colabora = %v", err)
	}
}

func TestConvitesRecusadosEInvalidos(t *testing.T) {
	ctx := context.Background()
	s, _ := comPessoas(t)

	// Cenário "Convidar a si mesmo".
	if _, err := s.Convidar(ctx, "ana", "ana@exemplo.com", colaboracao.Editor); errs.KindOf(err) != errs.KindValidation {
		t.Errorf("a si mesmo = %v", err)
	}
	// Cenário "Convite repetido".
	if _, err := s.Convidar(ctx, "ana", "carla@exemplo.com", colaboracao.Leitor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Convidar(ctx, "ana", "Carla@exemplo.com", colaboracao.Leitor); codigoDe(err) != contract.CodigoConviteExistente {
		t.Errorf("repetido = %v", err)
	}
	// Cenário "Recusar".
	if err := s.RecusarConvite(ctx, "carla@exemplo.com", "ana"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecusarConvite(ctx, "carla@exemplo.com", "ana"); codigoDe(err) != contract.CodigoConviteNaoEncontrado {
		t.Errorf("recusar de novo = %v", err)
	}
	// Cancelar pelo dono.
	if _, err := s.Convidar(ctx, "ana", "dani@exemplo.com", colaboracao.Leitor); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelarConvite(ctx, "ana", " DANI@exemplo.com"); err != nil {
		t.Errorf("cancelar = %v", err)
	}
	// Cenário "Convidado sem conta": o convite fica até ele se cadastrar.
	if _, err := s.Convidar(ctx, "ana", "novo@exemplo.com", colaboracao.Editor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CriarPerfil(ctx, "novo", "novo@exemplo.com", "Novo", perfil.Cliente); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.ConvitesRecebidos(ctx, "novo@exemplo.com"); len(r) != 1 {
		t.Errorf("convite do recém-cadastrado = %+v", r)
	}
	if _, err := s.ConvitesRecebidos(ctx, ""); errs.KindOf(err) != errs.KindValidation {
		t.Errorf("sem e-mail = %v", err)
	}
	if _, err := s.AceitarConvite(ctx, "ana", "ana@exemplo.com", "ana"); codigoDe(err) != contract.CodigoConviteNaoEncontrado {
		t.Errorf("aceitar a própria = %v", err)
	}
}

func TestSextoConvite(t *testing.T) {
	ctx := context.Background()
	s, _ := comPessoas(t)
	for _, e := range []string{"1@x.com", "2@x.com", "3@x.com", "4@x.com", "5@x.com"} {
		if _, err := s.Convidar(ctx, "ana", e, colaboracao.Leitor); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Convidar(ctx, "ana", "6@x.com", colaboracao.Leitor); codigoDe(err) != contract.CodigoLimiteColaboradores {
		t.Errorf("sexto = %v", err)
	}
}

func aceitarBruno(t *testing.T, s *Service, papel colaboracao.Papel) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Convidar(ctx, "ana", "bruno@exemplo.com", papel); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AceitarConvite(ctx, "bruno", "bruno@exemplo.com", "ana"); err != nil {
		t.Fatal(err)
	}
}

func TestGestaoPeloDono(t *testing.T) {
	ctx := context.Background()
	s, _ := comPessoas(t)
	aceitarBruno(t, s, colaboracao.Editor)

	// Cenário "Mudar para leitor".
	m, err := s.MudarPapel(ctx, "ana", "bruno", colaboracao.Leitor)
	if err != nil || m.Papel != colaboracao.Leitor {
		t.Fatalf("mudar = %+v, %v", m, err)
	}
	if papel, _ := s.ObterAcesso(ctx, "bruno", "ana"); papel != contract.PapelLeitor {
		t.Errorf("papel = %q", papel)
	}
	if _, err := s.MudarPapel(ctx, "ana", "bruno", "DONO"); errs.KindOf(err) != errs.KindValidation {
		t.Errorf("papel inválido = %v", err)
	}
	if _, err := s.MudarPapel(ctx, "ana", "carla", colaboracao.Editor); codigoDe(err) != contract.CodigoColaboradorNaoEncontrado {
		t.Errorf("não membro = %v", err)
	}
	// Cenário "Remover colaborador".
	if err := s.RemoverColaborador(ctx, "ana", "bruno"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObterAcesso(ctx, "bruno", "ana"); codigoDe(err) != contract.CodigoContaNaoEncontrada {
		t.Errorf("acesso depois de removido = %v", err)
	}
	if err := s.RemoverColaborador(ctx, "ana", "bruno"); codigoDe(err) != contract.CodigoColaboradorNaoEncontrado {
		t.Errorf("remover de novo = %v", err)
	}
}

func TestColaboradorSai(t *testing.T) {
	ctx := context.Background()
	s, _ := comPessoas(t)
	aceitarBruno(t, s, colaboracao.Leitor)
	if err := s.SairDaConta(ctx, "bruno", "ana"); err != nil {
		t.Fatal(err)
	}
	if err := s.SairDaConta(ctx, "bruno", "ana"); codigoDe(err) != contract.CodigoContaNaoEncontrada {
		t.Errorf("sair de novo = %v", err)
	}
	if err := s.SairDaConta(ctx, "bruno", " "); errs.KindOf(err) != errs.KindValidation {
		t.Errorf("sem dono = %v", err)
	}
}

func TestNomesDe(t *testing.T) {
	ctx := context.Background()
	s, _ := comPessoas(t)
	nomes, err := s.NomesDe(ctx, []string{"ana", "bruno", "ana", "fantasma", ""})
	if err != nil || len(nomes) != 2 || nomes["ana"] != "Ana" || nomes["bruno"] != "Bruno" {
		t.Errorf("nomes = %v, %v", nomes, err)
	}
	if _, err := s.NomesDe(ctx, make([]string, 101)); errs.KindOf(err) != errs.KindValidation {
		t.Errorf("101 ids = %v", err)
	}
}

func TestExclusaoEncerraColaboracoes(t *testing.T) {
	ctx := context.Background()
	c := montarAdmin(t)
	s := c.svc
	// montarAdmin já criou "ana"; falta o colaborador.
	if _, err := s.CriarPerfil(ctx, "bruno", "bruno@exemplo.com", "Bruno", perfil.Cliente); err != nil {
		t.Fatal(err)
	}
	aceitarBruno(t, s, colaboracao.Editor)
	if _, err := s.Convidar(ctx, "ana", "carla@exemplo.com", colaboracao.Leitor); err != nil {
		t.Fatal(err)
	}
	// Cenário "Dono com colaboradores é excluído".
	if err := s.ExcluirUsuario(ctx, "adm", "ana"); err != nil {
		t.Fatal(err)
	}
	if contas, _ := s.MinhasContas(ctx, "bruno"); len(contas) != 0 {
		t.Errorf("bruno ainda colabora: %+v", contas)
	}
	if r, _ := s.ConvitesRecebidos(ctx, "carla@exemplo.com"); len(r) != 0 {
		t.Errorf("convite da conta excluída: %+v", r)
	}
}
