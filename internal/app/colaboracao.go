package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
)

// Colaboracoes persiste membros e convites das contas compartilhadas (ADR
// 0010). Faz parte do Store.
type Colaboracoes interface {
	// DaConta devolve os membros e os convites pendentes da conta.
	DaConta(ctx context.Context, donoID string) ([]colaboracao.Membro, []colaboracao.Convite, error)
	// ContasDoMembro devolve as participações do usuário em outras contas.
	ContasDoMembro(ctx context.Context, userID string) ([]colaboracao.Membro, error)
	// ConvitesDoEmail devolve os convites pendentes para o e-mail.
	ConvitesDoEmail(ctx context.Context, email string) ([]colaboracao.Convite, error)
	// ObterMembro devolve o membro, ou o 404 colaborador_nao_encontrado.
	ObterMembro(ctx context.Context, donoID, userID string) (colaboracao.Membro, error)
	// ObterConvite devolve o convite, ou o 404 convite_nao_encontrado.
	ObterConvite(ctx context.Context, donoID, email string) (colaboracao.Convite, error)
	// CriarConvite grava o convite; repetido é o 409 convite_existente.
	CriarConvite(ctx context.Context, c colaboracao.Convite) error
	// ExcluirConvite apaga o convite, ou devolve o 404.
	ExcluirConvite(ctx context.Context, donoID, email string) error
	// ExcluirMembro apaga o membro, ou devolve o 404.
	ExcluirMembro(ctx context.Context, donoID, userID string) error
	// SalvarMembro regrava um membro existente, ou devolve o 404.
	SalvarMembro(ctx context.Context, m colaboracao.Membro) error
	// AceitarConvite apaga o convite e grava o membro de uma vez; sem o
	// convite, devolve o 404 e não grava nada.
	AceitarConvite(ctx context.Context, c colaboracao.Convite, m colaboracao.Membro) error
	// ExcluirColaboracoesDe apaga a conta do usuário (membros e convites) e
	// as participações dele; devolve quantos itens apagou.
	ExcluirColaboracoesDe(ctx context.Context, userID string) (int, error)
}

// maxNomes limita os IDs de uma chamada a NomesDe.
const maxNomes = 100

func exigirID(campo, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return "", errs.Validation("Requisição inválida.", errs.FieldError{Field: campo, Message: "obrigatório"})
	}
	return id, nil
}

// ObterAcesso devolve o papel do usuário na conta do dono: DONO na própria,
// o papel de membro nas outras. Quem não é membro recebe 404, sem saber se a
// conta existe.
func (s *Service) ObterAcesso(ctx context.Context, userID, donoID string) (string, error) {
	donoID, err := exigirID("donoId", donoID)
	if err != nil {
		return "", err
	}
	if donoID == userID {
		return contract.PapelDono, nil
	}
	m, err := s.store.ObterMembro(ctx, donoID, userID)
	if err != nil && errs.KindOf(err) == errs.KindNotFound {
		return "", colaboracao.ErrContaNaoEncontrada()
	}
	if err != nil {
		return "", err
	}
	return string(m.Papel), nil
}

// ListarColaboradores devolve os membros e convites da conta do dono.
func (s *Service) ListarColaboradores(ctx context.Context, donoID string) ([]colaboracao.Membro, []colaboracao.Convite, error) {
	return s.store.DaConta(ctx, donoID)
}

// Convidar convida um e-mail para a conta do dono (spec colaboracao,
// "Convidar colaborador").
func (s *Service) Convidar(ctx context.Context, donoID, email string, papel colaboracao.Papel) (colaboracao.Convite, error) {
	dono, err := s.store.Obter(ctx, donoID)
	if err != nil {
		return colaboracao.Convite{}, err
	}
	membros, convites, err := s.store.DaConta(ctx, donoID)
	if err != nil {
		return colaboracao.Convite{}, err
	}
	c, err := colaboracao.NovoConvite(donoID, dono.Nome, dono.Email, email, papel, membros, convites, s.agora())
	if err != nil {
		return colaboracao.Convite{}, err
	}
	if err := s.store.CriarConvite(ctx, c); err != nil {
		return colaboracao.Convite{}, err
	}
	s.log.InfoContext(ctx, "convite criado", "donoId", donoID, "papel", c.Papel)
	return c, nil
}

// CancelarConvite apaga um convite pendente da conta do dono.
func (s *Service) CancelarConvite(ctx context.Context, donoID, email string) error {
	return s.store.ExcluirConvite(ctx, donoID, colaboracao.NormalizarEmail(email))
}

func emailVerificado(email string) (string, error) {
	e := colaboracao.NormalizarEmail(email)
	if !colaboracao.EmailValido(e) {
		return "", errs.Validation("Requisição inválida.", errs.FieldError{Field: "email", Message: "obrigatório"})
	}
	return e, nil
}

// ConvitesRecebidos devolve os convites para o e-mail verificado do usuário.
func (s *Service) ConvitesRecebidos(ctx context.Context, email string) ([]colaboracao.Convite, error) {
	e, err := emailVerificado(email)
	if err != nil {
		return nil, err
	}
	return s.store.ConvitesDoEmail(ctx, e)
}

// AceitarConvite torna o usuário membro da conta, com o papel do convite
// (spec, "Aceitar convite"). O e-mail é o verificado do token: só quem
// recebeu o convite o aceita.
func (s *Service) AceitarConvite(ctx context.Context, userID, email, donoID string) (colaboracao.Membro, error) {
	e, err := emailVerificado(email)
	if err != nil {
		return colaboracao.Membro{}, err
	}
	if donoID, err = exigirID("donoId", donoID); err != nil {
		return colaboracao.Membro{}, err
	}
	if donoID == userID {
		return colaboracao.Membro{}, colaboracao.ErrConviteNaoEncontrado()
	}
	c, err := s.store.ObterConvite(ctx, donoID, e)
	if err != nil {
		return colaboracao.Membro{}, err
	}
	eu, err := s.store.Obter(ctx, userID)
	if err != nil {
		return colaboracao.Membro{}, err
	}
	m := colaboracao.Membro{
		DonoID: donoID, NomeDono: c.NomeDono, UserID: userID, Nome: eu.Nome, Email: e,
		Papel: c.Papel, Desde: s.agora().UTC().Truncate(time.Millisecond),
	}
	if err := s.store.AceitarConvite(ctx, c, m); err != nil {
		return colaboracao.Membro{}, err
	}
	s.log.InfoContext(ctx, "convite aceito", "donoId", donoID, "userId", userID, "papel", m.Papel)
	return m, nil
}

// RecusarConvite apaga o convite recebido.
func (s *Service) RecusarConvite(ctx context.Context, email, donoID string) error {
	e, err := emailVerificado(email)
	if err != nil {
		return err
	}
	if donoID, err = exigirID("donoId", donoID); err != nil {
		return err
	}
	return s.store.ExcluirConvite(ctx, donoID, e)
}

// MudarPapel troca o papel de um membro da conta do dono.
func (s *Service) MudarPapel(ctx context.Context, donoID, userID string, papel colaboracao.Papel) (colaboracao.Membro, error) {
	if err := colaboracao.ValidarPapel(papel); err != nil {
		return colaboracao.Membro{}, err
	}
	m, err := s.store.ObterMembro(ctx, donoID, strings.TrimSpace(userID))
	if err != nil {
		return colaboracao.Membro{}, err
	}
	m.Papel = papel
	if err := s.store.SalvarMembro(ctx, m); err != nil {
		return colaboracao.Membro{}, err
	}
	s.log.InfoContext(ctx, "papel alterado", "donoId", donoID, "userId", m.UserID, "papel", papel)
	return m, nil
}

// RemoverColaborador tira um membro da conta do dono.
func (s *Service) RemoverColaborador(ctx context.Context, donoID, userID string) error {
	if err := s.store.ExcluirMembro(ctx, donoID, strings.TrimSpace(userID)); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "colaborador removido", "donoId", donoID, "userId", userID)
	return nil
}

// SairDaConta tira o usuário da conta de outra pessoa.
func (s *Service) SairDaConta(ctx context.Context, userID, donoID string) error {
	donoID, err := exigirID("donoId", donoID)
	if err != nil {
		return err
	}
	err = s.store.ExcluirMembro(ctx, donoID, userID)
	if err != nil && errs.KindOf(err) == errs.KindNotFound {
		return colaboracao.ErrContaNaoEncontrada()
	}
	if err == nil {
		s.log.InfoContext(ctx, "colaborador saiu da conta", "donoId", donoID, "userId", userID)
	}
	return err
}

// MinhasContas devolve as contas de outras pessoas em que o usuário é
// membro.
func (s *Service) MinhasContas(ctx context.Context, userID string) ([]colaboracao.Membro, error) {
	return s.store.ContasDoMembro(ctx, userID)
}

// NomesDe devolve o nome do perfil de cada ID; quem não tem perfil fica de
// fora.
func (s *Service) NomesDe(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) > maxNomes {
		return nil, errs.Validation("Requisição inválida.", errs.FieldError{Field: "ids", Message: "no máximo 100"})
	}
	nomes := map[string]string{}
	vistos := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(vistos, id) {
			continue
		}
		vistos = append(vistos, id)
		p, err := s.store.Obter(ctx, id)
		if err != nil && errs.KindOf(err) == errs.KindNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		nomes[id] = p.Nome
	}
	return nomes, nil
}
