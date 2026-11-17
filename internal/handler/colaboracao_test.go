package handler

import (
	"context"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
)

// O ciclo da colaboração pelo contrato: convite, aceite, acesso, nomes,
// papel, saída e remoção.
func TestColaboracaoPeloContrato(t *testing.T) {
	ctx := context.Background()
	c, _ := montar()
	for _, p := range [][3]string{{"ana", "ana@x.com", "Ana"}, {"bruno", "bruno@x.com", "Bruno"}} {
		in := contract.CriarPerfilIn{Email: p[1], Nome: p[2], Papel: contract.PapelCliente}
		if err := c.Call(ctx, contract.OpCriarPerfil, p[0], in, &contract.Perfil{}); err != nil {
			t.Fatal(err)
		}
	}

	var conv contract.Convite
	if err := c.Call(ctx, contract.OpConvidar, "ana", contract.ConvidarIn{Email: "Bruno@x.com", Papel: contract.PapelEditor}, &conv); err != nil {
		t.Fatal(err)
	}
	if conv.Email != "bruno@x.com" || conv.NomeDono != "Ana" || conv.Papel != "EDITOR" {
		t.Errorf("convite = %+v", conv)
	}
	var recebidos contract.ConvitesRecebidos
	if err := c.Call(ctx, contract.OpListarConvitesRecebidos, "bruno", contract.ListarConvitesRecebidosIn{Email: "bruno@x.com"}, &recebidos); err != nil || len(recebidos.Items) != 1 {
		t.Fatalf("recebidos = %+v, %v", recebidos, err)
	}

	var acesso contract.Acesso
	err := c.Call(ctx, contract.OpObterAcesso, "bruno", contract.ObterAcessoIn{DonoID: "ana"}, &acesso)
	if e := errs.As(err); e == nil || e.Code != contract.CodigoContaNaoEncontrada {
		t.Errorf("acesso antes do aceite = %v", err)
	}

	var conta contract.Conta
	if err := c.Call(ctx, contract.OpAceitarConvite, "bruno", contract.ResponderConviteIn{DonoID: "ana", Email: "bruno@x.com"}, &conta); err != nil {
		t.Fatal(err)
	}
	if conta != (contract.Conta{DonoID: "ana", NomeDono: "Ana", Papel: "EDITOR"}) {
		t.Errorf("conta = %+v", conta)
	}
	if err := c.Call(ctx, contract.OpObterAcesso, "bruno", contract.ObterAcessoIn{DonoID: "ana"}, &acesso); err != nil || acesso.Papel != "EDITOR" {
		t.Errorf("acesso = %+v, %v", acesso, err)
	}
	var minhas contract.MinhasContas
	if err := c.Call(ctx, contract.OpListarMinhasContas, "bruno", nil, &minhas); err != nil || len(minhas.Items) != 1 {
		t.Errorf("minhas contas = %+v, %v", minhas, err)
	}
	var lista contract.Colaboradores
	if err := c.Call(ctx, contract.OpListarColaboradores, "ana", nil, &lista); err != nil || len(lista.Membros) != 1 || lista.Convites == nil {
		t.Errorf("colaboradores = %+v, %v", lista, err)
	}
	var nomes contract.Nomes
	if err := c.Call(ctx, contract.OpNomesDe, "ana", contract.NomesDeIn{IDs: []string{"ana", "bruno"}}, &nomes); err != nil || nomes.Nomes["bruno"] != "Bruno" {
		t.Errorf("nomes = %+v, %v", nomes, err)
	}

	var col contract.Colaborador
	if err := c.Call(ctx, contract.OpMudarPapel, "ana", contract.MudarPapelIn{UserID: "bruno", Papel: contract.PapelLeitor}, &col); err != nil || col.Papel != "LEITOR" {
		t.Errorf("mudar papel = %+v, %v", col, err)
	}
	if err := c.Call(ctx, contract.OpSairDaConta, "bruno", contract.SairDaContaIn{DonoID: "ana"}, nil); err != nil {
		t.Errorf("sair = %v", err)
	}
	if err := c.Call(ctx, contract.OpRemoverColaborador, "ana", contract.RemoverColaboradorIn{UserID: "bruno"}, nil); errs.KindOf(err) != errs.KindNotFound {
		t.Errorf("remover quem saiu = %v", err)
	}

	// Convite recusado e cancelado.
	if err := c.Call(ctx, contract.OpConvidar, "ana", contract.ConvidarIn{Email: "carla@x.com", Papel: contract.PapelLeitor}, &conv); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, contract.OpRecusarConvite, "carla", contract.ResponderConviteIn{DonoID: "ana", Email: "carla@x.com"}, nil); err != nil {
		t.Errorf("recusar = %v", err)
	}
	if err := c.Call(ctx, contract.OpConvidar, "ana", contract.ConvidarIn{Email: "dani@x.com", Papel: contract.PapelLeitor}, &conv); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, contract.OpCancelarConvite, "ana", contract.CancelarConviteIn{Email: "dani@x.com"}, nil); err != nil {
		t.Errorf("cancelar = %v", err)
	}

	// Sem usuário no envelope: 400.
	if err := c.Call(ctx, contract.OpListarMinhasContas, "", nil, &minhas); errs.KindOf(err) != errs.KindBadRequest {
		t.Errorf("sem usuário = %v", err)
	}
}
