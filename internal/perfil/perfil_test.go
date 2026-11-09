package perfil

import (
	"strings"
	"testing"
	"time"

	"github.com/Rossete-Inc/gestor-gastos-go-commons/errs"
)

var agora = time.Date(2026, 11, 9, 15, 4, 5, 0, time.FixedZone("BRT", -3*3600))

func TestNovoNormaliza(t *testing.T) {
	p, err := Novo(" sub-1 ", "  Ana@Exemplo.COM ", "  Ana   Souza ", Cliente, agora)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "sub-1" || p.Email != "ana@exemplo.com" || p.Nome != "Ana Souza" {
		t.Errorf("perfil = %+v", p)
	}
	if p.Status != Ativo || p.Papel != Cliente {
		t.Errorf("status/papel = %s/%s", p.Status, p.Papel)
	}
	if !p.CriadoEm.Equal(agora) || p.CriadoEm.Location() != time.UTC || !p.AtualizadoEm.Equal(p.CriadoEm) {
		t.Errorf("datas = %v / %v", p.CriadoEm, p.AtualizadoEm)
	}
}

func TestNovoValida(t *testing.T) {
	casos := []struct {
		nome               string
		id, email, nomeUsu string
		papel              Papel
		campos             []string
	}{
		{"tudo inválido", "", "", "", "root", []string{"id", "email", "nome", "papel"}},
		{"e-mail sem arroba", "u", "ana.exemplo.com", "Ana", Cliente, []string{"email"}},
		{"e-mail com nome", "u", "Ana <ana@x.com>", "Ana", Cliente, []string{"email"}},
		{"e-mail longo", "u", strings.Repeat("a", 250) + "@x.com", "Ana", Cliente, []string{"email"}},
		{"nome longo", "u", "a@x.com", strings.Repeat("é", MaxNome+1), Cliente, []string{"nome"}},
		{"nome só espaços", "u", "a@x.com", "   ", Cliente, []string{"nome"}},
		{"nome com controle", "u", "a@x.com", "Ana\u0000", Admin, []string{"nome"}},
	}
	for _, tt := range casos {
		t.Run(tt.nome, func(t *testing.T) {
			_, err := Novo(tt.id, tt.email, tt.nomeUsu, tt.papel, agora)
			e := errs.As(err)
			if e == nil || e.Kind != errs.KindValidation {
				t.Fatalf("erro = %v", err)
			}
			var got []string
			for _, f := range e.Fields {
				got = append(got, f.Field)
			}
			if strings.Join(got, ",") != strings.Join(tt.campos, ",") {
				t.Errorf("campos = %v, quer %v", got, tt.campos)
			}
		})
	}

	t.Run("nome no limite", func(t *testing.T) {
		if _, err := Novo("u", "a@x.com", strings.Repeat("é", MaxNome), Admin, agora); err != nil {
			t.Error(err)
		}
	})
}

func TestMaquinaDeEstados(t *testing.T) {
	// Matriz completa da spec gestao-usuarios, "Ciclo de vida da conta".
	permitidas := map[[2]Status]bool{
		{PendenteConfirmacao, Ativo}: true,
		{Ativo, Bloqueado}:           true,
		{Bloqueado, Ativo}:           true,
		{Ativo, Excluido}:            true,
		{Bloqueado, Excluido}:        true,
	}
	todos := []Status{PendenteConfirmacao, Ativo, Bloqueado, Excluido}
	for _, de := range todos {
		for _, para := range todos {
			quer := permitidas[[2]Status{de, para}]
			if got := de.PodeIrPara(para); got != quer {
				t.Errorf("%s → %s = %v, quer %v", de, para, got, quer)
			}
		}
	}
}

func TestTransicoes(t *testing.T) {
	depois := agora.Add(time.Hour)
	p, _ := Novo("u", "a@x.com", "Ana", Cliente, agora)

	if err := p.Bloquear(depois); err != nil || p.Status != Bloqueado || !p.AtualizadoEm.Equal(depois) {
		t.Fatalf("bloquear: %v, %+v", err, p)
	}
	if err := p.Reativar(depois); err != nil || p.Status != Ativo {
		t.Fatalf("reativar: %v, %+v", err, p)
	}
	if err := p.Excluir(depois); err != nil || p.Status != Excluido {
		t.Fatalf("excluir: %v, %+v", err, p)
	}

	// Cenário "Transição inválida": reativar uma conta EXCLUIDO é recusado.
	err := p.Reativar(depois.Add(time.Hour))
	if e := errs.As(err); e == nil || e.Kind != errs.KindConflict || e.Code != "transicao_invalida" {
		t.Fatalf("erro = %v", err)
	}
	if p.Status != Excluido || !p.AtualizadoEm.Equal(depois) {
		t.Errorf("transição recusada alterou o perfil: %+v", p)
	}
}

func TestPapelDosGrupos(t *testing.T) {
	casos := map[string]struct {
		grupos []string
		quer   Papel
	}{
		"admin":        {[]string{"admin"}, Admin},
		"os dois":      {[]string{"cliente", "admin"}, Admin},
		"cliente":      {[]string{"cliente"}, Cliente},
		"sem grupo":    {nil, Cliente},
		"desconhecido": {[]string{"Admin", "root"}, Cliente},
	}
	for nome, c := range casos {
		if got := PapelDosGrupos(c.grupos); got != c.quer {
			t.Errorf("%s: %s, quer %s", nome, got, c.quer)
		}
	}
}

func TestContract(t *testing.T) {
	p, _ := Novo("u", "a@x.com", "Ana", Admin, agora)
	c := p.Contract()
	if c.ID != "u" || c.Email != "a@x.com" || c.Nome != "Ana" || c.Papel != "admin" || c.Status != "ATIVO" || !c.CriadoEm.Equal(agora) {
		t.Errorf("contract = %+v", c)
	}
}
