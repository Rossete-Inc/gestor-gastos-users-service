// Package colaboracao é o domínio dos colaboradores de uma conta (change
// colaboradores, ADR 0010): membros com papel Editor ou Leitor e convites
// por e-mail, até 5 por conta somando os dois.
package colaboracao

import (
	"net/mail"
	"strings"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
)

// Papel de um membro.
type Papel string

const (
	Editor Papel = contract.PapelEditor
	Leitor Papel = contract.PapelLeitor
)

// Valido informa se o papel pode ser dado a um membro.
func (p Papel) Valido() bool { return p == Editor || p == Leitor }

// MaxEmail é o tamanho máximo de um e-mail (RFC 5321).
const MaxEmail = 254

// Membro é uma pessoa com acesso à conta do dono.
type Membro struct {
	DonoID   string
	NomeDono string
	UserID   string
	Nome     string
	Email    string
	Papel    Papel
	Desde    time.Time
}

// Convite pendente para um e-mail.
type Convite struct {
	DonoID   string
	NomeDono string
	// Email normalizado (NormalizarEmail).
	Email    string
	Papel    Papel
	CriadoEm time.Time
}

// NormalizarEmail tira os espaços das pontas e põe em minúsculas.
//
//	NormalizarEmail(" Ana@Exemplo.com ") // => "ana@exemplo.com"
func NormalizarEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// EmailValido informa se o e-mail normalizado tem forma de endereço.
func EmailValido(e string) bool {
	if e == "" || len(e) > MaxEmail || strings.ContainsAny(e, " <>") {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e
}

// Contrato converte para o formato do contrato público.
func (m Membro) Contrato() contract.Colaborador {
	return contract.Colaborador{UserID: m.UserID, Nome: m.Nome, Email: m.Email, Papel: string(m.Papel), Desde: m.Desde}
}

// Conta converte o membro na conta vista por ele.
func (m Membro) Conta() contract.Conta {
	return contract.Conta{DonoID: m.DonoID, NomeDono: m.NomeDono, Papel: string(m.Papel)}
}

// Contrato converte para o formato do contrato público.
func (c Convite) Contrato() contract.Convite {
	return contract.Convite{DonoID: c.DonoID, NomeDono: c.NomeDono, Email: c.Email, Papel: string(c.Papel), CriadoEm: c.CriadoEm}
}

// NovoConvite confere os dados de um convite (cenários "Convidar
// colaborador") contra os membros e convites atuais da conta e o e-mail do
// próprio dono.
func NovoConvite(donoID, nomeDono, emailDono, email string, papel Papel, membros []Membro, convites []Convite, agora time.Time) (Convite, error) {
	email = NormalizarEmail(email)
	var campos []errs.FieldError
	if !EmailValido(email) {
		campos = append(campos, errs.FieldError{Field: "email", Message: "informe um e-mail válido"})
	} else if email == NormalizarEmail(emailDono) {
		campos = append(campos, errs.FieldError{Field: "email", Message: "você não pode convidar a si mesmo"})
	}
	if !papel.Valido() {
		campos = append(campos, errs.FieldError{Field: "papel", Message: "deve ser EDITOR ou LEITOR"})
	}
	if len(campos) > 0 {
		return Convite{}, errs.Validation("Convite inválido.", campos...)
	}
	for _, m := range membros {
		if NormalizarEmail(m.Email) == email {
			return Convite{}, errs.Conflict(contract.CodigoJaColaborador, "Esta pessoa já colabora na sua conta.")
		}
	}
	for _, c := range convites {
		if c.Email == email {
			return Convite{}, errs.Conflict(contract.CodigoConviteExistente, "Já existe um convite para este e-mail.")
		}
	}
	if len(membros)+len(convites) >= contract.MaxColaboradores {
		return Convite{}, errs.Conflict(contract.CodigoLimiteColaboradores, "Sua conta já tem 5 colaboradores ou convites.")
	}
	return Convite{DonoID: donoID, NomeDono: nomeDono, Email: email, Papel: papel, CriadoEm: agora.UTC().Truncate(time.Millisecond)}, nil
}

// ValidarPapel recusa um papel que não seja EDITOR ou LEITOR.
func ValidarPapel(p Papel) error {
	if p.Valido() {
		return nil
	}
	return errs.Validation("Papel inválido.", errs.FieldError{Field: "papel", Message: "deve ser EDITOR ou LEITOR"})
}

// ErrContaNaoEncontrada é o 404 de quem não tem acesso à conta: não revela
// se ela existe.
func ErrContaNaoEncontrada() error {
	return errs.NotFound(contract.CodigoContaNaoEncontrada, "Conta não encontrada.")
}

// ErrConviteNaoEncontrado é o 404 de convite inexistente.
func ErrConviteNaoEncontrado() error {
	return errs.NotFound(contract.CodigoConviteNaoEncontrado, "Convite não encontrado.")
}

// ErrColaboradorNaoEncontrado é o 404 de quem não é membro da conta.
func ErrColaboradorNaoEncontrado() error {
	return errs.NotFound(contract.CodigoColaboradorNaoEncontrado, "Colaborador não encontrado.")
}
