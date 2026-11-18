package app

import (
	"context"
	"strings"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/notificacao"
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

// fusoBrasilia é America/Sao_Paulo, em UTC-3 fixo (o Brasil não tem horário
// de verão desde 2019), como no BFF.
var fusoBrasilia = time.FixedZone("BRT", -3*60*60)

// ComPush liga o envio das notificações (ADR 0011). Sem ele, os alertas
// são só registrados em log.
func (s *Service) ComPush(e notificacao.Enviador) *Service {
	s.push = e
	return s
}

// NotificarMetas envia uma notificação com os alertas de uma escrita a
// todos os aparelhos do dono e dos colaboradores da conta (spec
// metas-gastos, "Notificação push da meta"). Uma assinatura expirada é
// apagada. Falha no envio a um aparelho só vai para o log: devolver erro
// faria o EventBridge repetir o evento e reenviar aos que já receberam.
func (s *Service) NotificarMetas(ctx context.Context, donoID string, alertas []notificacao.Alerta) error {
	if len(alertas) == 0 {
		return nil
	}
	if s.push == nil {
		s.log.WarnContext(ctx, "push desligado; alertas de meta não enviados", "donoId", donoID, "alertas", len(alertas))
		return nil
	}
	membros, _, err := s.store.DaConta(ctx, donoID)
	if err != nil {
		return err
	}
	destinatarios := []string{donoID}
	for _, m := range membros {
		destinatarios = append(destinatarios, m.UserID)
	}
	msg := notificacao.Montar(alertas, s.agora().In(fusoBrasilia).Format("2006-01"))

	enviados, expiradas := 0, 0
	for _, userID := range destinatarios {
		aparelhos, err := s.store.AssinaturasDe(ctx, userID)
		if err != nil {
			return err
		}
		for _, a := range aparelhos {
			expirada, err := s.push.Enviar(ctx, a, msg)
			switch {
			case err != nil:
				s.log.WarnContext(ctx, "falha ao enviar push", "userId", userID, "erro", err)
			case expirada:
				expiradas++
				if err := s.store.ExcluirAssinatura(ctx, userID, a.ID()); err != nil {
					s.log.WarnContext(ctx, "falha ao apagar assinatura expirada", "userId", userID, "erro", err)
				}
			default:
				enviados++
			}
		}
	}
	s.log.InfoContext(ctx, "alertas de meta notificados", "donoId", donoID, "alertas", len(alertas), "enviados", enviados, "expiradas", expiradas)
	return nil
}
