package models

type NatsRequestPayload struct {
	Data []byte `json:"data"`
}

type NatsResponsePayload struct {
	Status  string `json:"status"`  // "success" 或 "error"
	Message string `json:"message"` // 成功或错误信息
}
