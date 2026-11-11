// Package contas faz no Cognito as operações de admin sobre as contas
// (design.md, "Status da conta"): o Cognito é a fonte da verdade do acesso,
// e o perfil no DynamoDB só espelha papel e status.
//
// O app conhece só a interface Contas. Na AWS, Cognito chama a API de admin
// do User Pool; no cmd/local e nos testes, Fake guarda tudo em memória, já
// que o cognito-local não suporta todas essas operações (tarefa 10.1).
//
// O username de cada conta é o próprio sub, que é o ID do perfil: o pool
// entra por e-mail (SignInAliases), e o Cognito gera o username igual ao sub.
package contas

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// Contas são as operações de admin sobre o acesso de uma conta.
type Contas interface {
	// Bloquear impede novos logins e revoga os refresh tokens (o access
	// token já emitido expira em até 15 minutos, design.md).
	Bloquear(ctx context.Context, id string) error
	// Reativar volta a permitir o login.
	Reativar(ctx context.Context, id string) error
	// DefinirPapel põe a conta no grupo do papel e a tira do outro. O papel
	// novo vale a partir do próximo token (spec, "Promover a admin").
	DefinirPapel(ctx context.Context, id string, papel perfil.Papel) error
	// Excluir apaga a conta. Conta que já não existe não é erro, para a
	// exclusão poder ser repetida.
	Excluir(ctx context.Context, id string) error
}

// CognitoAPI é o subconjunto do client do Cognito usado aqui.
type CognitoAPI interface {
	AdminDisableUser(ctx context.Context, in *cip.AdminDisableUserInput, opt ...func(*cip.Options)) (*cip.AdminDisableUserOutput, error)
	AdminEnableUser(ctx context.Context, in *cip.AdminEnableUserInput, opt ...func(*cip.Options)) (*cip.AdminEnableUserOutput, error)
	AdminUserGlobalSignOut(ctx context.Context, in *cip.AdminUserGlobalSignOutInput, opt ...func(*cip.Options)) (*cip.AdminUserGlobalSignOutOutput, error)
	AdminAddUserToGroup(ctx context.Context, in *cip.AdminAddUserToGroupInput, opt ...func(*cip.Options)) (*cip.AdminAddUserToGroupOutput, error)
	AdminRemoveUserFromGroup(ctx context.Context, in *cip.AdminRemoveUserFromGroupInput, opt ...func(*cip.Options)) (*cip.AdminRemoveUserFromGroupOutput, error)
	AdminDeleteUser(ctx context.Context, in *cip.AdminDeleteUserInput, opt ...func(*cip.Options)) (*cip.AdminDeleteUserOutput, error)
}

// Cognito implementa Contas no User Pool.
type Cognito struct {
	api  CognitoAPI
	pool string
}

// NewCognito cria o adaptador para o User Pool pool.
func NewCognito(api CognitoAPI, pool string) *Cognito { return &Cognito{api: api, pool: pool} }

// Bloquear implementa Contas: desabilita e encerra todas as sessões.
func (c *Cognito) Bloquear(ctx context.Context, id string) error {
	if _, err := c.api.AdminDisableUser(ctx, &cip.AdminDisableUserInput{UserPoolId: aws.String(c.pool), Username: aws.String(id)}); err != nil {
		return falha("desabilitar a conta", err)
	}
	if _, err := c.api.AdminUserGlobalSignOut(ctx, &cip.AdminUserGlobalSignOutInput{UserPoolId: aws.String(c.pool), Username: aws.String(id)}); err != nil {
		return falha("encerrar as sessões", err)
	}
	return nil
}

// Reativar implementa Contas.
func (c *Cognito) Reativar(ctx context.Context, id string) error {
	if _, err := c.api.AdminEnableUser(ctx, &cip.AdminEnableUserInput{UserPoolId: aws.String(c.pool), Username: aws.String(id)}); err != nil {
		return falha("reativar a conta", err)
	}
	return nil
}

// DefinirPapel implementa Contas. Primeiro entra no grupo novo e só depois
// sai do antigo: se a segunda chamada falhar, a conta fica nos dois grupos
// (vale admin, PapelDosGrupos) e repetir a operação conserta.
func (c *Cognito) DefinirPapel(ctx context.Context, id string, papel perfil.Papel) error {
	antigo := perfil.Cliente
	if papel == perfil.Cliente {
		antigo = perfil.Admin
	}
	if _, err := c.api.AdminAddUserToGroup(ctx, &cip.AdminAddUserToGroupInput{
		UserPoolId: aws.String(c.pool), Username: aws.String(id), GroupName: aws.String(string(papel)),
	}); err != nil {
		return falha("pôr a conta no grupo "+string(papel), err)
	}
	if _, err := c.api.AdminRemoveUserFromGroup(ctx, &cip.AdminRemoveUserFromGroupInput{
		UserPoolId: aws.String(c.pool), Username: aws.String(id), GroupName: aws.String(string(antigo)),
	}); err != nil {
		return falha("tirar a conta do grupo "+string(antigo), err)
	}
	return nil
}

// Excluir implementa Contas.
func (c *Cognito) Excluir(ctx context.Context, id string) error {
	_, err := c.api.AdminDeleteUser(ctx, &cip.AdminDeleteUserInput{UserPoolId: aws.String(c.pool), Username: aws.String(id)})
	var naoExiste *types.UserNotFoundException
	if errors.As(err, &naoExiste) {
		return nil
	}
	if err != nil {
		return falha("excluir a conta", err)
	}
	return nil
}

func falha(oque string, err error) error {
	return errs.Internal(fmt.Errorf("cognito: %s: %w", oque, err))
}

// Fake implementa Contas em memória, para o cmd/local e os testes.
type Fake struct {
	mu        sync.Mutex
	bloqueada map[string]bool
	papel     map[string]perfil.Papel
	excluida  map[string]bool
	// Chamadas registra as operações, na ordem ("Bloquear u-1").
	Chamadas []string
	// Err, se não nil, é devolvido por todas as operações.
	Err error
}

// NewFake cria um Fake vazio.
func NewFake() *Fake {
	return &Fake{bloqueada: map[string]bool{}, papel: map[string]perfil.Papel{}, excluida: map[string]bool{}}
}

func (f *Fake) registrar(op, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Chamadas = append(f.Chamadas, op+" "+id)
	return f.Err
}

// Bloquear implementa Contas.
func (f *Fake) Bloquear(_ context.Context, id string) error {
	if err := f.registrar("Bloquear", id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bloqueada[id] = true
	return nil
}

// Reativar implementa Contas.
func (f *Fake) Reativar(_ context.Context, id string) error {
	if err := f.registrar("Reativar", id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.bloqueada, id)
	return nil
}

// DefinirPapel implementa Contas.
func (f *Fake) DefinirPapel(_ context.Context, id string, papel perfil.Papel) error {
	if err := f.registrar("DefinirPapel:"+string(papel), id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.papel[id] = papel
	return nil
}

// Excluir implementa Contas.
func (f *Fake) Excluir(_ context.Context, id string) error {
	if err := f.registrar("Excluir", id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.excluida[id] = true
	return nil
}

// Bloqueada informa se a conta está bloqueada no Fake.
func (f *Fake) Bloqueada(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bloqueada[id]
}

// Papel devolve o papel definido no Fake, ou "" se nunca foi definido.
func (f *Fake) Papel(id string) perfil.Papel {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.papel[id]
}

// Excluida informa se a conta foi excluída no Fake.
func (f *Fake) Excluida(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.excluida[id]
}
