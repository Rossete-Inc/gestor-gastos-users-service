package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
)

// LambdaFunc é a assinatura passada a lambda.Start.
type LambdaFunc func(ctx context.Context, raw json.RawMessage) (any, error)

// Lambda devolve o handler único da função na AWS. A mesma função (alias
// live) recebe dois formatos:
//
//   - o envelope do invoke, chamado pelo BFF de forma síncrona;
//   - o evento do EventBridge, entregue de forma assíncrona pela regra de
//     UserConfirmed.
//
// O formato é reconhecido pelo campo "detail-type", que só o evento tem.
//
// No invoke, erros de domínio voltam dentro do envelope e a função não falha.
// No evento, o erro é devolvido para a AWS tentar de novo e, esgotadas as
// tentativas, mandar o evento para a DLQ.
func Lambda(svc *app.Service, log *slog.Logger) LambdaFunc {
	inv := Invoke(svc, log)
	ev := Events(svc, log)
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var sonda struct {
			DetailType *string `json:"detail-type"`
		}
		if err := json.Unmarshal(raw, &sonda); err != nil {
			return nil, fmt.Errorf("payload não é um objeto JSON: %w", err)
		}
		if sonda.DetailType != nil {
			var e events.Event
			if err := json.Unmarshal(raw, &e); err != nil {
				return nil, fmt.Errorf("evento inválido: %w", err)
			}
			return nil, ev.Dispatch(ctx, e)
		}
		var req invoke.Request
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("envelope do invoke inválido: %w", err)
		}
		return inv.Dispatch(ctx, req), nil
	}
}
