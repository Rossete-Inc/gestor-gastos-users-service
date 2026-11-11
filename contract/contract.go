// Package contract é o contrato público do users-service: os nomes das
// operações chamadas pelo BFF via invoke e os formatos de entrada e saída.
//
// É o único pacote não interno do módulo. O BFF o importa numa versão
// fixada, então mudar um campo aqui é mudança de contrato.
package contract

import (
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
)

// Operações atendidas pelo invoke.Router do serviço.
const (
	// OpObterPerfil devolve o perfil do userId do envelope. Sem payload.
	// Perfil inexistente: 404 "perfil_nao_encontrado".
	OpObterPerfil = "ObterPerfil"
	// OpCriarPerfil cria o perfil do userId do envelope com CriarPerfilIn.
	// É idempotente: se o perfil já existe, devolve o existente sem alterar.
	OpCriarPerfil = "CriarPerfil"

	// Operações de admin. O BFF só chama com o token de quem está no grupo
	// admin; o userId do envelope é o admin que age.

	// OpListarUsuarios devolve PaginaUsuarios, do mais novo para o mais
	// antigo. Cursor adulterado: 400 "cursor_invalido".
	OpListarUsuarios = "ListarUsuarios"
	// OpAtualizarUsuario bloqueia, reativa ou troca o papel do usuário
	// AtualizarUsuarioIn.ID e devolve o Perfil atualizado. Pedir o estado que
	// a conta já tem não muda nada. Erros: 404 "usuario_nao_encontrado"; 422
	// "validacao" (status, papel); 409 "transicao_invalida",
	// "bloquear_a_si_mesmo" ou "ultimo_admin".
	OpAtualizarUsuario = "AtualizarUsuario"
	// OpExcluirUsuario remove o acesso na hora e publica UserDeleted, que
	// apaga gastos e categorias do usuário nos outros serviços. Sem payload
	// na resposta. Repetir a exclusão republica o evento. Erros: 404
	// "usuario_nao_encontrado"; 409 "excluir_a_si_mesmo" ou "ultimo_admin".
	OpExcluirUsuario = "ExcluirUsuario"
)

// Códigos de erro das operações de admin (campo "code" do problem+json).
const (
	CodigoUsuarioNaoEncontrado = "usuario_nao_encontrado"
	CodigoTransicaoInvalida    = "transicao_invalida"
	CodigoBloquearASiMesmo     = "bloquear_a_si_mesmo"
	CodigoExcluirASiMesmo      = "excluir_a_si_mesmo"
	CodigoUltimoAdmin          = "ultimo_admin"
	CodigoCursorInvalido       = "cursor_invalido"
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

// ListarUsuariosIn é o payload de ListarUsuarios.
type ListarUsuariosIn struct {
	// Cursor é o NextCursor da página anterior. Vazio = primeira página.
	Cursor string `json:"cursor,omitempty"`
	// Limit de itens por página: padrão 20, máximo 100.
	Limit int `json:"limit,omitempty"`
}

// PaginaUsuarios é a resposta de ListarUsuarios: {"items":[...],"nextCursor":"..."},
// com items sempre presente e nextCursor ausente na última página.
type PaginaUsuarios = page.Page[Perfil]

// AtualizarUsuarioIn é o payload de AtualizarUsuario (UsuarioPatch do
// openapi do BFF mais o ID). Informe Status, Papel ou os dois.
type AtualizarUsuarioIn struct {
	ID string `json:"id"`
	// Status BLOQUEADO bloqueia; ATIVO reativa.
	Status string `json:"status,omitempty"`
	Papel  string `json:"papel,omitempty"`
}

// ExcluirUsuarioIn é o payload de ExcluirUsuario.
type ExcluirUsuarioIn struct {
	ID string `json:"id"`
}
