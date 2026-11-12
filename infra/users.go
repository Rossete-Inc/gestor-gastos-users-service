package main

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscodedeploy"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdadestinations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/aws-cdk-go/awscdk/v2/interfaces/interfacesawscloudwatch"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// UsersProps configura a stack users.
type UsersProps struct {
	awscdk.StackProps
	Ambiente string
	// Codigo é o pacote da Lambda. nil = binário ausente: a stack registra
	// MsgFaltaBuild e não é sintetizada.
	Codigo awslambda.Code
	// Parametros são os valores lidos do SSM no synth (LerParametrosSSM).
	// nil = parâmetros do CloudFormation resolvidos no deploy.
	Parametros Parametros
}

// NovaStackUsers cria a stack gestor-gastos-<env>-users:
//
//   - tabela DynamoDB on-demand com RETAIN (PK/SK e GSI1, como internal/store);
//   - Lambda arm64 de 256 MB com alias live, que atende o invoke do BFF e o
//     UserConfirmed;
//   - CodeDeploy com canary de 10% por 5 minutos e rollback automático por
//     falha ou alarme de erros;
//   - regra do EventBridge para UserConfirmed, com retentativas e DLQ;
//   - operações de admin: Cognito Admin só no User Pool do ambiente e
//     PutEvents no barramento, para o UserDeleted;
//   - logs por 7 dias;
//   - ARN do alias e nome da tabela publicados no SSM para o BFF.
func NovaStackUsers(scope constructs.Construct, id string, props *UsersProps) awscdk.Stack {
	stack := awscdk.NewStack(scope, jsii.String(id), &props.StackProps)
	env := props.Ambiente

	if props.Codigo == nil {
		awscdk.Annotations_Of(stack).AddError(jsii.String(MsgFaltaBuild))
		return stack
	}

	params := props.Parametros
	if params == nil {
		params = ParametrosCloudFormation(stack, env)
	}

	nome := NomeStack(env, NomeServico)

	tabela := novaTabela(stack, nome)
	alias := novaFuncao(stack, env, nome, props.Codigo, tabela, params)
	permissoesDeAdmin(alias, params)

	dlq := awssqs.Queue_FromQueueArn(stack, jsii.String("Dlq"), jsii.String(params[ParamDlqArn]))

	// Erro da função numa invocação assíncrona (o EventBridge entrega assim):
	// a Lambda tenta mais 2 vezes e depois manda o registro para a DLQ do
	// barramento. A DLQ do target da regra (abaixo) só cobre falhas de entrega
	// (throttling, permissão), não erros da função.
	alias.ConfigureAsyncInvoke(&awslambda.EventInvokeConfigOptions{
		OnFailure:     awslambdadestinations.NewSqsDestination(dlq),
		RetryAttempts: jsii.Number(2),
		MaxEventAge:   awscdk.Duration_Hours(jsii.Number(1)),
	})

	novoCanary(stack, nome, alias)

	barramento := awsevents.EventBus_FromEventBusName(stack, jsii.String("Barramento"), jsii.String(params[ParamBusName]))
	awsevents.NewRule(stack, jsii.String("UserConfirmed"), &awsevents.RuleProps{
		RuleName:    jsii.String(nome + "-user-confirmed"),
		Description: jsii.String("UserConfirmed do trigger Post Confirmation: cria o perfil no users-service"),
		EventBus:    barramento,
		EventPattern: &awsevents.EventPattern{
			Source:     jsii.Strings(SourceAuth),
			DetailType: jsii.Strings(DetailTypeUserConfirmed),
		},
		Targets: &[]awsevents.IRuleTarget{
			awseventstargets.NewLambdaFunction(alias, &awseventstargets.LambdaFunctionProps{
				DeadLetterQueue: dlq,
				RetryAttempts:   jsii.Number(8),
				MaxEventAge:     awscdk.Duration_Hours(jsii.Number(2)),
			}),
		},
	})

	publicarParametro(stack, env, ParamFunctionArn, *alias.FunctionArn(),
		"ARN do alias live da Lambda do users-service, invocado pelo BFF")
	publicarParametro(stack, env, ParamTableName, *tabela.TableName(),
		"Nome da tabela DynamoDB do users-service")

	// Saída informativa (sem exportName: quem consome lê do SSM).
	awscdk.NewCfnOutput(stack, jsii.String("FunctionArn"), &awscdk.CfnOutputProps{
		Value:       alias.FunctionArn(),
		Description: jsii.String("ARN do alias live (também em " + CaminhoSSM(env, ParamFunctionArn) + ")"),
	})

	return stack
}

// novaTabela cria a tabela no formato de internal/store: PK=USER#<id>,
// SK=PROFILE e GSI1 (GSI1PK=USERS, GSI1SK=<criadoEm>#<id>) com projeção ALL.
// On-demand: sem custo parada além do armazenamento. RETAIN: apagar a stack
// não apaga os perfis.
func novaTabela(stack awscdk.Stack, nome string) awsdynamodb.Table {
	texto := func(n string) *awsdynamodb.Attribute {
		return &awsdynamodb.Attribute{Name: jsii.String(n), Type: awsdynamodb.AttributeType_STRING}
	}
	tabela := awsdynamodb.NewTable(stack, jsii.String("Tabela"), &awsdynamodb.TableProps{
		TableName:     jsii.String(nome),
		PartitionKey:  texto("PK"),
		SortKey:       texto("SK"),
		BillingMode:   awsdynamodb.BillingMode_PAY_PER_REQUEST,
		RemovalPolicy: awscdk.RemovalPolicy_RETAIN,
	})
	tabela.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName:      jsii.String("GSI1"),
		PartitionKey:   texto("GSI1PK"),
		SortKey:        texto("GSI1SK"),
		ProjectionType: awsdynamodb.ProjectionType_ALL,
	})
	return tabela
}

// novaFuncao cria a Lambda e o alias live, que aponta sempre para a versão
// atual. Quem move o tráfego do alias entre versões é o CodeDeploy.
func novaFuncao(stack awscdk.Stack, env, nome string, codigo awslambda.Code, tabela awsdynamodb.Table, params Parametros) awslambda.Alias {
	logs := awslogs.NewLogGroup(stack, jsii.String("Logs"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String("/aws/lambda/" + nome),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	fn := awslambda.NewFunction(stack, jsii.String("Funcao"), &awslambda.FunctionProps{
		FunctionName: jsii.String(nome),
		Description:  jsii.String("users-service: invoke do BFF e consumidor de UserConfirmed"),
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		Handler:      jsii.String("bootstrap"),
		Code:         codigo,
		MemorySize:   jsii.Number(256),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(10)),
		LogGroup:     logs,
		Environment: &map[string]*string{
			"RUNTIME":     jsii.String("aws"),
			"ENV":         jsii.String(env),
			"USERS_TABLE": tabela.TableName(),
			"LOG_LEVEL":   jsii.String("info"),
			// Os dois juntos ligam as operações de admin (cmd/lambda).
			"USER_POOL_ID":   jsii.String(params[ParamUserPoolID]),
			"EVENT_BUS_NAME": jsii.String(params[ParamBusName]),
		},
	})
	tabela.GrantReadWriteData(fn)

	return awslambda.NewAlias(stack, jsii.String("Live"), &awslambda.AliasProps{
		AliasName:   jsii.String(AliasLive),
		Version:     fn.CurrentVersion(),
		Description: jsii.String("Versão em produção; o CodeDeploy faz o canary entre versões"),
	})
}

// AcoesCognitoAdmin são as operações do internal/contas.Cognito: bloquear
// (disable + global sign-out), reativar, trocar de grupo e excluir.
var AcoesCognitoAdmin = []string{
	"cognito-idp:AdminDisableUser",
	"cognito-idp:AdminEnableUser",
	"cognito-idp:AdminUserGlobalSignOut",
	"cognito-idp:AdminAddUserToGroup",
	"cognito-idp:AdminRemoveUserFromGroup",
	"cognito-idp:AdminDeleteUser",
}

// permissoesDeAdmin dá à função só o que as operações de admin usam: as
// AcoesCognitoAdmin no ARN do User Pool do ambiente (não userpool/*) e
// events:PutEvents no barramento, para publicar o UserDeleted.
func permissoesDeAdmin(alias awslambda.Alias, params Parametros) {
	alias.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:       jsii.String("CognitoAdminNoPool"),
		Actions:   jsii.Strings(AcoesCognitoAdmin...),
		Resources: jsii.Strings(params[ParamUserPoolArn]),
	}))
	barramento := awsevents.EventBus_FromEventBusName(alias.Stack(), jsii.String("BarramentoUserDeleted"), jsii.String(params[ParamBusName]))
	barramento.GrantPutEventsTo(alias, nil)
}

// novoCanary liga o alias ao CodeDeploy: cada versão nova recebe 10% do
// tráfego por 5 minutos e só depois os 100%. Volta sozinho para a versão
// anterior se o deploy falhar ou se o alarme de erros disparar.
//
// O alarme não tem ação (sem SNS) e fica dentro dos 10 alarmes gratuitos.
func novoCanary(stack awscdk.Stack, nome string, alias awslambda.Alias) awscodedeploy.LambdaDeploymentGroup {
	erros := awscloudwatch.NewAlarm(stack, jsii.String("ErrosNoAlias"), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(nome + "-erros"),
		AlarmDescription:   jsii.String("Erros no alias live do users-service; durante o canary, dispara o rollback"),
		Metric:             alias.MetricErrors(&awscloudwatch.MetricOptions{Period: awscdk.Duration_Minutes(jsii.Number(1))}),
		Threshold:          jsii.Number(1),
		EvaluationPeriods:  jsii.Number(1),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	return awscodedeploy.NewLambdaDeploymentGroup(stack, jsii.String("Deploy"), &awscodedeploy.LambdaDeploymentGroupProps{
		DeploymentGroupName: jsii.String(nome),
		Alias:               alias,
		DeploymentConfig:    awscodedeploy.LambdaDeploymentConfig_CANARY_10PERCENT_5MINUTES(),
		Alarms:              &[]interfacesawscloudwatch.IAlarmRef{erros},
		AutoRollback: &awscodedeploy.AutoRollbackConfig{
			FailedDeployment:  jsii.Bool(true),
			StoppedDeployment: jsii.Bool(true),
			DeploymentInAlarm: jsii.Bool(true),
		},
	})
}
