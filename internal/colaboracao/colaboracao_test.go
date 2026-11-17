package colaboracao

import (
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
)

var agora = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func convidar(email string, papel Papel, membros []Membro, convites []Convite) (Convite, error) {
	return NovoConvite("dono", "Ana", "ana@exemplo.com", email, papel, membros, convites, agora)
}

func TestConviteValido(t *testing.T) {
	c, err := convidar("  Bruno@Exemplo.com ", Editor, nil, nil)
	if err != nil || c.Email != "bruno@exemplo.com" || c.Papel != Editor || c.NomeDono != "Ana" || c.DonoID != "dono" {
		t.Fatalf("convite = %+v, %v", c, err)
	}
	if got := c.Contrato(); got.Email != "bruno@exemplo.com" || got.Papel != "EDITOR" {
		t.Errorf("contrato = %+v", got)
	}
}

func TestConvitesInvalidos(t *testing.T) {
	bruno := Membro{UserID: "b", Email: "Bruno@exemplo.com"}
	pendente := Convite{Email: "carla@exemplo.com"}
	quatro := []Convite{{Email: "1@x.com"}, {Email: "2@x.com"}, {Email: "3@x.com"}, {Email: "4@x.com"}}
	casos := map[string]struct {
		email    string
		papel    Papel
		membros  []Membro
		convites []Convite
		kind     errs.Kind
		codigo   string
	}{
		"e-mail inválido":  {"nao-e-email", Editor, nil, nil, errs.KindValidation, "validacao"},
		"vazio":            {"  ", Editor, nil, nil, errs.KindValidation, "validacao"},
		"o próprio":        {"ANA@exemplo.com", Leitor, nil, nil, errs.KindValidation, "validacao"},
		"papel inválido":   {"x@exemplo.com", "DONO", nil, nil, errs.KindValidation, "validacao"},
		"já colabora":      {"bruno@exemplo.com", Editor, []Membro{bruno}, nil, errs.KindConflict, contract.CodigoJaColaborador},
		"convite repetido": {"carla@exemplo.com", Leitor, nil, []Convite{pendente}, errs.KindConflict, contract.CodigoConviteExistente},
		"limite de 5":      {"novo@exemplo.com", Editor, []Membro{bruno}, quatro, errs.KindConflict, contract.CodigoLimiteColaboradores},
	}
	for nome, c := range casos {
		_, err := convidar(c.email, c.papel, c.membros, c.convites)
		e := errs.As(err)
		if e == nil || e.Kind != c.kind || (c.kind == errs.KindConflict && e.Code != c.codigo) {
			t.Errorf("%s: erro = %v", nome, err)
		}
	}
	// Com 4 ocupados ainda cabe um.
	if _, err := convidar("novo@exemplo.com", Editor, nil, quatro); err != nil {
		t.Errorf("quinto = %v", err)
	}
}

func TestEmail(t *testing.T) {
	if NormalizarEmail(" Ana@Exemplo.com ") != "ana@exemplo.com" {
		t.Error("normalizar")
	}
	for _, e := range []string{"ana@exemplo.com", "a.b+c@sub.exemplo.com.br"} {
		if !EmailValido(e) {
			t.Errorf("%s deveria valer", e)
		}
	}
	for _, e := range []string{"", "ana", "ana@", "Ana <ana@x.com>", "a b@x.com"} {
		if EmailValido(e) {
			t.Errorf("%q não deveria valer", e)
		}
	}
}

func TestPapel(t *testing.T) {
	if ValidarPapel(Leitor) != nil || ValidarPapel("ADMIN") == nil {
		t.Error("validar papel")
	}
	m := Membro{DonoID: "d", NomeDono: "Ana", UserID: "u", Papel: Leitor}
	if c := m.Conta(); c.DonoID != "d" || c.NomeDono != "Ana" || c.Papel != "LEITOR" {
		t.Errorf("conta = %+v", c)
	}
	for _, f := range []func() error{ErrContaNaoEncontrada, ErrConviteNaoEncontrado, ErrColaboradorNaoEncontrado} {
		if errs.KindOf(f()) != errs.KindNotFound {
			t.Error("404")
		}
	}
}
