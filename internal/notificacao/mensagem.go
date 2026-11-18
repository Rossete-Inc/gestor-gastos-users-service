// Package notificacao monta e envia as notificações push dos alertas de
// meta (spec metas-gastos, "Notificação push da meta"; ADR 0011).
package notificacao

import (
	"fmt"
	"strconv"
	"strings"
)

// Alerta é um limiar (80 ou 100) que uma meta cruzou num mês.
type Alerta struct {
	NomeCategoria string
	Mes           string // AAAA-MM
	Limiar        int
	TotalCentavos int64
	MetaCentavos  int64
}

// Mensagem é o que o service worker mostra. URL é a rota aberta no toque;
// Tag agrupa as notificações de meta no aparelho.
type Mensagem struct {
	Titulo string `json:"titulo"`
	Corpo  string `json:"corpo"`
	URL    string `json:"url"`
	Tag    string `json:"tag"`
}

// Montar escreve uma única mensagem para todos os alertas de uma escrita
// (spec, "Um lançamento cruza limiares em vários meses"). mesCorrente
// (AAAA-MM) decide quando citar o mês.
func Montar(alertas []Alerta, mesCorrente string) Mensagem {
	m := Mensagem{Titulo: "Gestor de Gastos", URL: "/painel", Tag: "metas"}
	if len(alertas) == 1 {
		m.Corpo = frase(alertas[0], mesCorrente)
		return m
	}
	partes := make([]string, 0, len(alertas))
	for _, a := range alertas {
		partes = append(partes, frase(a, mesCorrente))
	}
	m.Corpo = fmt.Sprintf("%d metas atingidas. %s", len(alertas), strings.Join(partes, " "))
	return m
}

// frase descreve um alerta, por exemplo "Meta de Lazer: 80% usados
// (R$ 820,00 de R$ 1.000,00)." ou "Meta de Compras estourou em dezembro
// (R$ 600,00 de R$ 500,00).".
func frase(a Alerta, mesCorrente string) string {
	quando := ""
	if a.Mes != mesCorrente {
		quando = " em " + nomeDoMes(a.Mes, mesCorrente)
	}
	valores := fmt.Sprintf("(%s de %s)", Reais(a.TotalCentavos), Reais(a.MetaCentavos))
	if a.Limiar >= 100 {
		return fmt.Sprintf("Meta de %s estourou%s %s.", a.NomeCategoria, quando, valores)
	}
	return fmt.Sprintf("Meta de %s: %d%% usados%s %s.", a.NomeCategoria, a.Limiar, quando, valores)
}

var meses = [...]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}

// nomeDoMes devolve "dezembro", ou "janeiro de 2027" quando o ano é outro.
func nomeDoMes(mes, mesCorrente string) string {
	ano, num, ok := strings.Cut(mes, "-")
	n, err := strconv.Atoi(num)
	if !ok || err != nil || n < 1 || n > 12 {
		return mes
	}
	if strings.HasPrefix(mesCorrente, ano+"-") {
		return meses[n-1]
	}
	return meses[n-1] + " de " + ano
}

// Reais formata centavos como "R$ 1.234,56".
func Reais(centavos int64) string {
	sinal := ""
	if centavos < 0 {
		sinal, centavos = "-", -centavos
	}
	inteiro := strconv.FormatInt(centavos/100, 10)
	var b strings.Builder
	for i, c := range inteiro {
		if i > 0 && (len(inteiro)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return fmt.Sprintf("%sR$ %s,%02d", sinal, b.String(), centavos%100)
}
