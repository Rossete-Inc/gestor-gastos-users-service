package memstore

import (
	"context"
	"slices"
	"strings"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
)

func ordenarMembros(l []colaboracao.Membro) []colaboracao.Membro {
	slices.SortFunc(l, func(a, b colaboracao.Membro) int { return strings.Compare(a.DonoID+a.UserID, b.DonoID+b.UserID) })
	return l
}

// DaConta implementa app.Store.
func (m *Store) DaConta(_ context.Context, donoID string) ([]colaboracao.Membro, []colaboracao.Convite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, nil, m.Err
	}
	membros, convites := []colaboracao.Membro{}, []colaboracao.Convite{}
	for _, x := range m.membros[donoID] {
		membros = append(membros, x)
	}
	for _, c := range m.convites[donoID] {
		convites = append(convites, c)
	}
	slices.SortFunc(convites, func(a, b colaboracao.Convite) int { return strings.Compare(a.Email, b.Email) })
	return ordenarMembros(membros), convites, nil
}

// ContasDoMembro implementa app.Store.
func (m *Store) ContasDoMembro(_ context.Context, userID string) ([]colaboracao.Membro, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	l := []colaboracao.Membro{}
	for _, conta := range m.membros {
		if x, ok := conta[userID]; ok {
			l = append(l, x)
		}
	}
	return ordenarMembros(l), nil
}

// ConvitesDoEmail implementa app.Store.
func (m *Store) ConvitesDoEmail(_ context.Context, email string) ([]colaboracao.Convite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	l := []colaboracao.Convite{}
	for _, conta := range m.convites {
		if c, ok := conta[email]; ok {
			l = append(l, c)
		}
	}
	slices.SortFunc(l, func(a, b colaboracao.Convite) int { return strings.Compare(a.DonoID, b.DonoID) })
	return l, nil
}

// ObterMembro implementa app.Store.
func (m *Store) ObterMembro(_ context.Context, donoID, userID string) (colaboracao.Membro, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return colaboracao.Membro{}, m.Err
	}
	x, ok := m.membros[donoID][userID]
	if !ok {
		return colaboracao.Membro{}, colaboracao.ErrColaboradorNaoEncontrado()
	}
	return x, nil
}

// ObterConvite implementa app.Store.
func (m *Store) ObterConvite(_ context.Context, donoID, email string) (colaboracao.Convite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return colaboracao.Convite{}, m.Err
	}
	c, ok := m.convites[donoID][email]
	if !ok {
		return colaboracao.Convite{}, colaboracao.ErrConviteNaoEncontrado()
	}
	return c, nil
}

// CriarConvite implementa app.Store.
func (m *Store) CriarConvite(_ context.Context, c colaboracao.Convite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.convites[c.DonoID][c.Email]; ok {
		return errs.Conflict(contract.CodigoConviteExistente, "Já existe um convite para este e-mail.")
	}
	if m.convites[c.DonoID] == nil {
		m.convites[c.DonoID] = map[string]colaboracao.Convite{}
	}
	m.convites[c.DonoID][c.Email] = c
	return nil
}

// ExcluirConvite implementa app.Store.
func (m *Store) ExcluirConvite(_ context.Context, donoID, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.convites[donoID][email]; !ok {
		return colaboracao.ErrConviteNaoEncontrado()
	}
	delete(m.convites[donoID], email)
	return nil
}

// ExcluirMembro implementa app.Store.
func (m *Store) ExcluirMembro(_ context.Context, donoID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.membros[donoID][userID]; !ok {
		return colaboracao.ErrColaboradorNaoEncontrado()
	}
	delete(m.membros[donoID], userID)
	return nil
}

// SalvarMembro implementa app.Store.
func (m *Store) SalvarMembro(_ context.Context, x colaboracao.Membro) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.membros[x.DonoID][x.UserID]; !ok {
		return colaboracao.ErrColaboradorNaoEncontrado()
	}
	m.membros[x.DonoID][x.UserID] = x
	return nil
}

// AceitarConvite implementa app.Store.
func (m *Store) AceitarConvite(_ context.Context, c colaboracao.Convite, x colaboracao.Membro) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.convites[c.DonoID][c.Email]; !ok {
		return colaboracao.ErrConviteNaoEncontrado()
	}
	delete(m.convites[c.DonoID], c.Email)
	if m.membros[x.DonoID] == nil {
		m.membros[x.DonoID] = map[string]colaboracao.Membro{}
	}
	m.membros[x.DonoID][x.UserID] = x
	return nil
}

// ExcluirColaboracoesDe implementa app.Store.
func (m *Store) ExcluirColaboracoesDe(_ context.Context, userID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return 0, m.Err
	}
	n := len(m.membros[userID]) + len(m.convites[userID])
	delete(m.membros, userID)
	delete(m.convites, userID)
	for _, conta := range m.membros {
		if _, ok := conta[userID]; ok {
			delete(conta, userID)
			n++
		}
	}
	return n, nil
}
