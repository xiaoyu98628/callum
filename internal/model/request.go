package model

type Request struct {
	// Model 是实际发送给远端接口的模型 ID。
	Model    string
	Messages []Message
}
