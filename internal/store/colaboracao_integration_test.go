//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
)

// Colaboração no DynamoDB Local (colaboradores, tarefa 2.2).

func codigo(err error) string {
	if e := errs.As(err); e != nil {
		return e.Code
	}
	return ""
}

func convite(dono, email string) colaboracao.Convite {
	return colaboracao.Convite{DonoID: dono, NomeDono: "Dono " + dono, Email: email, Papel: colaboracao.Editor, CriadoEm: base}
}

func membro(dono, user string) colaboracao.Membro {
	return colaboracao.Membro{DonoID: dono, NomeDono: "Dono " + dono, UserID: user, Nome: "Nome " + user, Email: user + "@x.com", Papel: colaboracao.Editor, Desde: base}
}

func TestConviteEAceite(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)

	c := convite("ana", "bruno@x.com")
	if err := d.CriarConvite(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := d.CriarConvite(ctx, c); codigo(err) != contract.CodigoConviteExistente {
		t.Errorf("repetido = %v", err)
	}
	if err := d.CriarConvite(ctx, convite("carla", "bruno@x.com")); err != nil {
		t.Fatal(err)
	}
	recebidos, err := d.ConvitesDoEmail(ctx, "bruno@x.com")
	if err != nil || len(recebidos) != 2 || recebidos[0].DonoID != "ana" || recebidos[0].NomeDono != "Dono ana" {
		t.Fatalf("recebidos = %+v, %v", recebidos, err)
	}
	if lido, err := d.ObterConvite(ctx, "ana", "bruno@x.com"); err != nil || lido != c {
		t.Errorf("obter convite = %+v, %v", lido, err)
	}

	m := membro("ana", "bruno")
	if err := d.AceitarConvite(ctx, c, m); err != nil {
		t.Fatal(err)
	}
	// O convite sumiu na mesma transação; aceitar de novo é 404.
	if err := d.AceitarConvite(ctx, c, m); codigo(err) != contract.CodigoConviteNaoEncontrado {
		t.Errorf("aceitar de novo = %v", err)
	}
	if lido, err := d.ObterMembro(ctx, "ana", "bruno"); err != nil || lido != m {
		t.Errorf("membro = %+v, %v", lido, err)
	}
	contas, err := d.ContasDoMembro(ctx, "bruno")
	if err != nil || len(contas) != 1 || contas[0].DonoID != "ana" {
		t.Errorf("contas = %+v, %v", contas, err)
	}
	membros, convites, err := d.DaConta(ctx, "ana")
	if err != nil || len(membros) != 1 || len(convites) != 0 {
		t.Errorf("da conta = %+v %+v, %v", membros, convites, err)
	}
	// O perfil (USER#) não aparece entre os colaboradores.
	if _, _, err := d.Criar(ctx, novo(t, "ana", base)); err != nil {
		t.Fatal(err)
	}
	if membros, _, _ := d.DaConta(ctx, "ana"); len(membros) != 1 {
		t.Errorf("membros com o perfil = %+v", membros)
	}
}

func TestMudarERemover(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	m := membro("ana", "bruno")
	if err := d.SalvarMembro(ctx, m); codigo(err) != contract.CodigoColaboradorNaoEncontrado {
		t.Errorf("salvar inexistente = %v", err)
	}
	c := convite("ana", "bruno@x.com")
	if err := d.CriarConvite(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := d.AceitarConvite(ctx, c, m); err != nil {
		t.Fatal(err)
	}
	m.Papel = colaboracao.Leitor
	if err := d.SalvarMembro(ctx, m); err != nil {
		t.Fatal(err)
	}
	if lido, _ := d.ObterMembro(ctx, "ana", "bruno"); lido.Papel != colaboracao.Leitor {
		t.Errorf("papel = %s", lido.Papel)
	}
	if err := d.ExcluirMembro(ctx, "ana", "bruno"); err != nil {
		t.Fatal(err)
	}
	if err := d.ExcluirMembro(ctx, "ana", "bruno"); codigo(err) != contract.CodigoColaboradorNaoEncontrado {
		t.Errorf("excluir de novo = %v", err)
	}
	if err := d.ExcluirConvite(ctx, "ana", "x@x.com"); codigo(err) != contract.CodigoConviteNaoEncontrado {
		t.Errorf("excluir convite inexistente = %v", err)
	}
	if _, err := d.ObterConvite(ctx, "ana", "x@x.com"); codigo(err) != contract.CodigoConviteNaoEncontrado {
		t.Errorf("obter convite inexistente = %v", err)
	}
}

func TestExcluirColaboracoesDe(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	// Ana é dona (Bruno membro, convite para Carla) e colabora na conta da Dani.
	for _, par := range []struct {
		c colaboracao.Convite
		m colaboracao.Membro
	}{{convite("ana", "bruno@x.com"), membro("ana", "bruno")}, {convite("dani", "ana@x.com"), membro("dani", "ana")}} {
		if err := d.CriarConvite(ctx, par.c); err != nil {
			t.Fatal(err)
		}
		if err := d.AceitarConvite(ctx, par.c, par.m); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.CriarConvite(ctx, convite("ana", "carla@x.com")); err != nil {
		t.Fatal(err)
	}
	n, err := d.ExcluirColaboracoesDe(ctx, "ana")
	if err != nil || n != 3 {
		t.Fatalf("excluir = %d, %v", n, err)
	}
	if c, _ := d.ContasDoMembro(ctx, "bruno"); len(c) != 0 {
		t.Errorf("bruno ainda membro: %+v", c)
	}
	if c, _ := d.ConvitesDoEmail(ctx, "carla@x.com"); len(c) != 0 {
		t.Errorf("convite da carla: %+v", c)
	}
	if m, _, _ := d.DaConta(ctx, "dani"); len(m) != 0 {
		t.Errorf("ana ainda na conta da dani: %+v", m)
	}
	if n, _ := d.ExcluirColaboracoesDe(ctx, "ana"); n != 0 {
		t.Errorf("de novo = %d", n)
	}
}
