package tool

// ArgumentError 提供由可信工具生成的字段说明，不包含模型传入的字段值。
// 它仍匹配 ErrInvalid，供执行器在未执行操作时向模型反馈纠正原因。
type ArgumentError struct {
	Field   string
	Message string
}

func (e *ArgumentError) Error() string { return e.Field + ": " + e.Message }
func (*ArgumentError) Unwrap() error   { return ErrInvalid }
