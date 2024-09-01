package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/liushuangls/go-anthropic/v2"
)

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("missing %s environment variable", key))
	}
	return value
}

func main() {

	mRequest := anthropic.MessagesRequest{
		Model: "a regular string is valid here!",
		Messages: []anthropic.Message{
			{
				Role:    "regular strings are valid here too!",
				Content: []anthropic.MessageContent{anthropic.NewTextMessageContent("Hello, world!")},
			},
		},
	}

	client := anthropic.NewClient(mustGetEnv("ANTHROPIC_KEY"))
	resp, err := client.CreateMessages(context.Background(), mRequest)
	if err != nil {
		var e *anthropic.APIError
		if errors.As(err, &e) {
			fmt.Printf("Messages error, type: %s, message: %s", e.Type, e.Message)
		} else {
			fmt.Printf("Messages error: %v\n", err)
		}
		return
	}
	fmt.Println(resp.Content[0].GetText())

	rlhdr, err := resp.GetRateLimitHeaders()
	if err != nil {
		fmt.Printf("Rate limit error: %v\n", err)
		return
	}
	fmt.Println(rlhdr)
}
