package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/invoke"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/app"
	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/memstore"
)

func montarLambda() LambdaFunc {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return Lambda(app.New(memstore.New(), log), log)
}

// chamar passa pela mesma serialização da AWS: payload em bytes na entrada e
// JSON na saída.
func chamar(t *testing.T, h LambdaFunc, payload string) (invoke.Response, error) {
	t.Helper()
	out, err := h(context.Background(), json.RawMessage(payload))
	if err != nil || out == nil {
		return invoke.Response{}, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var resp invoke.Response
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	return resp, nil
}

const eventoUserConfirmed = `{
	"version": "0", "id": "e-1", "detail-type": "UserConfirmed", "source": "gestor-gastos.auth",
	"account": "123456789012", "time": "2026-11-10T12:00:00Z", "region": "us-east-1", "resources": [],
	"detail": {"userId": "u-1", "email": "Ana@X.com", "nome": "Ana"}
}`

func TestLambdaEventoCriaPerfilEInvokeLe(t *testing.T) {
	h := montarLambda()

	out, err := h(context.Background(), json.RawMessage(eventoUserConfirmed))
	if err != nil || out != nil {
		t.Fatalf("evento: out=%v err=%v", out, err)
	}

	resp, err := chamar(t, h, `{"operation":"ObterPerfil","userId":"u-1"}`)
	if err != nil || resp.Error != nil {
		t.Fatalf("invoke: resp=%+v err=%v", resp, err)
	}
	var p contract.Perfil
	if err := json.Unmarshal(resp.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != "u-1" || p.Email != "ana@x.com" || p.Papel != contract.PapelCliente || p.Status != contract.StatusAtivo {
		t.Errorf("perfil = %+v", p)
	}
}

func TestLambdaInvokeDevolveErroDeDominioNoEnvelope(t *testing.T) {
	resp, err := chamar(t, montarLambda(), `{"operation":"ObterPerfil","userId":"nao-existe"}`)
	if err != nil {
		t.Fatalf("a função não deve falhar por erro de domínio: %v", err)
	}
	if resp.Error == nil || resp.Error.Status != 404 || resp.Error.Code != "perfil_nao_encontrado" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestLambdaEventoComDetailInvalidoFalha(t *testing.T) {
	// O erro precisa chegar à AWS para haver retentativa e DLQ.
	_, err := montarLambda()(context.Background(), json.RawMessage(
		`{"id":"e-2","detail-type":"UserConfirmed","source":"gestor-gastos.auth","time":"2026-11-10T12:00:00Z","detail":"texto"}`))
	if err == nil {
		t.Error("esperava erro")
	}
}

func TestLambdaEventoDeOutroTipoEIgnorado(t *testing.T) {
	out, err := montarLambda()(context.Background(), json.RawMessage(
		`{"id":"e-3","detail-type":"OutroEvento","source":"x","time":"2026-11-10T12:00:00Z","detail":{}}`))
	if err != nil || out != nil {
		t.Errorf("out=%v err=%v", out, err)
	}
}

func TestLambdaPayloadInvalido(t *testing.T) {
	casos := map[string]string{
		"não é JSON":          `nada`,
		"não é objeto":        `[1,2]`,
		"evento malformado":   `{"detail-type":"UserConfirmed","time":"ontem"}`,
		"envelope malformado": `{"operation":1}`,
	}
	for nome, payload := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := montarLambda()(context.Background(), json.RawMessage(payload)); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}
