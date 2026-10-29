package kimi

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// normalizeUsage translates Kimi's inclusive message/start input count and its
// exclusive delta input count to Anthropic's non-cached input convention. The
// explicit presence of zero is retained; no stream event is summed twice.
func normalizeUsage(raw []byte, delta bool) ([]byte, error) {
	var root map[string]any
	if common.Unmarshal(raw, &root) != nil {
		return nil, errors.New("Invalid Kimi response JSON")
	}
	value := root
	if message, ok := root["message"].(map[string]any); ok {
		value = message
	}
	usage, ok := value["usage"].(map[string]any)
	if !ok {
		return raw, nil
	}
	readCount := func(key string) (int64, bool, error) {
		v, ok := usage[key]
		if !ok {
			return 0, false, nil
		}
		n, valid := v.(float64)
		if !valid || n < 0 || n > 2147483647 || n != float64(int64(n)) {
			return 0, true, errors.New("Invalid Kimi token usage")
		}
		return int64(n), true, nil
	}
	if _, _, err := readCount("output_tokens"); err != nil {
		return nil, err
	}
	input, hasInput, err := readCount("input_tokens")
	if err != nil {
		return nil, err
	}
	cache, _, err := readCount("cache_read_input_tokens")
	if err != nil {
		return nil, err
	}
	creation, _, err := readCount("cache_creation_input_tokens")
	if err != nil {
		return nil, err
	}
	prompt, hasPrompt, err := readCount("prompt_tokens")
	if err != nil {
		return nil, err
	}
	if hasInput {
		if hasPrompt {
			if input == prompt {
				if input < cache+creation {
					return nil, errors.New("Inconsistent Kimi token usage")
				}
				usage["input_tokens"] = input - cache - creation
			} else if input+cache+creation != prompt {
				return nil, errors.New("Inconsistent Kimi token usage")
			}
		} else if !delta {
			if input < cache+creation {
				return nil, errors.New("Inconsistent Kimi token usage")
			}
			usage["input_tokens"] = input - cache - creation
		}
	}
	return common.Marshal(root)
}

type codingStream struct {
	source   io.ReadCloser
	scanner  *bufio.Scanner
	pending  []byte
	target   types.RelayFormat
	terminal bool
	err      error
}

func newCodingStream(source io.ReadCloser, target types.RelayFormat) *codingStream {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	return &codingStream{source: source, scanner: scanner, target: target}
}
func (s *codingStream) Close() error { return s.source.Close() }
func (s *codingStream) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		if s.err != nil {
			return 0, s.err
		}
		if err := s.readEvent(); err != nil {
			s.err = err
			return 0, err
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

// SSE data may span multiple lines. Re-encode each complete JSON event as one
// data line so the shared protocol handlers receive exactly the same event.
func (s *codingStream) readEvent() error {
	var fields []string
	var data []string
	size := 0
	for {
		if !s.scanner.Scan() {
			if err := s.scanner.Err(); err != nil {
				return err
			}
			if size != 0 || !s.terminal {
				return io.ErrUnexpectedEOF
			}
			return io.EOF
		}
		line := s.scanner.Text()
		size += len(line)
		if size > 4<<20 {
			return errors.New("Kimi stream event exceeds size limit")
		}
		if line == "" {
			break
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(value, " "))
		} else {
			fields = append(fields, line)
		}
	}
	if len(data) == 0 {
		s.pending = []byte(strings.Join(fields, "\n") + "\n\n")
		return nil
	}
	raw := strings.Join(data, "\n")
	if raw == "[DONE]" {
		if s.target != types.RelayFormatOpenAI {
			return errors.New("Unexpected Kimi stream terminator")
		}
		s.terminal = true
	} else {
		var event map[string]any
		if common.UnmarshalJsonStr(raw, &event) != nil || event == nil {
			return errors.New("Invalid Kimi stream event")
		}
		eventType, _ := event["type"].(string)
		if eventType == "error" || eventType == "response.failed" || event["error"] != nil {
			return errors.New("Kimi upstream stream failed")
		}
		if s.target == types.RelayFormatClaude && eventType == "message_stop" || s.target == types.RelayFormatOpenAIResponses && (eventType == "response.completed" || eventType == "response.incomplete") {
			s.terminal = true
		}
		var encoded []byte
		var err error
		if s.target == types.RelayFormatClaude {
			encoded, err = normalizeUsage([]byte(raw), eventType == "message_delta")
		} else {
			encoded, err = common.Marshal(event)
		}
		if err != nil {
			return err
		}
		raw = string(encoded)
	}
	fields = append(fields, "data: "+raw)
	s.pending = []byte(strings.Join(fields, "\n") + "\n\n")
	return nil
}
