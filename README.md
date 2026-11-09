# gestor-gastos-users-service

Serviço de usuários do Gestor de Gastos (Go, Lambda, DynamoDB). Guarda o perfil de cada usuário e espelha papel e status da conta. O Cognito continua sendo a fonte da verdade do acesso (design.md, "Status da conta").

## Estrutura

| Pacote | O que faz |
| --- | --- |
| `contract` | Contrato público com o BFF: nomes das operações (`ObterPerfil`, `CriarPerfil`), papéis, status e formatos de entrada e saída. É o único pacote fora de `internal/` |
| `internal/perfil` | Domínio: validação do perfil, papéis (`cliente`, `admin`) e a máquina de estados do ciclo de vida da conta |
| `internal/app` | Casos de uso: obter perfil, criar perfil de forma idempotente e consumir `UserConfirmed` |
| `internal/store` | Store DynamoDB: `PK=USER#<id>`, `SK=PROFILE` e `GSI1` (`USERS`, `<criadoEm>#<id>`) para a listagem do admin |
| `internal/handler` | Liga os casos de uso ao `invoke.Router` e ao `events.Router` da go-commons, usados pela Lambda e pelo `cmd/local` |
| `cmd/local` | Servidor HTTP local que chama os mesmos handlers da Lambda (ver "Rodar localmente") |
| `internal/dynamotest` | Cria tabelas temporárias no DynamoDB Local para os testes de integração |

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

A esteira (`.github/workflows/ci.yml`) roda lint e os testes unitários e de integração em todo PR, com um DynamoDB Local como serviço. Ela reprova o pacote que ficar abaixo de 80% de cobertura.
