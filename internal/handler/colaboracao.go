package handler

import (
	"context"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
)

// operacao registra uma operação que exige o userId do envelope.
func operacao[In, Out any](r *invoke.Router, nome string, f func(ctx context.Context, userID string, in In) (Out, error)) {
	r.Handle(nome, invoke.Typed(func(ctx context.Context, userID string, in In) (Out, error) {
		if err := exigirUsuario(userID); err != nil {
			var zero Out
			return zero, err
		}
		return f(ctx, userID, in)
	}))
}

func membros(l []colaboracao.Membro) []contract.Colaborador {
	out := make([]contract.Colaborador, 0, len(l))
	for _, m := range l {
		out = append(out, m.Contrato())
	}
	return out
}

func convites(l []colaboracao.Convite) []contract.Convite {
	out := make([]contract.Convite, 0, len(l))
	for _, c := range l {
		out = append(out, c.Contrato())
	}
	return out
}

// colaboracaoNoRouter registra as operações de colaboração (ADR 0010).
func colaboracaoNoRouter(r *invoke.Router, svc *app.Service) {
	operacao(r, contract.OpObterAcesso, func(ctx context.Context, userID string, in contract.ObterAcessoIn) (contract.Acesso, error) {
		papel, err := svc.ObterAcesso(ctx, userID, in.DonoID)
		return contract.Acesso{DonoID: in.DonoID, Papel: papel}, err
	})
	operacao(r, contract.OpListarColaboradores, func(ctx context.Context, userID string, _ struct{}) (contract.Colaboradores, error) {
		m, c, err := svc.ListarColaboradores(ctx, userID)
		return contract.Colaboradores{Membros: membros(m), Convites: convites(c)}, err
	})
	operacao(r, contract.OpConvidar, func(ctx context.Context, userID string, in contract.ConvidarIn) (contract.Convite, error) {
		c, err := svc.Convidar(ctx, userID, in.Email, colaboracao.Papel(in.Papel))
		return c.Contrato(), err
	})
	operacao(r, contract.OpCancelarConvite, func(ctx context.Context, userID string, in contract.CancelarConviteIn) (any, error) {
		return nil, svc.CancelarConvite(ctx, userID, in.Email)
	})
	operacao(r, contract.OpListarConvitesRecebidos, func(ctx context.Context, _ string, in contract.ListarConvitesRecebidosIn) (contract.ConvitesRecebidos, error) {
		l, err := svc.ConvitesRecebidos(ctx, in.Email)
		return contract.ConvitesRecebidos{Items: convites(l)}, err
	})
	operacao(r, contract.OpAceitarConvite, func(ctx context.Context, userID string, in contract.ResponderConviteIn) (contract.Conta, error) {
		m, err := svc.AceitarConvite(ctx, userID, in.Email, in.DonoID)
		return m.Conta(), err
	})
	operacao(r, contract.OpRecusarConvite, func(ctx context.Context, _ string, in contract.ResponderConviteIn) (any, error) {
		return nil, svc.RecusarConvite(ctx, in.Email, in.DonoID)
	})
	operacao(r, contract.OpMudarPapel, func(ctx context.Context, userID string, in contract.MudarPapelIn) (contract.Colaborador, error) {
		m, err := svc.MudarPapel(ctx, userID, in.UserID, colaboracao.Papel(in.Papel))
		return m.Contrato(), err
	})
	operacao(r, contract.OpRemoverColaborador, func(ctx context.Context, userID string, in contract.RemoverColaboradorIn) (any, error) {
		return nil, svc.RemoverColaborador(ctx, userID, in.UserID)
	})
	operacao(r, contract.OpSairDaConta, func(ctx context.Context, userID string, in contract.SairDaContaIn) (any, error) {
		return nil, svc.SairDaConta(ctx, userID, in.DonoID)
	})
	operacao(r, contract.OpListarMinhasContas, func(ctx context.Context, userID string, _ struct{}) (contract.MinhasContas, error) {
		l, err := svc.MinhasContas(ctx, userID)
		out := contract.MinhasContas{Items: make([]contract.Conta, 0, len(l))}
		for _, m := range l {
			out.Items = append(out.Items, m.Conta())
		}
		return out, err
	})
	operacao(r, contract.OpNomesDe, func(ctx context.Context, _ string, in contract.NomesDeIn) (contract.Nomes, error) {
		n, err := svc.NomesDe(ctx, in.IDs)
		return contract.Nomes{Nomes: n}, err
	})
}

// pushNoRouter registra as operações de assinatura push (ADR 0011).
func pushNoRouter(r *invoke.Router, svc *app.Service) {
	operacao(r, contract.OpRegistrarAssinaturaPush, func(ctx context.Context, userID string, in contract.RegistrarAssinaturaPushIn) (any, error) {
		return nil, svc.RegistrarAssinaturaPush(ctx, userID, in.Endpoint, in.P256dh, in.Auth)
	})
	operacao(r, contract.OpRemoverAssinaturaPush, func(ctx context.Context, userID string, in contract.RemoverAssinaturaPushIn) (any, error) {
		return nil, svc.RemoverAssinaturaPush(ctx, userID, in.Endpoint)
	})
}
