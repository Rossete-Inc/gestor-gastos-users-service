// Package contract é o contrato público do users-service: os nomes das
// operações chamadas pelo BFF via invoke e os formatos de entrada e saída.
//
// É o único pacote não interno do módulo. O BFF o importa numa versão
// fixada, então mudar um campo aqui é mudança de contrato.
package contract

import "time"

// Operações atendidas pelo invoke.Router do serviço.
const (
	// OpObterPerfil devolve o perfil do userId do envelope. Sem payload.
	// Perfil inexistente: 404 "perfil_nao_encontrado".
	OpObterPerfil = "ObterPerfil"
	// OpCriarPerfil cria o perfil do userId do envelope com CriarPerfilIn.
	// É idempotente: se o perfil já existe, devolve o existente sem alterar.
	OpCriarPerfil = "CriarPerfil"
)

// Papéis.
const (
	PapelCliente = "cliente"
	PapelAdmin   = "admin"
)

// Status da conta.
const (
	StatusPendenteConfirmacao = "PENDENTE_CONFIRMACAO"
	StatusAtivo               = "ATIVO"
	StatusBloqueado           = "BLOQUEADO"
	StatusExcluido            = "EXCLUIDO"
)

// Perfil é a resposta de ObterPerfil e CriarPerfil.
type Perfil struct {
	ID       string    `json:"id"`
	Email    string    `json:"email"`
	Nome     string    `json:"nome"`
	Papel    string    `json:"papel"`
	Status   string    `json:"status"`
	CriadoEm time.Time `json:"criadoEm"`
}

// CriarPerfilIn é o payload de CriarPerfil. O BFF obtém e-mail e nome do
// Cognito e o papel dos grupos do token.
type CriarPerfilIn struct {
	Email string `json:"email"`
	Nome  string `json:"nome"`
	Papel string `json:"papel"`
}
