// Package app tem os casos de uso do users-service. Não conhece transporte
// (invoke, eventos) nem banco: recebe um Store.
package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/contas"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// Store persiste perfis.
type Store interface {
	// Criar grava o perfil só se ainda não existir. Se já existir, devolve o
	// perfil gravado e criado=false, sem alterá-lo.
	Criar(ctx context.Context, p perfil.Perfil) (gravado perfil.Perfil, criado bool, err error)
	// Obter devolve o perfil ou errs.NotFound("perfil_nao_encontrado").
	Obter(ctx context.Context, id string) (perfil.Perfil, error)
	// Listar devolve os perfis do mais novo para o mais antigo, a partir de
	// inicio (nil = primeira página), e a chave da próxima página (nil = fim).
	Listar(ctx context.Context, limit int, inicio page.Key) ([]perfil.Perfil, page.Key, error)
	// Salvar regrava um perfil existente. Inexistente vira
	// errs.NotFound("perfil_nao_encontrado"), sem criar nada.
	Salvar(ctx context.Context, p perfil.Perfil) error
	// ContarAdminsAtivos conta os perfis admin com status ATIVO.
	ContarAdminsAtivos(ctx context.Context) (int, error)
}

// errAdminDesligado só aparece se o handler expuser uma operação de admin
// sem ComAdmin, o que seria bug: o handler só as registra com o admin ligado.
var errAdminDesligado = errors.New("operações de admin sem Cognito e barramento configurados")

// Service implementa os casos de uso.
type Service struct {
	store   Store
	log     *slog.Logger
	agora   func() time.Time
	contas  contas.Contas
	eventos events.Publisher
}

// New cria o Service. log nil usa slog.Default().
func New(store Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, log: log, agora: time.Now}
}

// ObterPerfil devolve o perfil do próprio usuário.
func (s *Service) ObterPerfil(ctx context.Context, userID string) (perfil.Perfil, error) {
	return s.store.Obter(ctx, userID)
}

// CriarPerfil cria o perfil do usuário de forma idempotente: chamado de
// novo, ou depois do evento UserConfirmed, devolve o perfil existente sem
// alterá-lo. Assim o /me pode criar o perfil sob demanda sem disputar com o
// consumidor do evento.
func (s *Service) CriarPerfil(ctx context.Context, userID, email, nome string, papel perfil.Papel) (perfil.Perfil, error) {
	p, err := perfil.Novo(userID, email, nome, papel, s.agora())
	if err != nil {
		return perfil.Perfil{}, err
	}
	gravado, criado, err := s.store.Criar(ctx, p)
	if err != nil {
		return perfil.Perfil{}, err
	}
	if criado {
		s.log.InfoContext(ctx, "perfil criado", "userId", gravado.ID, "papel", gravado.Papel)
	}
	return gravado, nil
}

// ConsumirUserConfirmed cria o perfil da conta recém-confirmada. O cadastro
// público só concede o papel cliente (spec gestao-usuarios, "Cadastro
// público"). Evento repetido não tem efeito.
func (s *Service) ConsumirUserConfirmed(ctx context.Context, d events.UserConfirmedDetail) error {
	_, err := s.CriarPerfil(ctx, d.UserID, d.Email, d.Nome, perfil.Cliente)
	return err
}
