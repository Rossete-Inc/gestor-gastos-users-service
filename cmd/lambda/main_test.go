package main

import (
	"context"
	"testing"

	"github.com/Rossete-Inc/gestor-gastos-users-service/internal/handler"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

var envValido = map[string]string{"RUNTIME": "aws", "ENV": "dev", "USERS_TABLE": "gestor-gastos-dev-users"}

func TestLoadConfig(t *testing.T) {
	c, err := LoadConfig(env(envValido))
	if err != nil {
		t.Fatal(err)
	}
	if c.Table != "gestor-gastos-dev-users" || c.Base.Env != "dev" || c.Base.Region != "us-east-1" {
		t.Errorf("config = %+v", c)
	}
}

func TestLoadConfigAdmin(t *testing.T) {
	m := map[string]string{"USER_POOL_ID": "us-east-1_abc", "EVENT_BUS_NAME": "gestor-gastos-dev"}
	for k, v := range envValido {
		m[k] = v
	}
	c, err := LoadConfig(env(m))
	if err != nil || !c.Admin() || c.UserPoolID != "us-east-1_abc" || c.BusName != "gestor-gastos-dev" {
		t.Fatalf("config = %+v, %v", c, err)
	}
	if c, _ := LoadConfig(env(envValido)); c.Admin() {
		t.Error("admin ligado sem USER_POOL_ID")
	}
	var recebido any
	if codigo := executar(context.Background(), env(m), func(h any) { recebido = h }); codigo != 0 || recebido == nil {
		t.Errorf("executar com admin: código %d", codigo)
	}
}

func TestLoadConfigRecusa(t *testing.T) {
	casos := map[string]map[string]string{
		"só USER_POOL_ID":   {"RUNTIME": "aws", "ENV": "dev", "USERS_TABLE": "t", "USER_POOL_ID": "p"},
		"só EVENT_BUS_NAME": {"RUNTIME": "aws", "ENV": "dev", "USERS_TABLE": "t", "EVENT_BUS_NAME": "b"},
		"sem RUNTIME":       {"ENV": "dev", "USERS_TABLE": "t"},
		"RUNTIME=local":     {"RUNTIME": "local", "USERS_TABLE": "t"},
		"sem USERS_TABLE":   {"RUNTIME": "aws", "ENV": "dev"},
		"sem ENV":           {"RUNTIME": "aws", "USERS_TABLE": "t"},
	}
	for nome, m := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := LoadConfig(env(m)); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}

func TestExecutarEntregaOHandler(t *testing.T) {
	var recebido any
	if codigo := executar(context.Background(), env(envValido), func(h any) { recebido = h }); codigo != 0 {
		t.Fatalf("código = %d", codigo)
	}
	if _, ok := recebido.(handler.LambdaFunc); !ok {
		t.Errorf("handler = %T", recebido)
	}
}

func TestExecutarFalhaSemConfig(t *testing.T) {
	chamou := false
	if codigo := executar(context.Background(), env(map[string]string{}), func(any) { chamou = true }); codigo != 1 || chamou {
		t.Errorf("código = %d, iniciou = %v", codigo, chamou)
	}
}
