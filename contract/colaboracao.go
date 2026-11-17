package contract

import "time"

// Colaboração (change colaboradores, ADR 0010): o dono de uma conta convida
// pessoas, como Editor ou Leitor, para ver e lançar gastos na conta dele. O
// BFF resolve a conta ativa (cabeçalho X-Conta) com ObterAcesso e chama os
// serviços de dados com o userId do dono.
//
// Em todas as operações o userId do envelope é quem age: o dono (gestão) ou
// o convidado/colaborador. O e-mail do convidado vem do BFF, que o lê do
// token verificado (GetUser), nunca do corpo da requisição do app.
const (
	// OpObterAcesso devolve o papel do usuário do envelope na conta
	// ObterAcessoIn.DonoID: DONO se for a própria, EDITOR ou LEITOR se for
	// membro. Sem acesso: 404 "conta_nao_encontrada".
	OpObterAcesso = "ObterAcesso"
	// OpListarColaboradores devolve os membros e os convites pendentes da
	// conta do usuário do envelope.
	OpListarColaboradores = "ListarColaboradores"
	// OpConvidar convida ConvidarIn.Email com o papel. Erros: 422
	// "validacao" (email, papel, inclusive o próprio e-mail); 409
	// "limite_colaboradores" (5 entre membros e convites), "ja_colaborador"
	// ou "convite_existente".
	OpConvidar = "Convidar"
	// OpCancelarConvite apaga o convite da conta do envelope. Inexistente:
	// 404 "convite_nao_encontrado".
	OpCancelarConvite = "CancelarConvite"
	// OpListarConvitesRecebidos devolve os convites para o e-mail
	// verificado do usuário (ListarConvitesRecebidosIn.Email).
	OpListarConvitesRecebidos = "ListarConvitesRecebidos"
	// OpAceitarConvite torna o usuário do envelope membro da conta, com o
	// papel do convite, e apaga o convite. Sem convite para o e-mail: 404
	// "convite_nao_encontrado".
	OpAceitarConvite = "AceitarConvite"
	// OpRecusarConvite apaga o convite recebido. Inexistente: 404
	// "convite_nao_encontrado".
	OpRecusarConvite = "RecusarConvite"
	// OpMudarPapel troca o papel de um membro da conta do envelope.
	// Não membro: 404 "colaborador_nao_encontrado".
	OpMudarPapel = "MudarPapel"
	// OpRemoverColaborador tira um membro da conta do envelope. Não membro:
	// 404 "colaborador_nao_encontrado".
	OpRemoverColaborador = "RemoverColaborador"
	// OpSairDaConta tira o usuário do envelope da conta SairDaContaIn.DonoID.
	// Não membro: 404 "conta_nao_encontrada".
	OpSairDaConta = "SairDaConta"
	// OpListarMinhasContas devolve as contas de outras pessoas em que o
	// usuário do envelope é membro.
	OpListarMinhasContas = "ListarMinhasContas"
	// OpNomesDe devolve o nome do perfil de cada ID (autoria dos gastos). IDs
	// sem perfil ficam de fora do mapa. No máximo 100 IDs.
	OpNomesDe = "NomesDe"
)

// Papéis numa conta.
const (
	PapelDono   = "DONO"
	PapelEditor = "EDITOR"
	PapelLeitor = "LEITOR"
)

// MaxColaboradores é o limite de membros mais convites pendentes por conta.
const MaxColaboradores = 5

// Códigos de erro da colaboração.
const (
	CodigoContaNaoEncontrada       = "conta_nao_encontrada"
	CodigoConviteNaoEncontrado     = "convite_nao_encontrado"
	CodigoColaboradorNaoEncontrado = "colaborador_nao_encontrado"
	CodigoLimiteColaboradores      = "limite_colaboradores"
	CodigoJaColaborador            = "ja_colaborador"
	CodigoConviteExistente         = "convite_existente"
)

// ObterAcessoIn é o payload de ObterAcesso.
type ObterAcessoIn struct {
	DonoID string `json:"donoId"`
}

// Acesso é a resposta de ObterAcesso.
type Acesso struct {
	DonoID string `json:"donoId"`
	Papel  string `json:"papel"`
}

// Colaborador é um membro da conta.
type Colaborador struct {
	UserID string    `json:"userId"`
	Nome   string    `json:"nome"`
	Email  string    `json:"email"`
	Papel  string    `json:"papel"`
	Desde  time.Time `json:"desde"`
}

// Convite pendente para um e-mail.
type Convite struct {
	DonoID   string    `json:"donoId"`
	NomeDono string    `json:"nomeDono"`
	Email    string    `json:"email"`
	Papel    string    `json:"papel"`
	CriadoEm time.Time `json:"criadoEm"`
}

// Colaboradores é a resposta de ListarColaboradores, com as listas sempre
// presentes.
type Colaboradores struct {
	Membros  []Colaborador `json:"membros"`
	Convites []Convite     `json:"convites"`
}

// ConvidarIn é o payload de Convidar.
type ConvidarIn struct {
	Email string `json:"email"`
	Papel string `json:"papel"`
}

// CancelarConviteIn é o payload de CancelarConvite.
type CancelarConviteIn struct {
	Email string `json:"email"`
}

// ListarConvitesRecebidosIn é o payload de ListarConvitesRecebidos.
type ListarConvitesRecebidosIn struct {
	// Email verificado do usuário, lido pelo BFF do token.
	Email string `json:"email"`
}

// ConvitesRecebidos é a resposta de ListarConvitesRecebidos.
type ConvitesRecebidos struct {
	Items []Convite `json:"items"`
}

// ResponderConviteIn é o payload de AceitarConvite e RecusarConvite.
type ResponderConviteIn struct {
	DonoID string `json:"donoId"`
	// Email verificado do usuário, lido pelo BFF do token.
	Email string `json:"email"`
}

// Conta é uma conta de outra pessoa em que o usuário colabora.
type Conta struct {
	DonoID   string `json:"donoId"`
	NomeDono string `json:"nomeDono"`
	Papel    string `json:"papel"`
}

// MinhasContas é a resposta de ListarMinhasContas.
type MinhasContas struct {
	Items []Conta `json:"items"`
}

// MudarPapelIn é o payload de MudarPapel.
type MudarPapelIn struct {
	UserID string `json:"userId"`
	Papel  string `json:"papel"`
}

// RemoverColaboradorIn é o payload de RemoverColaborador.
type RemoverColaboradorIn struct {
	UserID string `json:"userId"`
}

// SairDaContaIn é o payload de SairDaConta.
type SairDaContaIn struct {
	DonoID string `json:"donoId"`
}

// NomesDeIn é o payload de NomesDe.
type NomesDeIn struct {
	IDs []string `json:"ids"`
}

// Nomes é a resposta de NomesDe: ID -> nome.
type Nomes struct {
	Nomes map[string]string `json:"nomes"`
}
