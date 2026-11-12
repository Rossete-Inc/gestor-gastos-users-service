package contas

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// cognitoFalso registra as chamadas ("Op pool usuario [grupo]") e devolve o
// erro configurado para a operação.
type cognitoFalso struct {
	chamadas []string
	erros    map[string]error
}

func (c *cognitoFalso) reg(op string, pool, user *string, grupo ...*string) error {
	s := op + " " + aws.ToString(pool) + " " + aws.ToString(user)
	for _, g := range grupo {
		s += " " + aws.ToString(g)
	}
	c.chamadas = append(c.chamadas, s)
	return c.erros[op]
}

func (c *cognitoFalso) AdminDisableUser(_ context.Context, in *cip.AdminDisableUserInput, _ ...func(*cip.Options)) (*cip.AdminDisableUserOutput, error) {
	return &cip.AdminDisableUserOutput{}, c.reg("Disable", in.UserPoolId, in.Username)
}

func (c *cognitoFalso) AdminEnableUser(_ context.Context, in *cip.AdminEnableUserInput, _ ...func(*cip.Options)) (*cip.AdminEnableUserOutput, error) {
	return &cip.AdminEnableUserOutput{}, c.reg("Enable", in.UserPoolId, in.Username)
}

func (c *cognitoFalso) AdminUserGlobalSignOut(_ context.Context, in *cip.AdminUserGlobalSignOutInput, _ ...func(*cip.Options)) (*cip.AdminUserGlobalSignOutOutput, error) {
	return &cip.AdminUserGlobalSignOutOutput{}, c.reg("SignOut", in.UserPoolId, in.Username)
}

func (c *cognitoFalso) AdminAddUserToGroup(_ context.Context, in *cip.AdminAddUserToGroupInput, _ ...func(*cip.Options)) (*cip.AdminAddUserToGroupOutput, error) {
	return &cip.AdminAddUserToGroupOutput{}, c.reg("Add", in.UserPoolId, in.Username, in.GroupName)
}

func (c *cognitoFalso) AdminRemoveUserFromGroup(_ context.Context, in *cip.AdminRemoveUserFromGroupInput, _ ...func(*cip.Options)) (*cip.AdminRemoveUserFromGroupOutput, error) {
	return &cip.AdminRemoveUserFromGroupOutput{}, c.reg("Remove", in.UserPoolId, in.Username, in.GroupName)
}

func (c *cognitoFalso) AdminDeleteUser(_ context.Context, in *cip.AdminDeleteUserInput, _ ...func(*cip.Options)) (*cip.AdminDeleteUserOutput, error) {
	return &cip.AdminDeleteUserOutput{}, c.reg("Delete", in.UserPoolId, in.Username)
}

var ctx = context.Background()

func TestCognitoChamaAsOperacoesDeAdmin(t *testing.T) {
	api := &cognitoFalso{}
	c := NewCognito(api, "pool-1")

	for _, err := range []error{
		c.Bloquear(ctx, "u-1"),
		c.Reativar(ctx, "u-1"),
		c.DefinirPapel(ctx, "u-1", perfil.Admin),
		c.DefinirPapel(ctx, "u-1", perfil.Cliente),
		c.Excluir(ctx, "u-1"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Bloquear revoga as sessões; trocar papel entra no grupo novo antes de
	// sair do antigo.
	quero := []string{
		"Disable pool-1 u-1", "SignOut pool-1 u-1",
		"Enable pool-1 u-1",
		"Add pool-1 u-1 admin", "Remove pool-1 u-1 cliente",
		"Add pool-1 u-1 cliente", "Remove pool-1 u-1 admin",
		"Delete pool-1 u-1",
	}
	if !slices.Equal(api.chamadas, quero) {
		t.Errorf("chamadas =\n%v\nquero\n%v", api.chamadas, quero)
	}
}

func TestCognitoExcluirContaQueJaNaoExiste(t *testing.T) {
	api := &cognitoFalso{erros: map[string]error{"Delete": &types.UserNotFoundException{Message: aws.String("não existe")}}}
	if err := NewCognito(api, "p").Excluir(ctx, "u-1"); err != nil {
		t.Errorf("excluir de novo = %v", err)
	}
}

func TestCognitoErrosViramInternos(t *testing.T) {
	fora := errors.New("cognito fora")
	casos := map[string]func(c *Cognito) error{
		"Disable": func(c *Cognito) error { return c.Bloquear(ctx, "u-1") },
		"SignOut": func(c *Cognito) error { return c.Bloquear(ctx, "u-1") },
		"Enable":  func(c *Cognito) error { return c.Reativar(ctx, "u-1") },
		"Add":     func(c *Cognito) error { return c.DefinirPapel(ctx, "u-1", perfil.Admin) },
		"Remove":  func(c *Cognito) error { return c.DefinirPapel(ctx, "u-1", perfil.Admin) },
		"Delete":  func(c *Cognito) error { return c.Excluir(ctx, "u-1") },
	}
	for op, chamar := range casos {
		api := &cognitoFalso{erros: map[string]error{op: fora}}
		err := chamar(NewCognito(api, "p"))
		if errs.KindOf(err) != errs.KindInternal || !errors.Is(err, fora) {
			t.Errorf("%s = %v", op, err)
		}
	}
}

func TestFake(t *testing.T) {
	f := NewFake()
	_ = f.Bloquear(ctx, "u-1")
	_ = f.DefinirPapel(ctx, "u-1", perfil.Admin)
	if !f.Bloqueada("u-1") || f.Papel("u-1") != perfil.Admin {
		t.Error("estado do fake")
	}
	_ = f.Reativar(ctx, "u-1")
	_ = f.Excluir(ctx, "u-1")
	if f.Bloqueada("u-1") || !f.Excluida("u-1") {
		t.Error("reativar/excluir no fake")
	}
	f.Err = errors.New("x")
	for _, err := range []error{f.Bloquear(ctx, "a"), f.Reativar(ctx, "a"), f.DefinirPapel(ctx, "a", perfil.Cliente), f.Excluir(ctx, "a")} {
		if err == nil {
			t.Error("fake ignorou Err")
		}
	}
	if len(f.Chamadas) != 8 {
		t.Errorf("chamadas = %v", f.Chamadas)
	}
}
