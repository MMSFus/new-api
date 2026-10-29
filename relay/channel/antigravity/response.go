package antigravity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// responseStream removes only the Cloud Code envelope. It passes usage-only,
// tool-call and signature-only events through to the existing Gemini handlers.
// Reading is demand-driven and Close closes the original response, so client
// cancellation and backpressure do not leave a producer goroutine behind.
type responseStream struct {
	source  io.ReadCloser
	scanner *bufio.Scanner
	pending *bytes.Reader
	done    bool
}

func newResponseStream(source io.ReadCloser) *responseStream {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	return &responseStream{source: source, scanner: scanner}
}
func (s *responseStream) Close() error { return s.source.Close() }
func (s *responseStream) Read(p []byte) (int, error) {
	for {
		if s.pending != nil {
			n, err := s.pending.Read(p)
			if n > 0 {
				return n, nil
			}
			if err != io.EOF {
				return n, err
			}
			s.pending = nil
		}
		if s.done {
			return 0, io.EOF
		}
		data, err := s.event()
		if err != nil {
			return 0, err
		}
		if data == nil {
			s.done = true
			return 0, io.EOF
		}
		events, err := streamEvents(data)
		if err != nil {
			return 0, err
		}
		s.pending = bytes.NewReader(events)
	}
}
func (s *responseStream) event() ([]byte, error) {
	for {
		var data []string
		size := 0
		for s.scanner.Scan() {
			line := s.scanner.Text()
			if line == "" {
				if len(data) > 0 {
					break
				}
				continue
			}
			if val, ok := strings.CutPrefix(line, "data:"); ok {
				size += len(val) + 1
				if size > 8<<20 {
					return nil, errors.New("Antigravity stream event exceeds size limit")
				}
				data = append(data, strings.TrimPrefix(val, " "))
			}
		}
		if err := s.scanner.Err(); err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, nil
		}
		raw := strings.Join(data, "\n")
		if raw == "[DONE]" {
			return nil, nil
		}
		var envelope map[string]json.RawMessage
		if common.UnmarshalJsonStr(raw, &envelope) != nil {
			return nil, errors.New("Invalid Antigravity stream event")
		}
		if _, ok := envelope["error"]; ok {
			return nil, errors.New("Antigravity stream returned an error")
		}
		payload, ok := envelope["response"]
		if !ok {
			return nil, errors.New("Antigravity stream event is missing response")
		}
		var response map[string]json.RawMessage
		if common.Unmarshal(payload, &response) != nil || response == nil {
			return nil, errors.New("Invalid Antigravity response")
		}
		if _, ok := response["error"]; ok {
			return nil, errors.New("Antigravity stream returned an error")
		}
		return payload, nil
	}
}

func aggregateResponse(s *responseStream) ([]byte, error) {
	result := map[string]any{}
	candidates := map[int]map[string]any{}
	total := 0
	events := 0
	var usage *dto.GeminiUsageMetadata
	for {
		raw, err := s.event()
		if err != nil {
			return nil, err
		}
		if raw == nil {
			break
		}
		total += len(raw)
		events++
		if total > 32<<20 {
			return nil, errors.New("Antigravity response exceeds size limit")
		}
		var chunk map[string]any
		if common.Unmarshal(raw, &chunk) != nil {
			return nil, errors.New("Invalid Antigravity response")
		}
		if rawUsage, ok := chunk["usageMetadata"]; ok {
			encoded, err := common.Marshal(rawUsage)
			if err != nil {
				return nil, err
			}
			var metadata dto.GeminiUsageMetadata
			if common.Unmarshal(encoded, &metadata) != nil {
				return nil, errors.New("Invalid Antigravity usage metadata")
			}
			usage = dto.MergeGeminiUsageMetadataNonZero(usage, &metadata)
		}
		// usageMetadata contains cumulative counts, never per-event increments.
		for k, v := range chunk {
			if k != "candidates" && k != "usageMetadata" {
				result[k] = v
			}
		}
		items, _ := chunk["candidates"].([]any)
		for _, item := range items {
			candidate, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("Invalid Antigravity candidate")
			}
			index, _ := candidate["index"].(float64)
			if index < 0 || index > 128 || index != float64(int(index)) {
				return nil, errors.New("Invalid Antigravity candidate index")
			}
			id := int(index)
			previous := candidates[id]
			if previous == nil {
				candidates[id] = candidate
				continue
			}
			content, _ := candidate["content"].(map[string]any)
			oldContent, _ := previous["content"].(map[string]any)
			if content != nil {
				if oldContent == nil {
					oldContent = map[string]any{}
					previous["content"] = oldContent
				}
				parts, _ := oldContent["parts"].([]any)
				next, _ := content["parts"].([]any)
				// A trailing signature belongs to the preceding content part.
				for _, p := range next {
					part, ok := p.(map[string]any)
					if ok && len(part) == 1 && part["thoughtSignature"] != nil && len(parts) > 0 {
						if last, ok := parts[len(parts)-1].(map[string]any); ok {
							last["thoughtSignature"] = part["thoughtSignature"]
							continue
						}
					}
					if ok && part["text"] != nil && len(parts) > 0 {
						previous, valid := parts[len(parts)-1].(map[string]any)
						if valid && previous["text"] != nil && previous["thought"] == part["thought"] && previous["thoughtSignature"] == nil && len(previous) <= 2 && len(part) <= 3 {
							oldText, oldOK := previous["text"].(string)
							nextText, nextOK := part["text"].(string)
							if oldOK && nextOK {
								previous["text"] = oldText + nextText
								if signature := part["thoughtSignature"]; signature != nil {
									previous["thoughtSignature"] = signature
								}
								continue
							}
						}
					}
					parts = append(parts, p)
				}
				for k, v := range content {
					if k != "parts" {
						oldContent[k] = v
					}
				}
				oldContent["parts"] = parts
			}
			for k, v := range candidate {
				if k != "content" {
					previous[k] = v
				}
			}
		}
	}
	if events == 0 {
		return nil, errors.New("Antigravity returned an empty stream")
	}
	indices := make([]int, 0, len(candidates))
	for i := range candidates {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	items := make([]any, 0, len(indices))
	for _, i := range indices {
		items = append(items, candidates[i])
	}
	result["candidates"] = items
	if usage != nil {
		result["usageMetadata"] = usage
	}
	return common.Marshal(result)
}

// Gemini's client converters process one content kind per chunk. Cloud Code
// can combine thinking, text and tools in a single event, so split those parts
// while retaining candidate indexes, final markers and cumulative usage.
func streamEvents(raw []byte) ([]byte, error) {
	var chunk map[string]any
	if err := common.Unmarshal(raw, &chunk); err != nil {
		return nil, err
	}
	candidates, _ := chunk["candidates"].([]any)
	var output bytes.Buffer
	wrote := false
	for candidateIndex, item := range candidates {
		candidate, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Invalid Antigravity candidate")
		}
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		if len(parts) == 0 {
			parts = []any{nil}
		}
		for partIndex, part := range parts {
			event := maps.Clone(chunk)
			current := maps.Clone(candidate)
			last := candidateIndex == len(candidates)-1 && partIndex == len(parts)-1
			if !last {
				delete(event, "usageMetadata")
			}
			if partIndex < len(parts)-1 {
				delete(current, "finishReason")
			}
			if part != nil {
				body := maps.Clone(content)
				body["parts"] = []any{part}
				current["content"] = body
			}
			event["candidates"] = []any{current}
			data, err := common.Marshal(event)
			if err != nil {
				return nil, err
			}
			output.WriteString("data: ")
			output.Write(data)
			output.WriteString("\n\n")
			wrote = true
		}
	}
	if !wrote {
		output.WriteString("data: ")
		output.Write(raw)
		output.WriteString("\n\n")
	}
	return output.Bytes(), nil
}
