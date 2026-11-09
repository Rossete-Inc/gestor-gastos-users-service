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
