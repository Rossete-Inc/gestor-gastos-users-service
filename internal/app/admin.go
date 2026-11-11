package app

import (
	"context"
	"strings"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/contas"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// Casos de uso do admin (spec gestao-usuarios). Quem chama é sempre um
// admin: o BFF só aceita /admin/* com o token do grupo admin. adminID é o
// usuário do envelope, isto é, o admin que age.
//
// A ordem é sempre Cognito primeiro e perfil depois: o Cognito decide o
// acesso, e se a gravação do perfil falhar, repetir a operação conserta o
// espelho (as operações no Cognito são idempotentes).
//
// Limitação aceita no MVP: a regra do último admin conta os admins ativos e
// age em seguida, sem transação. Dois admins rebaixando um ao outro no mesmo
// instante poderiam deixar o sistema sem admin; o primeiro admin é recriado
// pela CLI (docs/primeiro-admin.md do infra-base).

// ComAdmin liga as operações de bloquear, reativar, trocar papel e excluir.
// Sem isso (Lambda antes da tarefa 10.4), só a listagem funciona.
func (s *Service) ComAdmin(c contas.Contas, p events.Publisher) *Service {
	s.contas, s.eventos = c, p
	return s
}

// AdminHabilitado informa se ComAdmin foi chamado.
func (s *Service) AdminHabilitado() bool { return s.contas != nil && s.eventos != nil }

func usuarioNaoEncontrado() error {
	return errs.NotFound(contract.CodigoUsuarioNaoEncontrado, "Usuário não encontrado.")
}

// alvo lê o perfil sobre o qual o admin age. Perfil inexistente vira 404
// usuario_nao_encontrado, o código das rotas /admin.
func (s *Service) alvo(ctx context.Context, id string) (perfil.Perfil, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return perfil.Perfil{}, usuarioNaoEncontrado()
	}
	p, err := s.store.Obter(ctx, id)
	if err != nil && errs.KindOf(err) == errs.KindNotFound {
		return perfil.Perfil{}, usuarioNaoEncontrado()
	}
	return p, err
}

// ListarUsuarios devolve uma página de perfis, do mais novo para o mais
// antigo.
func (s *Service) ListarUsuarios(ctx context.Context, cursor string, limit int) (page.Page[perfil.Perfil], error) {
	inicio, err := decodificarCursor(cursor)
	if err != nil {
		return page.Page[perfil.Perfil]{}, err
	}
	itens, proxima, err := s.store.Listar(ctx, page.Limit(limit), inicio)
	if err != nil {
		return page.Page[perfil.Perfil]{}, err
	}
	return page.New(itens, codificarCursor(proxima)), nil
}

// Mudanca é o que o admin pede em AtualizarUsuario. Campo vazio = não muda.
type Mudanca struct {
	Status perfil.Status
	Papel  perfil.Papel
}

func validarMudanca(m Mudanca) error {
	var campos []errs.FieldError
	if m.Status == "" && m.Papel == "" {
		campos = append(campos, errs.FieldError{Field: "status", Message: "informe status, papel ou os dois"})
	}
	if m.Status != "" && m.Status != perfil.Ativo && m.Status != perfil.Bloqueado {
		campos = append(campos, errs.FieldError{Field: "status", Message: "deve ser ATIVO ou BLOQUEADO"})
	}
	if m.Papel != "" && !m.Papel.Valido() {
		campos = append(campos, errs.FieldError{Field: "papel", Message: "deve ser cliente ou admin"})
	}
	if len(campos) > 0 {
		return errs.Validation("Alteração de usuário inválida.", campos...)
	}
	return nil
}

// AtualizarUsuario bloqueia, reativa ou troca o papel de um usuário.
// Cenários: "Admin bloqueia usuário", "Admin reativa usuário", "Admin tenta
// bloquear a si mesmo", "Promover a admin", "Último admin" e "Transição
// inválida".
func (s *Service) AtualizarUsuario(ctx context.Context, adminID, id string, m Mudanca) (perfil.Perfil, error) {
	if !s.AdminHabilitado() {
		return perfil.Perfil{}, errs.Internal(errAdminDesligado)
	}
	if err := validarMudanca(m); err != nil {
		return perfil.Perfil{}, err
	}
	atual, err := s.alvo(ctx, id)
	if err != nil {
		return perfil.Perfil{}, err
	}
	if atual.Status == perfil.Excluido {
		return perfil.Perfil{}, errs.Conflict(contract.CodigoTransicaoInvalida, "A conta foi excluída e não pode mais ser alterada.")
	}

	agora := s.agora()
	novo := atual
	mudouPapel := m.Papel != "" && m.Papel != atual.Papel
	mudouStatus := m.Status != "" && m.Status != atual.Status
	if mudouStatus {
		if m.Status == perfil.Bloqueado && atual.ID == adminID {
			return perfil.Perfil{}, errs.Conflict(contract.CodigoBloquearASiMesmo, "Você não pode bloquear a própria conta.")
		}
		if err := novo.MudarStatus(m.Status, agora); err != nil {
			return perfil.Perfil{}, err
		}
	}
	if mudouPapel {
		if err := novo.MudarPapel(m.Papel, agora); err != nil {
			return perfil.Perfil{}, err
		}
	}
	if !mudouPapel && !mudouStatus {
		return atual, nil // a conta já está como pedido
	}
	if err := s.exigirOutroAdmin(ctx, atual, novo); err != nil {
		return perfil.Perfil{}, err
	}

	// Cognito primeiro: é ele que decide o acesso.
	if mudouPapel {
		if err := s.contas.DefinirPapel(ctx, novo.ID, novo.Papel); err != nil {
			return perfil.Perfil{}, err
		}
	}
	if mudouStatus {
		trocar := s.contas.Reativar
		if novo.Status == perfil.Bloqueado {
			trocar = s.contas.Bloquear
		}
		if err := trocar(ctx, novo.ID); err != nil {
			return perfil.Perfil{}, err
		}
	}
	if err := s.salvarAlvo(ctx, novo); err != nil {
		return perfil.Perfil{}, err
	}
	s.log.InfoContext(ctx, "usuário atualizado pelo admin", "adminId", adminID, "userId", novo.ID,
		"status", novo.Status, "papel", novo.Papel)
	return novo, nil
}

// ExcluirUsuario remove o acesso na hora (Cognito), marca o perfil como
// EXCLUIDO e publica UserDeleted, que apaga gastos e categorias do usuário
// nos outros serviços (cenário "Admin exclui usuário"). Repetir a exclusão
// de um perfil já EXCLUIDO republica o evento: cobre a falha na publicação.
func (s *Service) ExcluirUsuario(ctx context.Context, adminID, id string) error {
	if !s.AdminHabilitado() {
		return errs.Internal(errAdminDesligado)
	}
	atual, err := s.alvo(ctx, id)
	if err != nil {
		return err
	}
	if atual.ID == adminID {
		return errs.Conflict(contract.CodigoExcluirASiMesmo, "Você não pode excluir a própria conta.")
	}
	if atual.Status != perfil.Excluido {
		novo := atual
		if err := novo.Excluir(s.agora()); err != nil {
			return err
		}
		if err := s.exigirOutroAdmin(ctx, atual, novo); err != nil {
			return err
		}
		if err := s.contas.Excluir(ctx, novo.ID); err != nil {
			return err
		}
		if err := s.salvarAlvo(ctx, novo); err != nil {
			return err
		}
	}
	if err := s.eventos.Publish(ctx, events.UserDeleted, events.UserDeletedDetail{UserID: atual.ID}); err != nil {
		return errs.Internal(err)
	}
	s.log.InfoContext(ctx, "usuário excluído pelo admin", "adminId", adminID, "userId", atual.ID)
	return nil
}

// exigirOutroAdmin aplica "Último admin": se a mudança tira um admin ativo
// da contagem, precisa sobrar pelo menos um.
func (s *Service) exigirOutroAdmin(ctx context.Context, antes, depois perfil.Perfil) error {
	if !antes.AdminAtivo() || depois.AdminAtivo() {
		return nil
	}
	n, err := s.store.ContarAdminsAtivos(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errs.Conflict(contract.CodigoUltimoAdmin, "É preciso manter pelo menos um administrador ativo.")
	}
	return nil
}

// salvarAlvo grava o perfil; se ele sumiu entre a leitura e a gravação,
// responde como inexistente.
func (s *Service) salvarAlvo(ctx context.Context, p perfil.Perfil) error {
	err := s.store.Salvar(ctx, p)
	if err != nil && errs.KindOf(err) == errs.KindNotFound {
		return usuarioNaoEncontrado()
	}
	return err
}
