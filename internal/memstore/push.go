package memstore

import (
	"context"
	"slices"
	"strings"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
)

// SalvarAssinatura implementa app.Store.
func (s *Store) SalvarAssinatura(_ context.Context, a push.Assinatura) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	if s.assinaturas[a.UserID] == nil {
		s.assinaturas[a.UserID] = map[string]push.Assinatura{}
	}
	s.assinaturas[a.UserID][a.ID()] = a
	return nil
}

// AssinaturasDe implementa app.Store, na ordem do ID como o Dynamo.
func (s *Store) AssinaturasDe(_ context.Context, userID string) ([]push.Assinatura, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return nil, s.Err
	}
	var out []push.Assinatura
	for _, a := range s.assinaturas[userID] {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b push.Assinatura) int { return strings.Compare(a.ID(), b.ID()) })
	return out, nil
}

// ExcluirAssinatura implementa app.Store.
func (s *Store) ExcluirAssinatura(_ context.Context, userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	delete(s.assinaturas[userID], id)
	return nil
}

// ExcluirAssinaturasDe implementa app.Store.
func (s *Store) ExcluirAssinaturasDe(_ context.Context, userID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return 0, s.Err
	}
	n := len(s.assinaturas[userID])
	delete(s.assinaturas, userID)
	return n, nil
}
