// Package perfil é o domínio do users-service: perfil, papéis e o ciclo de
// vida da conta (spec gestao-usuarios).
//
// O Cognito é a fonte da verdade do acesso (design.md, "Status da conta");
// o perfil espelha papel e status para a listagem do admin e para regras
// como "último admin".
package perfil

import (
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
)

// Papel de acesso. Todo usuário ativo tem exatamente um.
type Papel string

const (
	Cliente Papel = contract.PapelCliente
	Admin   Papel = contract.PapelAdmin
)

// Valido informa se o papel existe.
func (p Papel) Valido() bool { return p == Cliente || p == Admin }

// PapelDosGrupos deriva o papel dos grupos do Cognito: admin se estiver no
// grupo admin, cliente em qualquer outro caso, inclusive sem grupo (o caso
// do ambiente local, que não tem o trigger Post Confirmation).
func PapelDosGrupos(grupos []string) Papel {
	if slices.Contains(grupos, string(Admin)) {
		return Admin
	}
	return Cliente
}

// Status da conta.
type Status string

const (
	PendenteConfirmacao Status = contract.StatusPendenteConfirmacao
	Ativo               Status = contract.StatusAtivo
	Bloqueado           Status = contract.StatusBloqueado
	Excluido            Status = contract.StatusExcluido
)

// transicoes são as únicas mudanças de status permitidas pela spec:
// pendente→ativo, ativo↔bloqueado e ativo ou bloqueado→excluído.
var transicoes = map[Status][]Status{
	PendenteConfirmacao: {Ativo},
	Ativo:               {Bloqueado, Excluido},
	Bloqueado:           {Ativo, Excluido},
}

// PodeIrPara informa se a transição de s para novo é permitida.
func (s Status) PodeIrPara(novo Status) bool { return slices.Contains(transicoes[s], novo) }

// Limites dos campos.
const (
	MaxNome  = 100
	MaxEmail = 254
)

// Perfil de um usuário. ID é o sub do Cognito.
type Perfil struct {
	ID           string
	Email        string
	Nome         string
	Papel        Papel
	Status       Status
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Novo cria o perfil de uma conta recém-confirmada, já com status ATIVO: o
// perfil só nasce depois da confirmação do e-mail (evento UserConfirmed ou
// primeiro /me). E-mail é normalizado em minúsculas e nome sem espaços nas
// pontas.
func Novo(id, email, nome string, papel Papel, agora time.Time) (Perfil, error) {
	id = strings.TrimSpace(id)
	email = strings.ToLower(strings.TrimSpace(email))
	nome = strings.Join(strings.Fields(nome), " ")

	var campos []errs.FieldError
	if id == "" {
		campos = append(campos, errs.FieldError{Field: "id", Message: "obrigatório"})
	}
	if msg := validarEmail(email); msg != "" {
		campos = append(campos, errs.FieldError{Field: "email", Message: msg})
	}
	if msg := validarNome(nome); msg != "" {
		campos = append(campos, errs.FieldError{Field: "nome", Message: msg})
	}
	if !papel.Valido() {
		campos = append(campos, errs.FieldError{Field: "papel", Message: "deve ser cliente ou admin"})
	}
	if len(campos) > 0 {
		return Perfil{}, errs.Validation("Dados do perfil inválidos.", campos...)
	}

	// Milissegundos: a mesma precisão gravada no banco, para o perfil devolvido
	// na criação ser igual ao lido depois.
	agora = agora.UTC().Truncate(time.Millisecond)
	return Perfil{ID: id, Email: email, Nome: nome, Papel: papel, Status: Ativo, CriadoEm: agora, AtualizadoEm: agora}, nil
}

func validarEmail(email string) string {
	if email == "" {
		return "obrigatório"
	}
	if len(email) > MaxEmail {
		return "muito longo"
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || a.Name != "" {
		return "formato inválido"
	}
	return ""
}

func validarNome(nome string) string {
	if nome == "" {
		return "obrigatório"
	}
	if utf8.RuneCountInString(nome) > MaxNome {
		return "deve ter no máximo 100 caracteres"
	}
	for _, r := range nome {
		if unicode.IsControl(r) {
			return "contém caracteres inválidos"
		}
	}
	return ""
}

// MudarStatus aplica uma transição do ciclo de vida. Transição fora da
// máquina de estados, como reativar uma conta EXCLUIDO, é recusada com 409.
func (p *Perfil) MudarStatus(novo Status, agora time.Time) error {
	if !p.Status.PodeIrPara(novo) {
		return errs.Conflict("transicao_invalida", "A conta não pode passar de "+string(p.Status)+" para "+string(novo)+".")
	}
	p.Status = novo
	p.AtualizadoEm = agora.UTC().Truncate(time.Millisecond)
	return nil
}

// Bloquear, Reativar e Excluir são as transições feitas pelo admin.
func (p *Perfil) Bloquear(agora time.Time) error { return p.MudarStatus(Bloqueado, agora) }
func (p *Perfil) Reativar(agora time.Time) error { return p.MudarStatus(Ativo, agora) }
func (p *Perfil) Excluir(agora time.Time) error  { return p.MudarStatus(Excluido, agora) }

// MudarPapel troca o papel. A regra do último admin depende dos outros
// perfis e fica no app.
func (p *Perfil) MudarPapel(novo Papel, agora time.Time) error {
	if !novo.Valido() {
		return errs.Validation("Papel inválido.", errs.FieldError{Field: "papel", Message: "deve ser cliente ou admin"})
	}
	p.Papel = novo
	p.AtualizadoEm = agora.UTC().Truncate(time.Millisecond)
	return nil
}

// AdminAtivo informa se o perfil conta para a regra do último admin.
func (p Perfil) AdminAtivo() bool { return p.Papel == Admin && p.Status == Ativo }

// Contract converte para o formato do contrato público.
func (p Perfil) Contract() contract.Perfil {
	return contract.Perfil{
		ID:       p.ID,
		Email:    p.Email,
		Nome:     p.Nome,
		Papel:    string(p.Papel),
		Status:   string(p.Status),
		CriadoEm: p.CriadoEm,
	}
}
