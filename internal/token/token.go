package token

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
)

type Counter struct {
	enc *tiktoken.Tiktoken
}

func New(encoding string) (*Counter, error) {
	enc, err := tiktoken.GetEncoding(encoding)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer %s: %w", encoding, err)
	}
	return &Counter{enc: enc}, nil
}

func (c *Counter) Count(text string) int {
	if text == "" {
		return 0
	}
	return len(c.enc.Encode(text, nil, nil))
}

func (c *Counter) Truncate(text string, maxTokens int) (string, int) {
	if text == "" || maxTokens <= 0 {
		return "", 0
	}
	tokens := c.enc.Encode(text, nil, nil)
	if len(tokens) <= maxTokens {
		return text, len(tokens)
	}

	truncated := c.enc.Decode(tokens[:maxTokens])
	return truncated, maxTokens
}
