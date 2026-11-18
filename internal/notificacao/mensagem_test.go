package notificacao

import "testing"

func TestReais(t *testing.T) {
	for centavos, want := range map[int64]string{
		0: "R$ 0,00", 5: "R$ 0,05", 82000: "R$ 820,00", 100000: "R$ 1.000,00",
		123456789: "R$ 1.234.567,89", -4590: "-R$ 45,90",
	} {
		if got := Reais(centavos); got != want {
			t.Errorf("Reais(%d) = %q, want %q", centavos, got, want)
		}
	}
}

func TestMontar(t *testing.T) {
	lazer80 := Alerta{NomeCategoria: "Lazer", Mes: "2026-11", Limiar: 80, TotalCentavos: 82000, MetaCentavos: 100000}
	casos := []struct {
		nome    string
		alertas []Alerta
		corpo   string
	}{
		// spec, "Notificação ao chegar a 80%"
		{"80% no mês corrente", []Alerta{lazer80}, "Meta de Lazer: 80% usados (R$ 820,00 de R$ 1.000,00)."},
		{"estourou", []Alerta{{NomeCategoria: "Lazer", Mes: "2026-11", Limiar: 100, TotalCentavos: 110000, MetaCentavos: 100000}},
			"Meta de Lazer estourou (R$ 1.100,00 de R$ 1.000,00)."},
		{"outro mês do ano", []Alerta{{NomeCategoria: "Lazer", Mes: "2026-12", Limiar: 80, TotalCentavos: 82000, MetaCentavos: 100000}},
			"Meta de Lazer: 80% usados em dezembro (R$ 820,00 de R$ 1.000,00)."},
		// spec, "Um lançamento cruza limiares em vários meses"
		{"vários meses numa mensagem", []Alerta{
			{NomeCategoria: "Compras", Mes: "2026-12", Limiar: 100, TotalCentavos: 50000, MetaCentavos: 50000},
			{NomeCategoria: "Compras", Mes: "2027-01", Limiar: 100, TotalCentavos: 60000, MetaCentavos: 50000},
		}, "2 metas atingidas. Meta de Compras estourou em dezembro (R$ 500,00 de R$ 500,00). Meta de Compras estourou em janeiro de 2027 (R$ 600,00 de R$ 500,00)."},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			m := Montar(c.alertas, "2026-11")
			if m.Corpo != c.corpo || m.Titulo != "Gestor de Gastos" || m.URL != "/painel" || m.Tag != "metas" {
				t.Fatalf("mensagem = %+v\nwant corpo %q", m, c.corpo)
			}
		})
	}
}
