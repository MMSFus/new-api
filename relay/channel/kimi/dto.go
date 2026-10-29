package kimi

// Kimi 网页版会话接口的 DTO。
//
// 创建会话：POST {site}/api/chat
//
//	{"name":"...", "born_from":"home", "kimiplus_id":"kimi", "is_example":false, "source":"web"}
//	→ {"id": "<chat_id>", ...}
//
// 补全（SSE）：POST {site}/api/chat/{chat_id}/completion/stream
//
//	{"messages":[{"role":"user","content":"..."}], "refs":[]}
//	→ data: {"event":"cmpl","text":"...",...} ... data: {"event":"all_done"}

type CreateChatRequest struct {
	Name         string `json:"name"`
	BornFrom     string `json:"born_from"`
	KimiplusID   string `json:"kimiplus_id"`
	IsExample    bool   `json:"is_example"`
	Source       string `json:"source"`
}

type CreateChatResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Messages []ChatMessage `json:"messages"`
	Refs     []any         `json:"refs"`
}

// StreamEvent 是 completion/stream SSE 的事件载荷。文本增量在
// event=="cmpl" 事件的 text 字段；流结束以 event=="all_done" 标志。
type StreamEvent struct {
	Event string `json:"event"`
	Text  string `json:"text"`
	View  string `json:"view"`
}
