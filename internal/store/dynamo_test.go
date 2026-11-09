package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Caminhos de erro que o DynamoDB Local não reproduz. O caminho feliz está
// nos testes de integração (dynamo_integration_test.go).

type fakeAPI struct {
	putErr   error
	getErr   error
	getItem  map[string]types.AttributeValue
	queryErr error
	queryOut *dynamodb.QueryOutput
}

func (f *fakeAPI) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{Item: f.getItem}, f.getErr
}

func (f *fakeAPI) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, f.putErr
}

func (f *fakeAPI) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if f.queryOut == nil {
		f.queryOut = &dynamodb.QueryOutput{}
	}
	return f.queryOut, f.queryErr
}

func perfilValido(t *testing.T) perfil.Perfil {
	t.Helper()
	p, err := perfil.Novo("u-1", "a@x.com", "Ana", perfil.Cliente, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func exigeInterno(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("esperava erro interno, veio nil")
	}
	if errs.KindOf(err) != errs.KindInternal {
		t.Errorf("erro = %v, quer interno", err)
	}
}

func TestErrosDoDynamo(t *testing.T) {
	ctx := context.Background()
	falha := errors.New("ProvisionedThroughputExceededException")

	t.Run("PutItem", func(t *testing.T) {
		_, _, err := NewDynamo(&fakeAPI{putErr: falha}, "t").Criar(ctx, perfilValido(t))
		exigeInterno(t, err)
	})
	t.Run("GetItem depois de condição falhar", func(t *testing.T) {
		api := &fakeAPI{putErr: &types.ConditionalCheckFailedException{}, getErr: falha}
		_, criado, err := NewDynamo(api, "t").Criar(ctx, perfilValido(t))
		exigeInterno(t, err)
		if criado {
			t.Error("criado deveria ser false")
		}
	})
	t.Run("GetItem", func(t *testing.T) {
		_, err := NewDynamo(&fakeAPI{getErr: falha}, "t").Obter(ctx, "u")
		exigeInterno(t, err)
	})
	t.Run("Query", func(t *testing.T) {
		_, _, err := NewDynamo(&fakeAPI{queryErr: falha}, "t").Listar(ctx, 10, map[string]string{"PK": "x"})
		exigeInterno(t, err)
	})
}

func TestItensCorrompidos(t *testing.T) {
	ctx := context.Background()
	ok := map[string]types.AttributeValue{
		"id": s("u"), "criadoEm": s("2026-11-09T12:00:00.000Z"), "atualizadoEm": s("2026-11-09T12:00:00.000Z"),
	}
	comCampo := func(k string, v types.AttributeValue) map[string]types.AttributeValue {
		m := map[string]types.AttributeValue{}
		for kk, vv := range ok {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	casos := map[string]map[string]types.AttributeValue{
		"tipo errado":           comCampo("id", &types.AttributeValueMemberBOOL{Value: true}),
		"criadoEm inválido":     comCampo("criadoEm", s("ontem")),
		"atualizadoEm inválido": comCampo("atualizadoEm", s("ontem")),
	}
	for nome, it := range casos {
		t.Run("Obter/"+nome, func(t *testing.T) {
			_, err := NewDynamo(&fakeAPI{getItem: it}, "t").Obter(ctx, "u")
			exigeInterno(t, err)
		})
		t.Run("Listar/"+nome, func(t *testing.T) {
			api := &fakeAPI{queryOut: &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{it}}}
			_, _, err := NewDynamo(api, "t").Listar(ctx, 10, nil)
			exigeInterno(t, err)
		})
	}

	t.Run("chave de paginação não string", func(t *testing.T) {
		api := &fakeAPI{queryOut: &dynamodb.QueryOutput{
			Items:            []map[string]types.AttributeValue{ok},
			LastEvaluatedKey: map[string]types.AttributeValue{"PK": &types.AttributeValueMemberN{Value: "1"}},
		}}
		_, _, err := NewDynamo(api, "t").Listar(ctx, 10, nil)
		exigeInterno(t, err)
	})
}
