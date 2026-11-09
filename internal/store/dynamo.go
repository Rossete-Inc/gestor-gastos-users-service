// Package store grava os perfis no DynamoDB.
//
// Chaves (contrato com o CDK e com o seed do infra-base, docs/local.md):
// PK=USER#<id>, SK=PROFILE e GSI1 com GSI1PK=USERS e GSI1SK=<criadoEm>#<id>
// para a listagem do admin.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// TableName é o nome base da tabela; na AWS o CDK acrescenta o ambiente.
const TableName = "gestor-gastos-users"

const (
	skPerfil    = "PROFILE"
	gsi1        = "GSI1"
	gsi1PKTodos = "USERS"
	// formatoData tem largura fixa, para GSI1SK ordenar como texto.
	formatoData = "2006-01-02T15:04:05.000Z"
)

// API é o subconjunto do client DynamoDB usado aqui.
type API interface {
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, opt ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, opt ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	Query(ctx context.Context, in *dynamodb.QueryInput, opt ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// Dynamo implementa app.Store.
type Dynamo struct {
	api   API
	table string
}

// NewDynamo cria o store na tabela table.
func NewDynamo(api API, table string) *Dynamo { return &Dynamo{api: api, table: table} }

type item struct {
	PK           string `dynamodbav:"PK"`
	SK           string `dynamodbav:"SK"`
	GSI1PK       string `dynamodbav:"GSI1PK"`
	GSI1SK       string `dynamodbav:"GSI1SK"`
	ID           string `dynamodbav:"id"`
	Email        string `dynamodbav:"email"`
	Nome         string `dynamodbav:"nome"`
	Papel        string `dynamodbav:"papel"`
	Status       string `dynamodbav:"status"`
	CriadoEm     string `dynamodbav:"criadoEm"`
	AtualizadoEm string `dynamodbav:"atualizadoEm"`
}

func pk(id string) string { return "USER#" + id }

func paraItem(p perfil.Perfil) item {
	criado := p.CriadoEm.UTC().Format(formatoData)
	return item{
		PK: pk(p.ID), SK: skPerfil,
		GSI1PK: gsi1PKTodos, GSI1SK: criado + "#" + p.ID,
		ID: p.ID, Email: p.Email, Nome: p.Nome,
		Papel: string(p.Papel), Status: string(p.Status),
		CriadoEm: criado, AtualizadoEm: p.AtualizadoEm.UTC().Format(formatoData),
	}
}

func (it item) perfil() (perfil.Perfil, error) {
	criado, err := time.Parse(formatoData, it.CriadoEm)
	if err != nil {
		return perfil.Perfil{}, fmt.Errorf("criadoEm %q: %w", it.CriadoEm, err)
	}
	atualizado, err := time.Parse(formatoData, it.AtualizadoEm)
	if err != nil {
		return perfil.Perfil{}, fmt.Errorf("atualizadoEm %q: %w", it.AtualizadoEm, err)
	}
	return perfil.Perfil{
		ID: it.ID, Email: it.Email, Nome: it.Nome,
		Papel: perfil.Papel(it.Papel), Status: perfil.Status(it.Status),
		CriadoEm: criado, AtualizadoEm: atualizado,
	}, nil
}

// Criar implementa app.Store com PutItem condicional: dois pedidos
// simultâneos para o mesmo usuário gravam uma vez só.
func (d *Dynamo) Criar(ctx context.Context, p perfil.Perfil) (perfil.Perfil, bool, error) {
	av, err := attributevalue.MarshalMap(paraItem(p))
	if err != nil {
		return perfil.Perfil{}, false, errs.Internal(fmt.Errorf("serializar perfil: %w", err))
	}
	_, err = d.api.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.table),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	var jaExiste *types.ConditionalCheckFailedException
	switch {
	case errors.As(err, &jaExiste):
		gravado, err := d.Obter(ctx, p.ID)
		return gravado, false, err
	case err != nil:
		return perfil.Perfil{}, false, errs.Internal(fmt.Errorf("gravar perfil: %w", err))
	}
	return p, true, nil
}

// Obter implementa app.Store com leitura consistente, para o perfil recém-
// criado aparecer na hora.
func (d *Dynamo) Obter(ctx context.Context, id string) (perfil.Perfil, error) {
	out, err := d.api.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(d.table),
		Key:            map[string]types.AttributeValue{"PK": s(pk(id)), "SK": s(skPerfil)},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return perfil.Perfil{}, errs.Internal(fmt.Errorf("ler perfil: %w", err))
	}
	if len(out.Item) == 0 {
		return perfil.Perfil{}, errs.NotFound("perfil_nao_encontrado", "Perfil não encontrado.")
	}
	var it item
	if err := attributevalue.UnmarshalMap(out.Item, &it); err != nil {
		return perfil.Perfil{}, errs.Internal(fmt.Errorf("decodificar perfil: %w", err))
	}
	p, err := it.perfil()
	if err != nil {
		return perfil.Perfil{}, errs.Internal(err)
	}
	return p, nil
}

// Listar implementa app.Store pelo GSI1, do mais novo para o mais antigo.
func (d *Dynamo) Listar(ctx context.Context, limit int, inicio page.Key) ([]perfil.Perfil, page.Key, error) {
	in := &dynamodb.QueryInput{
		TableName:                 aws.String(d.table),
		IndexName:                 aws.String(gsi1),
		KeyConditionExpression:    aws.String("GSI1PK = :todos"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":todos": s(gsi1PKTodos)},
		ScanIndexForward:          aws.Bool(false),
		Limit:                     aws.Int32(int32(page.Limit(limit))),
	}
	if len(inicio) > 0 {
		in.ExclusiveStartKey = make(map[string]types.AttributeValue, len(inicio))
		for k, v := range inicio {
			in.ExclusiveStartKey[k] = s(v)
		}
	}
	out, err := d.api.Query(ctx, in)
	if err != nil {
		return nil, nil, errs.Internal(fmt.Errorf("listar perfis: %w", err))
	}

	var itens []item
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &itens); err != nil {
		return nil, nil, errs.Internal(fmt.Errorf("decodificar perfis: %w", err))
	}
	perfis := make([]perfil.Perfil, 0, len(itens))
	for _, it := range itens {
		p, err := it.perfil()
		if err != nil {
			return nil, nil, errs.Internal(err)
		}
		perfis = append(perfis, p)
	}

	var proxima page.Key
	if len(out.LastEvaluatedKey) > 0 {
		proxima = make(page.Key, len(out.LastEvaluatedKey))
		for k, v := range out.LastEvaluatedKey {
			sv, ok := v.(*types.AttributeValueMemberS)
			if !ok {
				return nil, nil, errs.Internal(fmt.Errorf("chave %s da paginação não é string", k))
			}
			proxima[k] = sv.Value
		}
	}
	return perfis, proxima, nil
}

func s(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }
