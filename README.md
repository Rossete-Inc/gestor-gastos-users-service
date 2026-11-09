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

## Regras

- **Ciclo de vida:** só são permitidas as transições pendente → ativo, ativo ↔ bloqueado e ativo ou bloqueado → excluído. Qualquer outra, como reativar uma conta excluída, devolve 409 `transicao_invalida`.
- **Criação do perfil:** o perfil nasce `ATIVO`, depois da confirmação do e-mail, pelo evento `UserConfirmed` ou pelo primeiro `/me`. A criação é idempotente: chamar de novo devolve o perfil existente sem alterá-lo, e duas criações simultâneas gravam uma vez só.
- **Papel no cadastro público:** o evento `UserConfirmed` sempre cria o perfil com o papel `cliente`. O `/me` usa o papel dos grupos do Cognito, então o admin do ambiente local nasce `admin`.
- **Normalização:** o e-mail é gravado em minúsculas. O nome é gravado sem espaços repetidos e tem no máximo 100 caracteres.

## Testes

```sh
go test ./... -cover
```

Os testes de integração do store rodam contra o DynamoDB Local do `gestor-gastos-infra-base`. Cada teste cria e apaga uma tabela própria, sem tocar na `gestor-gastos-users`:

```sh
# no infra-base
task local:up
# aqui
go test -tags integration ./internal/store -v
```

A esteira (`.github/workflows/ci.yml`) roda lint e os testes unitários e de integração em todo PR, com um DynamoDB Local como serviço. Ela reprova o pacote que ficar abaixo de 80% de cobertura.
