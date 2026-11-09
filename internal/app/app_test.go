package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
)

// memStore é um Store em memória com a mesma semântica do Dynamo.
type memStore struct {
	mu      sync.Mutex
	perfis  map[string]perfil.Perfil
	criados int
	err     error
}

func newMem() *memStore { return &memStore{perfis: map[string]perfil.Perfil{}} }

func (m *memStore) Criar(_ context.Context, p perfil.Perfil) (perfil.Perfil, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return perfil.Perfil{}, false, m.err
	}
	if atual, ok := m.perfis[p.ID]; ok {
		return atual, false, nil
	}
	m.perfis[p.ID] = p
	m.criados++
	return p, true, nil
}

func (m *memStore) Obter(_ context.Context, id string) (perfil.Perfil, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.perfis[id]; ok {
		return p, nil
	}
	return perfil.Perfil{}, errs.NotFound("perfil_nao_encontrado", "Perfil não encontrado.")
}

func (m *memStore) Listar(context.Context, int, page.Key) ([]perfil.Perfil, page.Key, error) {
	return nil, nil, nil
}

func novoService(st Store) *Service {
	s := New(st, nil)
	s.agora = func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) }
	return s
}

func TestCriarEObter(t *testing.T) {
	ctx := context.Background()
	st := newMem()
	s := novoService(st)

	p, err := s.CriarPerfil(ctx, "u-1", "Ana@X.com", "Ana", perfil.Admin)
	if err != nil {
		t.Fatal(err)
	}
	if p.Email != "ana@x.com" || p.Papel != perfil.Admin || p.Status != perfil.Ativo {
		t.Errorf("perfil = %+v", p)
	}
	got, err := s.ObterPerfil(ctx, "u-1")
	if err != nil || got != p {
		t.Errorf("ObterPerfil = %+v, %v", got, err)
	}
}

func TestCriarEIdempotente(t *testing.T) {
	ctx := context.Background()
	st := newMem()
	s := novoService(st)

	primeiro, _ := s.CriarPerfil(ctx, "u-1", "ana@x.com", "Ana", perfil.Cliente)
	// Uma segunda chamada (o /me depois do evento, ou o evento repetido) não
	// altera o perfil, mesmo com dados diferentes.
	segundo, err := s.CriarPerfil(ctx, "u-1", "outro@x.com", "Outro Nome", perfil.Admin)
	if err != nil {
		t.Fatal(err)
	}
	if segundo != primeiro || st.criados != 1 {
		t.Errorf("segundo = %+v, criados = %d", segundo, st.criados)
	}
}

func TestCriarValidaAntesDoStore(t *testing.T) {
	st := newMem()
	_, err := novoService(st).CriarPerfil(context.Background(), "u-1", "invalido", "", perfil.Cliente)
	if errs.KindOf(err) != errs.KindValidation {
		t.Errorf("erro = %v", err)
	}
	if len(st.perfis) != 0 {
		t.Error("perfil inválido chegou ao store")
	}
}

func TestErroDoStore(t *testing.T) {
	st := newMem()
	st.err = errs.Internal(errors.New("dynamodb fora"))
	_, err := novoService(st).CriarPerfil(context.Background(), "u-1", "a@x.com", "Ana", perfil.Cliente)
	if errs.KindOf(err) != errs.KindInternal {
		t.Errorf("erro = %v", err)
	}
}

func TestObterInexistente(t *testing.T) {
	_, err := novoService(newMem()).ObterPerfil(context.Background(), "nao-existe")
	if !errors.Is(err, errs.NotFound("perfil_nao_encontrado", "")) {
		t.Errorf("erro = %v", err)
	}
}

func TestConsumirUserConfirmed(t *testing.T) {
	ctx := context.Background()
	st := newMem()
	s := novoService(st)
	d := events.UserConfirmedDetail{UserID: "u-9", Email: "bia@x.com", Nome: "Bia"}

	// Entrega "pelo menos uma vez": o mesmo evento chega três vezes.
	for range 3 {
		if err := s.ConsumirUserConfirmed(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := s.ObterPerfil(ctx, "u-9")
	// Cenário "Cadastro público": a conta recebe somente o papel cliente.
	if p.Papel != perfil.Cliente || p.Status != perfil.Ativo || st.criados != 1 {
		t.Errorf("perfil = %+v, criados = %d", p, st.criados)
	}
}

func TestEventoDepoisDoMe(t *testing.T) {
	// O admin do ambiente local nasce pelo /me com papel admin; um
	// UserConfirmed atrasado não pode rebaixá-lo para cliente.
	ctx := context.Background()
	s := novoService(newMem())
	if _, err := s.CriarPerfil(ctx, "adm", "admin@local.test", "Admin", perfil.Admin); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumirUserConfirmed(ctx, events.UserConfirmedDetail{UserID: "adm", Email: "admin@local.test", Nome: "Admin"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.ObterPerfil(ctx, "adm"); p.Papel != perfil.Admin {
		t.Errorf("papel = %s", p.Papel)
	}
}

func TestCriarConcorrente(t *testing.T) {
	ctx := context.Background()
	st := newMem()
	s := novoService(st)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := s.CriarPerfil(ctx, "u-1", "a@x.com", "Ana", perfil.Cliente); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if st.criados != 1 {
		t.Errorf("criados = %d", st.criados)
	}
}
