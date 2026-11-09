//go:build integration

// Testes contra o DynamoDB Local do infra-base (task local:up). Cada teste
// cria uma tabela própria, com o mesmo esquema do seed, e a apaga no fim,
// sem tocar na tabela gestor-gastos-users usada no ambiente local.
//
//	go test -tags integration ./internal/store
package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/dynamotest"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/perfil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func tabelaDeTeste(t *testing.T) (*Dynamo, *dynamodb.Client) {
	t.Helper()
	c, nome := dynamotest.UsersTable(t, TableName)
	return NewDynamo(c, nome), c
}

var base = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func novo(t *testing.T, id string, criado time.Time) perfil.Perfil {
	t.Helper()
	p, err := perfil.Novo(id, id+"@x.com", "Nome "+id, perfil.Cliente, criado)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCriarEObter(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	p := novo(t, "u-1", base.Add(123*time.Millisecond))

	gravado, criado, err := d.Criar(ctx, p)
	if err != nil || !criado || gravado != p {
		t.Fatalf("Criar = %+v, %v, %v", gravado, criado, err)
	}
	obtido, err := d.Obter(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if obtido != p {
		t.Errorf("obtido = %+v\nquer   %+v", obtido, p)
	}
}

func TestChavesDoContrato(t *testing.T) {
	// As chaves são o contrato com o CDK e o seed (docs/local.md, "Tabelas").
	ctx := context.Background()
	d, c := tabelaDeTeste(t)
	p := novo(t, "u-1", base)
	if _, _, err := d.Criar(ctx, p); err != nil {
		t.Fatal(err)
	}
	out, err := c.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key:       map[string]types.AttributeValue{"PK": s("USER#u-1"), "SK": s("PROFILE")},
	})
	if err != nil || len(out.Item) == 0 {
		t.Fatalf("item não encontrado nas chaves do contrato: %v", err)
	}
	quer := map[string]string{"GSI1PK": "USERS", "GSI1SK": "2026-11-09T12:00:00.000Z#u-1"}
	for k, v := range quer {
		if got := out.Item[k].(*types.AttributeValueMemberS).Value; got != v {
			t.Errorf("%s = %q, quer %q", k, got, v)
		}
	}
}

func TestCriarNaoSobrescreve(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	original := novo(t, "u-1", base)
	if _, _, err := d.Criar(ctx, original); err != nil {
		t.Fatal(err)
	}
	outro, _ := perfil.Novo("u-1", "outro@x.com", "Outro", perfil.Admin, base.Add(time.Hour))
	gravado, criado, err := d.Criar(ctx, outro)
	if err != nil || criado {
		t.Fatalf("criado = %v, err = %v", criado, err)
	}
	if gravado != original {
		t.Errorf("devolveu %+v, quer o original %+v", gravado, original)
	}
	if atual, _ := d.Obter(ctx, "u-1"); atual != original {
		t.Errorf("perfil foi sobrescrito: %+v", atual)
	}
}

func TestCriarConcorrente(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	var criados atomic.Int32
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			p, _ := perfil.Novo("u-1", fmt.Sprintf("u%d@x.com", i), "Ana", perfil.Cliente, base)
			_, criado, err := d.Criar(ctx, p)
			if err != nil {
				t.Error(err)
			}
			if criado {
				criados.Add(1)
			}
		})
	}
	wg.Wait()
	if criados.Load() != 1 {
		t.Errorf("criados = %d, quer 1", criados.Load())
	}
}

func TestObterInexistente(t *testing.T) {
	d, _ := tabelaDeTeste(t)
	_, err := d.Obter(context.Background(), "nao-existe")
	if !errors.Is(err, errs.NotFound("perfil_nao_encontrado", "")) {
		t.Errorf("erro = %v", err)
	}
}

func TestListarPaginado(t *testing.T) {
	ctx := context.Background()
	d, _ := tabelaDeTeste(t)
	// 5 perfis criados com 1 minuto de diferença: u-0 é o mais antigo.
	for i := range 5 {
		if _, _, err := d.Criar(ctx, novo(t, fmt.Sprintf("u-%d", i), base.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}

	codec, _ := page.NewCodec([]byte("0123456789abcdef0123456789abcdef"))
	var ids []string
	var cursor string
	for paginas := 0; ; paginas++ {
		if paginas > 5 {
			t.Fatal("paginação não terminou")
		}
		inicio, err := codec.Decode("USERS", cursor)
		if err != nil {
			t.Fatal(err)
		}
		perfis, proxima, err := d.Listar(ctx, 2, inicio)
		if err != nil {
			t.Fatal(err)
		}
		if len(perfis) > 2 {
			t.Fatalf("página com %d itens", len(perfis))
		}
		for _, p := range perfis {
			ids = append(ids, p.ID)
		}
		// O cursor passa pelo codec, como fará o BFF.
		if cursor, err = codec.Encode("USERS", proxima); err != nil {
			t.Fatal(err)
		}
		if cursor == "" {
			break
		}
	}
	if fmt.Sprint(ids) != "[u-4 u-3 u-2 u-1 u-0]" {
		t.Errorf("ordem = %v, quer do mais novo para o mais antigo", ids)
	}
}

func TestListarVazio(t *testing.T) {
	d, _ := tabelaDeTeste(t)
	perfis, proxima, err := d.Listar(context.Background(), 0, nil)
	if err != nil || len(perfis) != 0 || proxima != nil {
		t.Errorf("Listar = %v, %v, %v", perfis, proxima, err)
	}
}
