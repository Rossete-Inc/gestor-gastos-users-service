// Command lambda é a função do users-service na AWS (stack
// gestor-gastos-<env>-users, em infra/). Uma função só atende os dois
// formatos de entrada, pelo handler.Lambda:
//
//   - o envelope do invoke, chamado pelo BFF pelo alias live;
//   - o evento UserConfirmed, entregue pela regra do EventBridge.
//
// Variáveis de ambiente (definidas pelo CDK):
//
//	RUNTIME=aws          só roda na AWS; localmente use o cmd/local
//	ENV=dev              ambiente
//	USERS_TABLE=...      tabela DynamoDB dos perfis
//	USER_POOL_ID=...     opcional: User Pool das operações de admin
//	EVENT_BUS_NAME=...   opcional: barramento onde sai o UserDeleted
//	LOG_LEVEL=info       opcional
//
// USER_POOL_ID e EVENT_BUS_NAME ligam as operações de admin (bloquear,
// reativar, trocar papel, excluir) e vêm juntos; até o CDK defini-los
// (tarefa 10.4), a função atende só o perfil e a listagem de usuários.
//
// Build (o mesmo do go-service.yml):
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -ldflags="-s -w" \
//	  -o dist/lambda/bootstrap ./cmd/lambda
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/config"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/logging"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/contas"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/handler"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/store"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
)

const (
	servico = "users-service"
	// fonteDosEventos é o source dos eventos publicados (UserDeleted).
	fonteDosEventos = "gestor-gastos.users-service"
)

// Config é a configuração da Lambda.
type Config struct {
	Base  config.Base
	Table string
	// UserPoolID e BusName ligam as operações de admin; vazios, ficam
	// desligadas.
	UserPoolID string
	BusName    string
}

// Admin informa se as operações de admin estão configuradas.
func (c Config) Admin() bool { return c.UserPoolID != "" }

// LoadConfig lê o ambiente. Só aceita RUNTIME=aws e informa de uma vez todas
// as variáveis que faltam.
func LoadConfig(lookup config.LookupFunc) (Config, error) {
	base, err := config.LoadFrom(lookup, servico)
	if err != nil {
		return Config{}, err
	}
	if base.IsLocal() {
		return Config{}, errors.New("cmd/lambda exige RUNTIME=aws; localmente use o cmd/local")
	}
	r := config.NewReader(lookup)
	c := Config{
		Base: base, Table: r.Require("USERS_TABLE"),
		UserPoolID: r.Get("USER_POOL_ID", ""), BusName: r.Get("EVENT_BUS_NAME", ""),
	}
	if err := r.Err(); err != nil {
		return Config{}, err
	}
	if (c.UserPoolID == "") != (c.BusName == "") {
		return Config{}, errors.New("USER_POOL_ID e EVENT_BUS_NAME ligam juntos as operações de admin: defina os dois ou nenhum")
	}
	return c, nil
}

// novoHandler monta o handler da função com o client DynamoDB da AWS.
func novoHandler(ctx context.Context, lookup config.LookupFunc) (handler.LambdaFunc, error) {
	cfg, err := LoadConfig(lookup)
	if err != nil {
		return nil, err
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Base.Region))
	if err != nil {
		return nil, fmt.Errorf("carregar configuração da AWS: %w", err)
	}
	log := logging.New(logging.Options{Service: servico, Env: cfg.Base.Env, Level: cfg.Base.LogLevel})
	svc := app.New(store.NewDynamo(dynamodb.NewFromConfig(awsCfg), cfg.Table), log)
	if cfg.Admin() {
		pub, err := events.NewPublisher(events.Options{
			Runtime: cfg.Base.Runtime, Source: fonteDosEventos,
			BusName: cfg.BusName, API: eventbridge.NewFromConfig(awsCfg),
		})
		if err != nil {
			return nil, err
		}
		svc.ComAdmin(contas.NewCognito(cognitoidentityprovider.NewFromConfig(awsCfg), cfg.UserPoolID), pub)
	}
	log.Info("users-service configurado", "admin", cfg.Admin())
	return handler.Lambda(svc, log), nil
}

// executar monta o handler e o entrega a iniciar (lambda.Start). Devolve o
// código de saída do processo.
func executar(ctx context.Context, lookup config.LookupFunc, iniciar func(any)) int {
	h, err := novoHandler(ctx, lookup)
	if err != nil {
		slog.Error("configuração inválida", "erro", err.Error())
		return 1
	}
	iniciar(h)
	return 0
}

func main() {
	os.Exit(executar(context.Background(), os.LookupEnv, lambda.Start))
}
