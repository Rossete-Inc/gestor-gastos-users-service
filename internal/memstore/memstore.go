// Package memstore é um app.Store em memória com a mesma semântica do store
// DynamoDB, inclusive a ordem e as chaves da listagem. Só é usado pelos
// testes e pelo cmd/local de testes.
package memstore

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// Store guarda os perfis por ID.
type Store struct {
	mu     sync.Mutex
	perfis map[string]perfil.Perfil
	// Criados conta os perfis criados de fato (não os já existentes).
	Criados int
	// Err, se não nil, é devolvido por todas as operações.
	Err error
}

// New cria um Store vazio.
func New() *Store { return &Store{perfis: map[string]perfil.Perfil{}} }

func naoEncontrado() error { return errs.NotFound("perfil_nao_encontrado", "Perfil não encontrado.") }

// Criar implementa app.Store.
func (m *Store) Criar(_ context.Context, p perfil.Perfil) (perfil.Perfil, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return perfil.Perfil{}, false, m.Err
	}
	if atual, ok := m.perfis[p.ID]; ok {
		return atual, false, nil
	}
	m.perfis[p.ID] = p
	m.Criados++
	return p, true, nil
}

// Obter implementa app.Store.
func (m *Store) Obter(_ context.Context, id string) (perfil.Perfil, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return perfil.Perfil{}, m.Err
	}
	if p, ok := m.perfis[id]; ok {
		return p, nil
	}
	return perfil.Perfil{}, naoEncontrado()
}

// Salvar implementa app.Store.
func (m *Store) Salvar(_ context.Context, p perfil.Perfil) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if _, ok := m.perfis[p.ID]; !ok {
		return naoEncontrado()
	}
	m.perfis[p.ID] = p
	return nil
}

// ContarAdminsAtivos implementa app.Store.
func (m *Store) ContarAdminsAtivos(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return 0, m.Err
	}
	n := 0
	for _, p := range m.perfis {
		if p.AdminAtivo() {
			n++
		}
	}
	return n, nil
}

// chave é a GSI1SK do Dynamo: data de criação e ID.
func chave(p perfil.Perfil) string {
	return p.CriadoEm.UTC().Format("2006-01-02T15:04:05.000Z") + "#" + p.ID
}

// Listar implementa app.Store, do mais novo para o mais antigo, com as
// mesmas chaves de continuação do GSI1.
func (m *Store) Listar(_ context.Context, limit int, inicio page.Key) ([]perfil.Perfil, page.Key, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, nil, m.Err
	}
	todos := make([]perfil.Perfil, 0, len(m.perfis))
	for _, p := range m.perfis {
		todos = append(todos, p)
	}
	slices.SortFunc(todos, func(a, b perfil.Perfil) int { return strings.Compare(chave(b), chave(a)) })

	limit = page.Limit(limit)
	itens := []perfil.Perfil{}
	for i, p := range todos {
		if inicio != nil && chave(p) >= inicio["GSI1SK"] {
			continue
		}
		itens = append(itens, p)
		if len(itens) == limit {
			if i == len(todos)-1 {
				return itens, nil, nil
			}
			return itens, page.Key{"PK": "USER#" + p.ID, "SK": "PROFILE", "GSI1PK": "USERS", "GSI1SK": chave(p)}, nil
		}
	}
	return itens, nil, nil
}
