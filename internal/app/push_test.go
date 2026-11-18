package app

import (
	"encoding/base64"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
)

var (
	chaveP256dh = base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	chaveAuth   = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
)

const (
	celular  = "https://fcm.googleapis.com/fcm/send/celular"
	notebook = "https://updates.push.services.mozilla.com/wpush/v2/notebook"
)

func registrar(t *testing.T, s *Service, userID, endpoint string) {
	t.Helper()
	if err := s.RegistrarAssinaturaPush(ctx, userID, endpoint, chaveP256dh, chaveAuth); err != nil {
		t.Fatal(err)
	}
}

func TestAssinaturasPush(t *testing.T) {
	st := newMem()
	s := novoService(st)

	// spec aplicativo-pwa, "Ativar notificações": um registro por aparelho
	registrar(t, s, "ana", celular)
	registrar(t, s, "ana", notebook)
	registrar(t, s, "ana", celular) // de novo: substitui, não duplica
	registrar(t, s, "bruno", celular)
	lista, err := st.AssinaturasDe(ctx, "ana")
	if err != nil || len(lista) != 2 {
		t.Fatalf("aparelhos da ana = %+v, %v", lista, err)
	}

	// spec, "Desativar notificações": só aquele aparelho sai
	if err := s.RemoverAssinaturaPush(ctx, "ana", " "+celular+" "); err != nil {
		t.Fatal(err)
	}
	lista, _ = st.AssinaturasDe(ctx, "ana")
	if len(lista) != 1 || lista[0].Endpoint != notebook {
		t.Fatalf("depois de remover = %+v", lista)
	}
	if outra, _ := st.AssinaturasDe(ctx, "bruno"); len(outra) != 1 {
		t.Fatalf("remover da ana mexeu no bruno: %+v", outra)
	}
	// remover de novo não é erro
	if err := s.RemoverAssinaturaPush(ctx, "ana", celular); err != nil {
		t.Fatalf("remover de novo = %v", err)
	}
}

func TestAssinaturaPushInvalida(t *testing.T) {
	s := novoService(newMem())
	err := s.RegistrarAssinaturaPush(ctx, "ana", "http://inseguro.example/x", chaveP256dh, chaveAuth)
	if e := errs.As(err); e == nil || e.Kind != errs.KindValidation || e.Fields[0].Field != "endpoint" {
		t.Fatalf("endpoint http = %v", err)
	}
	if err := s.RegistrarAssinaturaPush(ctx, " ", celular, chaveP256dh, chaveAuth); errs.KindOf(err) != errs.KindValidation {
		t.Fatalf("sem usuário = %v", err)
	}
	if err := s.RemoverAssinaturaPush(ctx, "ana", "nao-e-url"); errs.KindOf(err) != errs.KindValidation {
		t.Fatalf("remover endpoint inválido = %v", err)
	}
}

func TestExclusaoApagaAssinaturas(t *testing.T) {
	c := montarAdmin(t)
	registrar(t, c.svc, "ana", celular)
	registrar(t, c.svc, "bia", celular)
	if err := c.svc.ExcluirUsuario(ctx, "adm", "ana"); err != nil {
		t.Fatal(err)
	}
	if lista, _ := c.st.AssinaturasDe(ctx, "ana"); len(lista) != 0 {
		t.Fatalf("aparelhos da ana depois da exclusão = %+v", lista)
	}
	if lista, _ := c.st.AssinaturasDe(ctx, "bia"); len(lista) != 1 {
		t.Fatalf("a exclusão da ana mexeu na bia: %+v", lista)
	}
}
