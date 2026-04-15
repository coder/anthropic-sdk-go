package anthropic_test

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	anthropic "github.com/charmbracelet/anthropic-sdk-go"
)

// generateToolInput creates a realistic tool call input JSON object.
func generateToolInput(rng *rand.Rand, size int) map[string]any {
	input := make(map[string]any, 4)
	input["command"] = randomString(rng, size/4)
	input["working_directory"] = "/home/user/project/src/" + randomString(rng, 20)
	input["timeout_ms"] = rng.IntN(30000)
	input["args"] = []string{
		randomString(rng, 10),
		randomString(rng, 10),
		randomString(rng, 10),
	}
	return input
}

// generateToolResultText creates realistic tool output text.
func generateToolResultText(rng *rand.Rand, size int) string {
	// Simulate a mix of code output, file contents, and command results.
	var sb strings.Builder
	lines := size / 80
	if lines < 1 {
		lines = 1
	}
	for i := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(randomString(rng, 60+rng.IntN(40)))
	}
	return sb.String()
}

// randomString generates a random alphanumeric string of the given length.
func randomString(rng *rand.Rand, n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-./(){}[]"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rng.IntN(len(charset))]
	}
	return string(b)
}

// buildConversation generates a realistic multi-turn conversation with the
// specified number of message pairs. Each pair consists of:
//   - An assistant message with a tool_use block
//   - A user message with a tool_result block
//
// This mirrors the chatd agentic loop where the model calls tools and
// receives results on every step.
func buildConversation(numPairs int) anthropic.MessageNewParams {
	// Fixed seed for reproducibility across benchmark runs.
	rng := rand.New(rand.NewPCG(42, 0))

	// Start with an initial user message.
	messages := make([]anthropic.MessageParam, 0, 1+numPairs*2)
	messages = append(messages, anthropic.NewUserMessage(
		anthropic.NewTextBlock("Help me refactor the authentication module. "+
			"The current implementation has several issues with token refresh "+
			"and session management. Here are the relevant files:\n\n"+
			generateToolResultText(rng, 2000)),
	))

	for i := range numPairs {
		toolCallID := fmt.Sprintf("toolu_%06d", i)
		toolName := "execute_command"
		if i%3 == 1 {
			toolName = "read_file"
		} else if i%3 == 2 {
			toolName = "write_file"
		}

		// Assistant message: thinking + tool call.
		thinkingText := fmt.Sprintf(
			"I need to %s to understand the current state. %s",
			toolName, randomString(rng, 200+rng.IntN(300)),
		)
		assistantBlocks := []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock(thinkingText),
			anthropic.NewToolUseBlock(
				toolCallID,
				generateToolInput(rng, 200+rng.IntN(300)),
				toolName,
			),
		}
		messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))

		// User message: tool result.
		// Tool results vary in size — some are small errors, some are
		// large file contents.
		resultSize := 500 + rng.IntN(2000)
		if i%10 == 0 {
			// Every 10th result is a large file read.
			resultSize = 5000 + rng.IntN(10000)
		}
		isError := rng.IntN(20) == 0 // 5% error rate
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewToolResultBlock(
				toolCallID,
				generateToolResultText(rng, resultSize),
				isError,
			),
		))
	}

	return anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude4Sonnet20250514,
		MaxTokens: 16384,
		Messages:  messages,
	}
}

func BenchmarkMarshalMessageNewParams(b *testing.B) {
	for _, numPairs := range []int{1, 10, 100, 1000} {
		params := buildConversation(numPairs)

		// Pre-marshal once to get the size for context.
		data, err := json.Marshal(params)
		if err != nil {
			b.Fatal(err)
		}

		b.Run(fmt.Sprintf("pairs=%d/json_bytes=%d", numPairs, len(data)), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				data, err = json.Marshal(params)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N), "marshals")
			b.ReportMetric(float64(len(data)), "json_bytes")

			// Total messages = 1 initial + numPairs*2
			totalMessages := 1 + numPairs*2
			b.ReportMetric(float64(totalMessages), "messages")
		})
	}
}
