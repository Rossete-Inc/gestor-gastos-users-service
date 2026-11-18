package contract

// Notificações push (ADR 0011). O userId do envelope é a pessoa logada,
// nunca a conta ativa: cada pessoa liga as notificações nos próprios
// aparelhos e recebe os alertas de todas as contas de que participa.
const (
	// OpRegistrarAssinaturaPush grava a assinatura Web Push do aparelho
	// (RegistrarAssinaturaPushIn). Registrar o mesmo endpoint de novo só
	// atualiza as chaves. Dados inválidos: 422 "validacao" (endpoint,
	// p256dh, auth). Sem payload na resposta.
	OpRegistrarAssinaturaPush = "RegistrarAssinaturaPush"
	// OpRemoverAssinaturaPush apaga a assinatura do endpoint
	// (RemoverAssinaturaPushIn). Sem assinatura, não é erro. Endpoint
	// inválido: 422 "validacao" (endpoint). Sem payload na resposta.
	OpRemoverAssinaturaPush = "RemoverAssinaturaPush"
)

// RegistrarAssinaturaPushIn é a PushSubscription do navegador: o endpoint
// do serviço de push e as chaves p256dh e auth em base64url.
type RegistrarAssinaturaPushIn struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// RemoverAssinaturaPushIn é o payload de RemoverAssinaturaPush.
type RemoverAssinaturaPushIn struct {
	Endpoint string `json:"endpoint"`
}
