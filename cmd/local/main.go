// Command local roda o users-service na máquina do desenvolvedor, contra o
// DynamoDB Local do infra-base (task local:up).
//
// É um servidor HTTP que chama os mesmos handlers da Lambda:
//
//	POST /invoke  envelope do invoke (chamadas do BFF)
//	POST /events  eventos (UserConfirmed)
//	GET  /healthz
//
// Escuta só em 127.0.0.1: o /invoke confia no userId do envelope, e na AWS
// quem garante isso é o IAM, que deixa só o BFF invocar a Lambda.
//
//	RUNTIME=local go run ./cmd/local
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/config"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/events"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/logging"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/handler"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/store"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const servico = "users-service"

// Config é a configuração do cmd/local.
type Config struct {
	Base           config.Base
	Port           string
	DynamoEndpoint string
	Table          string
}

// LoadConfig lê o ambiente. Só aceita RUNTIME=local e um DynamoDB local, para
// nunca gravar numa tabela da AWS por engano.
func LoadConfig(lookup config.LookupFunc) (Config, error) {
	base, err := config.LoadFrom(lookup, servico)
	if err != nil {
		return Config{}, err
	}
	if !base.IsLocal() {
		return Config{}, errors.New("cmd/local exige RUNTIME=local")
	}
	r := config.NewReader(lookup)
	c := Config{
		Base:           base,
		Port:           r.Get("PORT", "8082"),
		DynamoEndpoint: r.Get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
		Table:          r.Get("USERS_TABLE", store.TableName),
	}
	if err := exigirLocal(c.DynamoEndpoint); err != nil {
		return Config{}, err
	}
	return c, nil
}

// exigirLocal aceita só a própria máquina ou um container do compose, como o
// seed do infra-base.
func exigirLocal(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fmt.Errorf("DYNAMODB_ENDPOINT inválido: %q", endpoint)
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1", "dynamodb", "host.docker.internal":
		return nil
	}
	return fmt.Errorf("DYNAMODB_ENDPOINT %q não é local; o cmd/local só roda contra o ambiente local", endpoint)
}

// NewHandler monta as rotas sobre os mesmos routers usados pela Lambda.
func NewHandler(svc *app.Service, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(invoke.Path, handler.Invoke(svc, log).HTTPHandler())
	mux.Handle(events.Path, handler.Events(svc, log).HTTPHandler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func main() {
	if err := run(context.Background(), os.LookupEnv); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, lookup config.LookupFunc) error {
	cfg, err := LoadConfig(lookup)
	if err != nil {
		return err
	}
	log := logging.New(logging.Options{Service: servico, Env: cfg.Base.Env, Level: cfg.Base.LogLevel})

	db := dynamodb.New(dynamodb.Options{
		Region:       cfg.Base.Region,
		BaseEndpoint: aws.String(cfg.DynamoEndpoint),
		Credentials:  credentials.NewStaticCredentialsProvider("local", "local", ""),
	})
	svc := app.New(store.NewDynamo(db, cfg.Table), log)

	srv := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", cfg.Port),
		Handler:           NewHandler(svc, log),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	erro := make(chan error, 1)
	go func() { erro <- srv.ListenAndServe() }()
	log.Info("users-service local no ar", "addr", "http://"+srv.Addr, "dynamodb", cfg.DynamoEndpoint, "table", cfg.Table)

	select {
	case err := <-erro:
		return err
	case <-ctx.Done():
		log.Info("encerrando")
		// O http.Server só considera ociosa uma conexão aberta e ainda sem
		// requisição (StateNew) depois de 5 s. Com um prazo de 5 s, uma conexão
		// dessas, comum em clientes com keep-alive, estourava o prazo e deixava o
		// TestPontaAPonta intermitente no CI. O prazo agora é maior, e se mesmo
		// assim estourar as conexões são fechadas à força: é o ambiente local.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); errors.Is(err, context.DeadlineExceeded) {
			log.Warn("encerramento passou do prazo; fechando as conexões restantes")
			return srv.Close()
		} else if err != nil {
			return err
		}
		return nil
	}
}
