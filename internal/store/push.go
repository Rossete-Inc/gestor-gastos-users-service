package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/push"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Assinaturas Web Push (ADR 0011) ficam na partição do usuário, ao lado do
// perfil: PK=USER#<id>, SK=PUSH#<sha256(endpoint)>. Sem atributos de GSI1,
// não aparecem na listagem do admin.

const prefixoPush = "PUSH#"

type itemPush struct {
	PK       string `dynamodbav:"PK"`
	SK       string `dynamodbav:"SK"`
	Endpoint string `dynamodbav:"endpoint"`
	P256dh   string `dynamodbav:"p256dh"`
	Auth     string `dynamodbav:"auth"`
	CriadoEm string `dynamodbav:"criadoEm"`
}

func chavePush(userID, id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": s(pk(userID)), "SK": s(prefixoPush + id)}
}

// SalvarAssinatura implementa app.Store: grava por cima de uma assinatura
// do mesmo aparelho (idempotente).
func (d *Dynamo) SalvarAssinatura(ctx context.Context, a push.Assinatura) error {
	av, err := attributevalue.MarshalMap(itemPush{
		PK: pk(a.UserID), SK: prefixoPush + a.ID(),
		Endpoint: a.Endpoint, P256dh: a.P256dh, Auth: a.Auth, CriadoEm: a.CriadoEm.UTC().Format(formatoData),
	})
	if err != nil {
		return errs.Internal(fmt.Errorf("serializar assinatura: %w", err))
	}
	if _, err := d.api.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(d.table), Item: av}); err != nil {
		return errs.Internal(fmt.Errorf("gravar assinatura: %w", err))
	}
	return nil
}

// AssinaturasDe implementa app.Store: as assinaturas de todos os aparelhos
// do usuário.
func (d *Dynamo) AssinaturasDe(ctx context.Context, userID string) ([]push.Assinatura, error) {
	in := &dynamodb.QueryInput{
		TableName:              aws.String(d.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :push)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": s(pk(userID)), ":push": s(prefixoPush),
		},
	}
	var out []push.Assinatura
	for {
		res, err := d.api.Query(ctx, in)
		if err != nil {
			return nil, errs.Internal(fmt.Errorf("listar assinaturas: %w", err))
		}
		for _, raw := range res.Items {
			var it itemPush
			if err := attributevalue.UnmarshalMap(raw, &it); err != nil {
				return nil, errs.Internal(fmt.Errorf("decodificar assinatura: %w", err))
			}
			criado, err := time.Parse(formatoData, it.CriadoEm)
			if err != nil {
				return nil, errs.Internal(fmt.Errorf("criadoEm %q: %w", it.CriadoEm, err))
			}
			out = append(out, push.Assinatura{
				UserID: strings.TrimPrefix(it.PK, "USER#"), Endpoint: it.Endpoint, P256dh: it.P256dh, Auth: it.Auth, CriadoEm: criado,
			})
		}
		if len(res.LastEvaluatedKey) == 0 {
			return out, nil
		}
		in.ExclusiveStartKey = res.LastEvaluatedKey
	}
}

// ExcluirAssinatura implementa app.Store. Sem assinatura, não é erro.
func (d *Dynamo) ExcluirAssinatura(ctx context.Context, userID, id string) error {
	if _, err := d.api.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(d.table), Key: chavePush(userID, id)}); err != nil {
		return errs.Internal(fmt.Errorf("apagar assinatura: %w", err))
	}
	return nil
}

// ExcluirAssinaturasDe implementa app.Store: apaga todos os aparelhos do
// usuário e devolve quantos apagou.
func (d *Dynamo) ExcluirAssinaturasDe(ctx context.Context, userID string) (int, error) {
	lista, err := d.AssinaturasDe(ctx, userID)
	if err != nil {
		return 0, err
	}
	for _, a := range lista {
		if err := d.ExcluirAssinatura(ctx, userID, a.ID()); err != nil {
			return 0, err
		}
	}
	return len(lista), nil
}
