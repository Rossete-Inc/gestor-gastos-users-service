# gestor-gastos-users-service

Serviço de usuários do Gestor de Gastos (Go, Lambda, DynamoDB). Guarda o perfil de cada usuário e espelha papel e status da conta. O Cognito continua sendo a fonte da verdade do acesso (design.md, "Status da conta").

## Estrutura

| Pacote | O que faz |
| --- | --- |
| `contract` | Contrato público com o BFF: nomes das operações (`ObterPerfil`, `CriarPerfil`), papéis, status e formatos de entrada e saída. É o único pacote fora de `internal/` |
| `internal/perfil` | Domínio: validação do perfil, papéis (`cliente`, `admin`) e a máquina de estados do ciclo de vida da conta |
| `internal/app` | Casos de uso: obter perfil, criar perfil de forma idempotente e consumir `UserConfirmed` |
| `internal/store` | Store DynamoDB: `PK=USER#<id>`, `SK=PROFILE` e `GSI1` (`USERS`, `<criadoEm>#<id>`) para a listagem do admin |
| `internal/handler` | Liga os casos de uso ao `invoke.Router` e ao `events.Router` da go-commons, usados pela Lambda e pelo `cmd/local`. `handler.Lambda` junta os dois numa função só |
| `cmd/lambda` | Entrada da Lambda na AWS (`RUNTIME=aws`), compilada em `dist/lambda/bootstrap` |
| `cmd/local` | Servidor HTTP local que chama os mesmos handlers da Lambda (ver "Rodar localmente") |
| `internal/dynamotest` | Cria tabelas temporárias no DynamoDB Local para os testes de integração |
| `infra` | App AWS CDK em Go (módulo próprio) com a stack `gestor-gastos-dev-users` (ver "Na AWS") |
| `scripts/smoke.sh` | Smoke test rodado pela esteira depois do deploy |

## Regras

- **Ciclo de vida:** só são permitidas as transições pendente → ativo, ativo ↔ bloqueado e ativo ou bloqueado → excluído. Qualquer outra, como reativar uma conta excluída, devolve 409 `transicao_invalida`.
- **Criação do perfil:** o perfil nasce `ATIVO`, depois da confirmação do e-mail, pelo evento `UserConfirmed` ou pelo primeiro `/me`. A criação é idempotente: chamar de novo devolve o perfil existente sem alterá-lo, e duas criações simultâneas gravam uma vez só.
- **Papel no cadastro público:** o evento `UserConfirmed` sempre cria o perfil com o papel `cliente`. O `/me` usa o papel dos grupos do Cognito, então o admin do ambiente local nasce `admin`.
- **Normalização:** o e-mail é gravado em minúsculas. O nome é gravado sem espaços repetidos e tem no máximo 100 caracteres.

## Rodar localmente

Com o ambiente local do infra-base no ar (`task local:up`):

```sh
RUNTIME=local go run ./cmd/local
```

O servidor sobe em http://127.0.0.1:8082 e chama os mesmos handlers da Lambda:

| Rota | Uso |
| --- | --- |
| `POST /invoke` | Envelope do `invoke`, chamado pelo BFF |
| `POST /events` | Eventos, como `UserConfirmed` |
| `GET /healthz` | Responde `ok` |

| Variável | Padrão |
| --- | --- |
| `RUNTIME` | obrigatória; só aceita `local` |
| `PORT` | `8082` |
| `DYNAMODB_ENDPOINT` | `http://localhost:8000`; endpoints fora da máquina são recusados |
| `USERS_TABLE` | `gestor-gastos-users` |
| `LOG_LEVEL` | `info` |

**Segurança local:** o `/invoke` confia no `userId` do envelope. Na AWS, só o BFF consegue invocar a Lambda, por IAM. Localmente, o servidor escuta **só em 127.0.0.1**.

Exemplos com `curl`:

```sh
# criar o perfil (o BFF faz isso no /me) e consultar
curl -s -X POST localhost:8082/invoke -H 'Content-Type: application/json' \
  -d '{"operation":"CriarPerfil","userId":"u-1","payload":{"email":"admin@local.test","nome":"Admin Local","papel":"admin"}}'
curl -s -X POST localhost:8082/invoke -H 'Content-Type: application/json' \
  -d '{"operation":"ObterPerfil","userId":"u-1"}'

# evento de conta confirmada: cria o perfil como cliente e responde 204
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8082/events -H 'Content-Type: application/json' \
  -d '{"id":"e1","detail-type":"UserConfirmed","source":"curl","time":"2026-11-09T18:30:00Z","detail":{"userId":"u-2","email":"cliente@local.test","nome":"Cliente Local"}}'
```

Erros chegam dentro do envelope como problem+json, por exemplo `{"error":{"status":404,"code":"perfil_nao_encontrado",...}}`.

## Testes

```sh
go test ./... -cover
```

Os testes de integração (store e `cmd/local` de ponta a ponta) rodam contra o DynamoDB Local do `gestor-gastos-infra-base`. Cada teste cria e apaga uma tabela própria, sem tocar na `gestor-gastos-users`:

```sh
# no infra-base
task local:up
# aqui
go test -tags integration ./... -v
```

Os testes de assertion do CDK ficam no módulo `infra/` e não precisam de AWS nem do binário da Lambda:

```sh
cd infra && go vet ./... && go test ./...
```

## Esteira

`.github/workflows/ci.yml` chama o workflow reutilizável `go-service.yml` do `gestor-gastos-infra-base` (docs/workflows.md de lá):

| Evento | O que roda |
| --- | --- |
| `pull_request` | Validado localmente, `go mod tidy`, `gofmt`, `go vet`, golangci-lint e testes unitários e de integração (DynamoDB Local), com no mínimo 80% de cobertura por pacote; build arm64 de `./cmd/lambda`; `cdk synth` e `cdk diff` contra dev, comentado no PR (role só leitura `gh-diff-gestor-gastos-users-service`) |
| `push` na `main` | Qualidade, build, **`cdk deploy` em dev** (role `gh-deploy-gestor-gastos-users-service-dev`, Environment `dev`) e `scripts/smoke.sh` |

O job `infra` do mesmo arquivo roda `go vet` e os testes do CDK, porque o `go test ./...` da raiz não entra no módulo `infra/`.

## Na AWS

Stack `gestor-gastos-dev-users` (`infra/`), implantada pela esteira a cada merge na `main`. Há um único ambiente na AWS, `dev` (ADR 0007 do infra-base).

| Recurso | Configuração |
| --- | --- |
| Tabela `gestor-gastos-dev-users` | DynamoDB on-demand, `PK`/`SK` e `GSI1` (`GSI1PK`/`GSI1SK`, projeção ALL), como `internal/store`. `RemovalPolicy: RETAIN`: apagar a stack não apaga os perfis |
| Lambda `gestor-gastos-dev-users` | `provided.al2023`, arm64, 256 MB, timeout de 10 s, sem VPC. Variáveis `RUNTIME=aws`, `ENV=dev`, `USERS_TABLE` e `LOG_LEVEL`. Permissão de leitura e escrita só na tabela |
| Alias `live` | O que o BFF invoca e a regra do EventBridge aciona |
| CodeDeploy | Canary `LambdaCanary10Percent5Minutes`: a versão nova recebe 10% do tráfego do `live` por 5 minutos e depois 100%. Rollback automático se o deploy falhar ou se o alarme `gestor-gastos-dev-users-erros` (erros no alias ≥ 1 em 1 minuto) disparar |
| Regra `gestor-gastos-dev-users-user-confirmed` | No barramento `gestor-gastos-dev`, filtra `source=gestor-gastos.auth` e `detail-type=UserConfirmed`. Target: o alias `live`, com até 8 retentativas de entrega em 2 horas e a DLQ do barramento |
| Falha da função no evento | Invocação assíncrona: a Lambda tenta mais 2 vezes (até 1 hora) e manda o registro para a mesma DLQ (destino `OnFailure`). A DLQ do target só cobre falhas de entrega, não erros da função. O consumidor é idempotente |
| Logs | `/aws/lambda/gestor-gastos-dev-users`, retenção de 7 dias |

Custo parado perto de zero: on-demand, sem provisioned concurrency, sem VPC, um alarme (dentro dos 10 gratuitos) e parâmetros Standard no SSM.

**Uma função para os dois formatos.** `handler.Lambda` olha o campo `detail-type`: se existe, é evento do EventBridge (o erro volta para a AWS tentar de novo); se não, é o envelope do invoke (erros de domínio voltam dentro do envelope e a função não falha). Uma função só mantém um canary, um alias e um alarme, e o cold start é compartilhado.

### Parâmetros no SSM

| Parâmetro | Direção | Valor | Quem usa |
| --- | --- | --- | --- |
| `/gestor-gastos/dev/events/bus-name` | lido | `gestor-gastos-dev` | regra do EventBridge |
| `/gestor-gastos/dev/events/dlq-arn` | lido | ARN da DLQ do barramento | DLQ do target e destino `OnFailure` |
| `/gestor-gastos/dev/users/function-arn` | publicado | ARN do alias `live` (`arn:aws:lambda:us-east-1:<conta>:function:gestor-gastos-dev-users:live`) | BFF (`lambda:InvokeFunction`) |
| `/gestor-gastos/dev/users/table-name` | publicado | `gestor-gastos-dev-users` | inspeção e, mais tarde, ferramentas de admin |

**Ordem de deploy:** infra-base (stack `events`) → users-service → BFF. O BFF lê `users/function-arn`, então o users-service precisa ter sido implantado antes.

**Falha clara quando falta parâmetro.** Na esteira (variável `AMBIENTE`, definida pelo `go-service.yml`), o app CDK lê os parâmetros do infra-base no synth, com as credenciais do job, e grava os valores no template. Se faltar algum, o `cdk synth`, o `cdk diff` e o `cdk deploy` param antes de tocar na conta, com uma mensagem que lista os ausentes e quem os publica:

```text
erro: faltam parâmetros no SSM (us-east-1), necessários para a stack gestor-gastos-dev-users:
  - /gestor-gastos/dev/events/dlq-arn (publicado pela stack gestor-gastos-dev-events do gestor-gastos-infra-base)
Implante o gestor-gastos-infra-base antes do users-service (...)
```

A leitura é feita no synth, e não por um parâmetro do CloudFormation (`valueForStringParameter`), porque a role de deploy do bootstrap do CDK só pode ler o parâmetro de versão do bootstrap: o `CreateChangeSet` falharia com `AccessDenied` em `ssm:GetParameters`. Fora da esteira (sem `AMBIENTE`), o synth roda offline e os valores viram parâmetros do CloudFormation. O contexto `lerSsm` força um modo ou outro (`-c lerSsm=true` ou `-c lerSsm=false`).

### Comandos

Pela raiz, para gerar o template sem credenciais:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -ldflags="-s -w" -o dist/lambda/bootstrap ./cmd/lambda
cd infra
npx --yes aws-cdk@2.1148.0 synth --context env=dev   # template em infra/cdk.out
```

Com credenciais de leitura da conta, para ver o mesmo diff da esteira:

```sh
AMBIENTE=dev npx --yes aws-cdk@2.1148.0 diff --method=template --context env=dev
```

O deploy é só pela esteira. Sem o binário em `dist/lambda/bootstrap`, o synth falha com uma mensagem dizendo como compilar.

### Smoke test

`scripts/smoke.sh` roda depois do deploy com a role de deploy. Ela não pode invocar a Lambda (só o BFF pode, porque o invoke confia no `userId` do envelope), então o script confere os parâmetros publicados: `users/function-arn` precisa apontar para o alias `live` e `users/table-name` para a tabela do ambiente. Que a versão nova responde já foi verificado pelo canary. O teste de ponta a ponta, `GET /me` com token real, fica na esteira do BFF.
