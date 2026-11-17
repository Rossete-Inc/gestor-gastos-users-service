package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/colaboracao"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Colaboração (ADR 0010), na mesma tabela dos perfis:
//
//	membro   PK=CONTA#<donoId>  SK=MEMBRO#<userId>  GSI2PK=MEMBRO#<userId>  GSI2SK=CONTA#<donoId>
//	convite  PK=CONTA#<donoId>  SK=CONVITE#<email>  GSI2PK=EMAIL#<email>    GSI2SK=CONTA#<donoId>
//
// O GSI2 responde "de quais contas participo" e "quais convites recebi".

const (
	gsi2           = "GSI2"
	prefixoConta   = "CONTA#"
	prefixoMembro  = "MEMBRO#"
	prefixoConvite = "CONVITE#"
	prefixoEmail   = "EMAIL#"
)

type itemColab struct {
	PK       string `dynamodbav:"PK"`
	SK       string `dynamodbav:"SK"`
	GSI2PK   string `dynamodbav:"GSI2PK"`
	GSI2SK   string `dynamodbav:"GSI2SK"`
	DonoID   string `dynamodbav:"donoId"`
	NomeDono string `dynamodbav:"nomeDono"`
	UserID   string `dynamodbav:"userId,omitempty"`
	Nome     string `dynamodbav:"nome,omitempty"`
	Email    string `dynamodbav:"email"`
	Papel    string `dynamodbav:"papel"`
	// Data é o "desde" do membro ou o "criadoEm" do convite.
	Data string `dynamodbav:"data"`
}

func pkConta(donoID string) string { return prefixoConta + donoID }

func chaveMembro(donoID, userID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": s(pkConta(donoID)), "SK": s(prefixoMembro + userID)}
}

func chaveConvite(donoID, email string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": s(pkConta(donoID)), "SK": s(prefixoConvite + email)}
}

func itemDoMembro(m colaboracao.Membro) (map[string]types.AttributeValue, error) {
	return marshalColab(itemColab{
		PK: pkConta(m.DonoID), SK: prefixoMembro + m.UserID,
		GSI2PK: prefixoMembro + m.UserID, GSI2SK: pkConta(m.DonoID),
		DonoID: m.DonoID, NomeDono: m.NomeDono, UserID: m.UserID, Nome: m.Nome, Email: m.Email,
		Papel: string(m.Papel), Data: m.Desde.UTC().Format(formatoData),
	})
}

func itemDoConvite(c colaboracao.Convite) (map[string]types.AttributeValue, error) {
	return marshalColab(itemColab{
		PK: pkConta(c.DonoID), SK: prefixoConvite + c.Email,
		GSI2PK: prefixoEmail + c.Email, GSI2SK: pkConta(c.DonoID),
		DonoID: c.DonoID, NomeDono: c.NomeDono, Email: c.Email,
		Papel: string(c.Papel), Data: c.CriadoEm.UTC().Format(formatoData),
	})
}

func marshalColab(it itemColab) (map[string]types.AttributeValue, error) {
	if it.DonoID == "" {
		return nil, errs.Internal(errors.New("colaboração sem donoId"))
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return nil, errs.Internal(fmt.Errorf("serializar colaboração: %w", err))
	}
	return av, nil
}

// decodificarColab devolve o membro ou o convite do item.
func decodificarColab(raw map[string]types.AttributeValue) (*colaboracao.Membro, *colaboracao.Convite, error) {
	var it itemColab
	if err := attributevalue.UnmarshalMap(raw, &it); err != nil {
		return nil, nil, errs.Internal(fmt.Errorf("decodificar colaboração: %w", err))
	}
	data, err := time.Parse(formatoData, it.Data)
	if err != nil {
		return nil, nil, errs.Internal(fmt.Errorf("data %q: %w", it.Data, err))
	}
	if strings.HasPrefix(it.SK, prefixoMembro) {
		return &colaboracao.Membro{
			DonoID: it.DonoID, NomeDono: it.NomeDono, UserID: it.UserID, Nome: it.Nome, Email: it.Email,
			Papel: colaboracao.Papel(it.Papel), Desde: data,
		}, nil, nil
	}
	return nil, &colaboracao.Convite{
		DonoID: it.DonoID, NomeDono: it.NomeDono, Email: it.Email, Papel: colaboracao.Papel(it.Papel), CriadoEm: data,
	}, nil
}

// consultar executa a Query seguindo as continuações e separa membros e
// convites.
func (d *Dynamo) consultar(ctx context.Context, in *dynamodb.QueryInput, oQue string) ([]colaboracao.Membro, []colaboracao.Convite, error) {
	membros, convites := []colaboracao.Membro{}, []colaboracao.Convite{}
	for {
		out, err := d.api.Query(ctx, in)
		if err != nil {
			return nil, nil, errs.Internal(fmt.Errorf("%s: %w", oQue, err))
		}
		for _, raw := range out.Items {
			m, c, err := decodificarColab(raw)
			if err != nil {
				return nil, nil, err
			}
			if m != nil {
				membros = append(membros, *m)
			} else {
				convites = append(convites, *c)
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			return membros, convites, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// DaConta implementa app.Store: membros e convites da conta, numa Query.
func (d *Dynamo) DaConta(ctx context.Context, donoID string) ([]colaboracao.Membro, []colaboracao.Convite, error) {
	return d.consultar(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.table),
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":pk": s(pkConta(donoID))},
		ConsistentRead:            aws.Bool(true),
	}, "ler colaboradores da conta")
}

func (d *Dynamo) porGSI2(ctx context.Context, chave, oQue string) ([]colaboracao.Membro, []colaboracao.Convite, error) {
	return d.consultar(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.table),
		IndexName:                 aws.String(gsi2),
		KeyConditionExpression:    aws.String("GSI2PK = :k"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":k": s(chave)},
	}, oQue)
}

// ContasDoMembro implementa app.Store: as participações do usuário (GSI2).
func (d *Dynamo) ContasDoMembro(ctx context.Context, userID string) ([]colaboracao.Membro, error) {
	m, _, err := d.porGSI2(ctx, prefixoMembro+userID, "ler contas do membro")
	return m, err
}

// ConvitesDoEmail implementa app.Store: os convites para o e-mail (GSI2).
func (d *Dynamo) ConvitesDoEmail(ctx context.Context, email string) ([]colaboracao.Convite, error) {
	_, c, err := d.porGSI2(ctx, prefixoEmail+email, "ler convites do e-mail")
	return c, err
}

// ObterMembro implementa app.Store.
func (d *Dynamo) ObterMembro(ctx context.Context, donoID, userID string) (colaboracao.Membro, error) {
	out, err := d.api.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(d.table), Key: chaveMembro(donoID, userID), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return colaboracao.Membro{}, errs.Internal(fmt.Errorf("ler membro: %w", err))
	}
	if len(out.Item) == 0 {
		return colaboracao.Membro{}, colaboracao.ErrColaboradorNaoEncontrado()
	}
	m, _, err := decodificarColab(out.Item)
	if err != nil || m == nil {
		return colaboracao.Membro{}, errs.Internal(fmt.Errorf("item de membro inválido: %v", err))
	}
	return *m, nil
}

// ObterConvite implementa app.Store.
func (d *Dynamo) ObterConvite(ctx context.Context, donoID, email string) (colaboracao.Convite, error) {
	out, err := d.api.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(d.table), Key: chaveConvite(donoID, email), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return colaboracao.Convite{}, errs.Internal(fmt.Errorf("ler convite: %w", err))
	}
	if len(out.Item) == 0 {
		return colaboracao.Convite{}, colaboracao.ErrConviteNaoEncontrado()
	}
	_, c, err := decodificarColab(out.Item)
	if err != nil || c == nil {
		return colaboracao.Convite{}, errs.Internal(fmt.Errorf("item de convite inválido: %v", err))
	}
	return *c, nil
}

// CriarConvite implementa app.Store; convite já existente é 409.
func (d *Dynamo) CriarConvite(ctx context.Context, c colaboracao.Convite) error {
	av, err := itemDoConvite(c)
	if err != nil {
		return err
	}
	_, err = d.api.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(d.table), Item: av, ConditionExpression: aws.String("attribute_not_exists(PK)")})
	var ccf *types.ConditionalCheckFailedException
	if errors.As(err, &ccf) {
		return errs.Conflict(contract.CodigoConviteExistente, "Já existe um convite para este e-mail.")
	}
	if err != nil {
		return errs.Internal(fmt.Errorf("gravar convite: %w", err))
	}
	return nil
}

// apagar remove o item com a chave; inexistente devolve naoExiste.
func (d *Dynamo) apagar(ctx context.Context, chave map[string]types.AttributeValue, naoExiste func() error, oQue string) error {
	_, err := d.api.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(d.table), Key: chave, ConditionExpression: aws.String("attribute_exists(PK)")})
	var ccf *types.ConditionalCheckFailedException
	if errors.As(err, &ccf) {
		return naoExiste()
	}
	if err != nil {
		return errs.Internal(fmt.Errorf("%s: %w", oQue, err))
	}
	return nil
}

// ExcluirConvite implementa app.Store.
func (d *Dynamo) ExcluirConvite(ctx context.Context, donoID, email string) error {
	return d.apagar(ctx, chaveConvite(donoID, email), colaboracao.ErrConviteNaoEncontrado, "apagar convite")
}

// ExcluirMembro implementa app.Store.
func (d *Dynamo) ExcluirMembro(ctx context.Context, donoID, userID string) error {
	return d.apagar(ctx, chaveMembro(donoID, userID), colaboracao.ErrColaboradorNaoEncontrado, "apagar membro")
}

// SalvarMembro implementa app.Store: regrava um membro existente.
func (d *Dynamo) SalvarMembro(ctx context.Context, m colaboracao.Membro) error {
	av, err := itemDoMembro(m)
	if err != nil {
		return err
	}
	_, err = d.api.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(d.table), Item: av, ConditionExpression: aws.String("attribute_exists(PK)")})
	var ccf *types.ConditionalCheckFailedException
	if errors.As(err, &ccf) {
		return colaboracao.ErrColaboradorNaoEncontrado()
	}
	if err != nil {
		return errs.Internal(fmt.Errorf("gravar membro: %w", err))
	}
	return nil
}

// AceitarConvite implementa app.Store: apaga o convite e grava o membro numa
// transação; sem o convite, nada muda (404).
func (d *Dynamo) AceitarConvite(ctx context.Context, c colaboracao.Convite, m colaboracao.Membro) error {
	av, err := itemDoMembro(m)
	if err != nil {
		return err
	}
	_, err = d.api.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Delete: &types.Delete{TableName: aws.String(d.table), Key: chaveConvite(c.DonoID, c.Email), ConditionExpression: aws.String("attribute_exists(PK)")}},
		{Put: &types.Put{TableName: aws.String(d.table), Item: av}},
	}})
	var tce *types.TransactionCanceledException
	if errors.As(err, &tce) && len(tce.CancellationReasons) > 0 && aws.ToString(tce.CancellationReasons[0].Code) == "ConditionalCheckFailed" {
		return colaboracao.ErrConviteNaoEncontrado()
	}
	if err != nil {
		return errs.Internal(fmt.Errorf("aceitar convite: %w", err))
	}
	return nil
}

// ExcluirColaboracoesDe implementa app.Store: apaga a conta do usuário
// (membros e convites) e as participações dele em outras contas. Rodar de
// novo não encontra nada.
func (d *Dynamo) ExcluirColaboracoesDe(ctx context.Context, userID string) (int, error) {
	membros, convites, err := d.DaConta(ctx, userID)
	if err != nil {
		return 0, err
	}
	participacoes, err := d.ContasDoMembro(ctx, userID)
	if err != nil {
		return 0, err
	}
	var chaves []map[string]types.AttributeValue
	for _, m := range membros {
		chaves = append(chaves, chaveMembro(m.DonoID, m.UserID))
	}
	for _, c := range convites {
		chaves = append(chaves, chaveConvite(c.DonoID, c.Email))
	}
	for _, m := range participacoes {
		chaves = append(chaves, chaveMembro(m.DonoID, m.UserID))
	}
	for _, k := range chaves {
		if _, err := d.api.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(d.table), Key: k}); err != nil {
			return 0, errs.Internal(fmt.Errorf("apagar colaboração: %w", err))
		}
	}
	return len(chaves), nil
}
