package app

import (
	"context"
	"strings"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
)

// Assinaturas persiste as assinaturas Web Push dos aparelhos (ADR 0011).
// Faz parte do Store.
type Assinaturas interface {
	// SalvarAssinatura grava a assinatura, substituindo a do mesmo aparelho.
	SalvarAssinatura(ctx context.Context, a push.Assinatura) error
	// AssinaturasDe devolve as assinaturas de todos os aparelhos do usuário.
	AssinaturasDe(ctx context.Context, userID string) ([]push.Assinatura, error)
	// ExcluirAssinatura apaga uma assinatura; sem ela, não é erro.
	ExcluirAssinatura(ctx context.Context, userID, id string) error
	// ExcluirAssinaturasDe apaga todos os aparelhos do usuário e devolve
	// quantos apagou.
	ExcluirAssinaturasDe(ctx context.Context, userID string) (int, error)
}

// RegistrarAssinaturaPush liga as notificações no aparelho do usuário
// (spec aplicativo-pwa, "Ativar notificações"). Registrar o mesmo aparelho
// de novo só atualiza as chaves.
func (s *Service) RegistrarAssinaturaPush(ctx context.Context, userID, endpoint, p256dh, auth string) error {
	userID, err := exigirID("userId", userID)
	if err != nil {
		return err
	}
	a, err := push.Nova(userID, endpoint, p256dh, auth, s.agora())
	if err != nil {
		return err
	}
	return s.store.SalvarAssinatura(ctx, a)
}

// RemoverAssinaturaPush desliga as notificações no aparelho (spec,
// "Desativar notificações"); os outros aparelhos não mudam. Sem assinatura
// para o endpoint, não é erro.
func (s *Service) RemoverAssinaturaPush(ctx context.Context, userID, endpoint string) error {
	userID, err := exigirID("userId", userID)
	if err != nil {
		return err
	}
	endpoint = strings.TrimSpace(endpoint)
	if err := push.ValidarEndpoint(endpoint); err != nil {
		return err
	}
	return s.store.ExcluirAssinatura(ctx, userID, push.ID(endpoint))
}
