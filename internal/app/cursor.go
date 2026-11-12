package app

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
	"github.com/Rossete-Inc/gestor-gastos-go-commons/page"
	"github.com/Rossete-Inc/gestor-gastos-users-service/contract"
)

// O cursor da listagem do admin é a chave de continuação do GSI1 em JSON e
// base64url, sem assinatura: a listagem é a mesma para qualquer admin e não
// tem filtro, então um cursor adulterado só muda a posição numa lista que o
// admin já pode ver inteira. Mesmo assim, a chave precisa ter exatamente o
// formato do GSI1, para nunca chegar ao DynamoDB nada além disso.

// maxCursor limita o tamanho aceito, como o Cursor do openapi do BFF.
const maxCursor = 2048

var gsi1sk = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z#[^#\s]{1,128}$`)

func cursorInvalido() error {
	return errs.BadRequest(contract.CodigoCursorInvalido, "Cursor de paginação inválido. Volte para a primeira página.")
}

func codificarCursor(k page.Key) string {
	if len(k) == 0 {
		return ""
	}
	b, _ := json.Marshal(k) // map[string]string sempre serializa
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodificarCursor(c string) (page.Key, error) {
	if c == "" {
		return nil, nil
	}
	if len(c) > maxCursor {
		return nil, cursorInvalido()
	}
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, cursorInvalido()
	}
	var k page.Key
	if err := json.Unmarshal(b, &k); err != nil || len(k) != 4 {
		return nil, cursorInvalido()
	}
	sk := k["GSI1SK"]
	id, _, _ := strings.Cut(strings.TrimPrefix(k["PK"], "USER#"), "#")
	if k["GSI1PK"] != "USERS" || k["SK"] != "PROFILE" || !gsi1sk.MatchString(sk) ||
		!strings.HasPrefix(k["PK"], "USER#") || !strings.HasSuffix(sk, "#"+id) {
		return nil, cursorInvalido()
	}
	return k, nil
}
