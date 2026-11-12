// Package handler liga os casos de uso aos transportes da go-commons: o
// invoke.Router (chamadas do BFF) e o events.Router (eventos). Os mesmos
// routers servem a Lambda e o cmd/local.
package handler

import (
	"context"
	"log/slog"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// Invoke monta o router das operações do contrato.
func Invoke(svc *app.Service, log *slog.Logger) *invoke.Router {
	r := invoke.NewRouter(log)

	r.Handle(contract.OpObterPerfil, invoke.Typed(func(ctx context.Context, userID string, _ struct{}) (contract.Perfil, error) {
		if err := exigirUsuario(userID); err != nil {
			return contract.Perfil{}, err
		}
		p, err := svc.ObterPerfil(ctx, userID)
		return p.Contract(), err
	}))

	r.Handle(contract.OpCriarPerfil, invoke.Typed(func(ctx context.Context, userID string, in contract.CriarPerfilIn) (contract.Perfil, error) {
		if err := exigirUsuario(userID); err != nil {
			return contract.Perfil{}, err
		}
		p, err := svc.CriarPerfil(ctx, userID, in.Email, in.Nome, perfil.Papel(in.Papel))
		return p.Contract(), err
	}))

	r.Handle(contract.OpListarUsuarios, invoke.Typed(func(ctx context.Context, userID string, in contract.ListarUsuariosIn) (contract.PaginaUsuarios, error) {
		if err := exigirUsuario(userID); err != nil {
			return contract.PaginaUsuarios{}, err
		}
		pg, err := svc.ListarUsuarios(ctx, in.Cursor, in.Limit)
		if err != nil {
			return contract.PaginaUsuarios{}, err
		}
		itens := make([]contract.Perfil, 0, len(pg.Items))
		for _, p := range pg.Items {
			itens = append(itens, p.Contract())
		}
		return page.New(itens, pg.NextCursor), nil
	}))

	// Sem Cognito e barramento (Lambda antes da tarefa 10.4), estas operações
	// não existem: o router responde 400 "operacao_desconhecida".
	if svc.AdminHabilitado() {
		r.Handle(contract.OpAtualizarUsuario, invoke.Typed(func(ctx context.Context, userID string, in contract.AtualizarUsuarioIn) (contract.Perfil, error) {
			if err := exigirUsuario(userID); err != nil {
				return contract.Perfil{}, err
			}
			p, err := svc.AtualizarUsuario(ctx, userID, in.ID, app.Mudanca{Status: perfil.Status(in.Status), Papel: perfil.Papel(in.Papel)})
			return p.Contract(), err
		}))

		r.Handle(contract.OpExcluirUsuario, invoke.Typed(func(ctx context.Context, userID string, in contract.ExcluirUsuarioIn) (any, error) {
			if err := exigirUsuario(userID); err != nil {
				return nil, err
			}
			return nil, svc.ExcluirUsuario(ctx, userID, in.ID)
		}))
	}

	return r
}

// Events monta o router dos eventos consumidos.
func Events(svc *app.Service, log *slog.Logger) *events.Router {
	r := events.NewRouter(log)
	r.Handle(events.UserConfirmed, events.Typed(func(ctx context.Context, _ events.Event, d events.UserConfirmedDetail) error {
		return svc.ConsumirUserConfirmed(ctx, d)
	}))
	return r
}

// exigirUsuario protege contra um envelope sem userId, que só acontece por
// bug no BFF: as operações sempre agem sobre o próprio usuário.
func exigirUsuario(userID string) error {
	if userID == "" {
		return errs.BadRequest("usuario_ausente", "Requisição sem usuário.")
	}
	return nil
}
