package kimi

const ChannelName = "kimi"

// ModelList 为 Kimi 网页版会话反代渠道暴露的模型标识。网页端不区分模型 ID，
// 统一由账号当前选用的模型应答，因此仅暴露占位模型 "kimi"。
var ModelList = []string{
	"kimi",
}
