package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/notificacao"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
)

// enviadorFalso registra os envios; endpoints em expiradas respondem como
// assinatura vencida, e em quebrados, com erro.
type enviadorFalso struct {
	enviados  []string
	mensagens []notificacao.Mensagem
	expiradas map[string]bool
	quebrados map[string]bool
}

func (f *enviadorFalso) Enviar(_ context.Context, a push.Assinatura, m notificacao.Mensagem) (bool, error) {
	if f.quebrados[a.Endpoint] {
		return false, errors.New("serviço fora do ar")
	}
	if f.expiradas[a.Endpoint] {
		return true, nil
	}
	f.enviados = append(f.enviados, a.UserID+" "+a.Endpoint)
	f.mensagens = append(f.mensagens, m)
	return false, nil
}

var lazer80 = []notificacao.Alerta{{NomeCategoria: "Lazer", Mes: "2026-11", Limiar: 80, TotalCentavos: 82000, MetaCentavos: 100000}}

func TestNotificarMetasParaAContaToda(t *testing.T) {
	s, _ := comPessoas(t)
	aceitarBruno(t, s, colaboracao.Leitor) // Leitor também recebe
	f := &enviadorFalso{}
	s.ComPush(f)
	registrar(t, s, "ana", celular)
	registrar(t, s, "ana", notebook)
	registrar(t, s, "bruno", celular+"-bruno")
	registrar(t, s, "carla", celular+"-carla") // não é da conta da Ana

	// spec, "Notificação ao chegar a 80%": dono e colaboradores, todos os aparelhos
	if err := s.NotificarMetas(ctx, "ana", lazer80); err != nil {
		t.Fatal(err)
	}
	slices.Sort(f.enviados)
	want := []string{"ana " + celular, "ana " + notebook, "bruno " + celular + "-bruno"}
	if !slices.Equal(f.enviados, want) {
		t.Fatalf("enviados = %v, want %v", f.enviados, want)
	}
	if f.mensagens[0].Corpo != "Meta de Lazer: 80% usados (R$ 820,00 de R$ 1.000,00)." {
		t.Fatalf("mensagem = %+v", f.mensagens[0])
	}
}

func TestNotificarMetasAssinaturaExpiradaEFalha(t *testing.T) {
	s, st := comPessoas(t)
	f := &enviadorFalso{expiradas: map[string]bool{celular: true}, quebrados: map[string]bool{notebook: true}}
	s.ComPush(f)
	registrar(t, s, "ana", celular)
	registrar(t, s, "ana", notebook)

	// a falha de um aparelho não vira erro (evita reenvio aos outros)
	if err := s.NotificarMetas(ctx, "ana", lazer80); err != nil {
		t.Fatalf("NotificarMetas = %v", err)
	}
	// spec aplicativo-pwa, "Aparelho que não aceita mais notificações": a expirada sai, a outra fica
	lista, _ := st.AssinaturasDe(ctx, "ana")
	if len(lista) != 1 || lista[0].Endpoint != notebook {
		t.Fatalf("assinaturas = %+v", lista)
	}
}

func TestNotificarMetasSemAparelhosOuSemPush(t *testing.T) {
	s, st := comPessoas(t)
	// spec, "Nenhum aparelho ativado": nada a enviar, sem erro
	f := &enviadorFalso{}
	s.ComPush(f)
	if err := s.NotificarMetas(ctx, "ana", lazer80); err != nil || len(f.enviados) != 0 {
		t.Fatalf("sem aparelhos = %v, %v", f.enviados, err)
	}
	// sem alertas, nem consulta
	st.Err = errors.New("não deveria consultar")
	if err := s.NotificarMetas(ctx, "ana", nil); err != nil {
		t.Fatalf("sem alertas = %v", err)
	}
	// erro ao ler a conta vira erro (o EventBridge tenta de novo)
	if err := s.NotificarMetas(ctx, "ana", lazer80); err == nil {
		t.Fatal("falha do store deveria ser erro")
	}
	st.Err = nil
	// push desligado: só log
	sem := novoService(st)
	if err := sem.NotificarMetas(ctx, "ana", lazer80); err != nil {
		t.Fatalf("sem push = %v", err)
	}
}
