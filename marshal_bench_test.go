package anthropic_test

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
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

// randomBase64 generates a fake base64 string of the given length.
func randomBase64(rng *rand.Rand, n int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rng.IntN(len(charset))]
	}
	return string(b)
}

// buildAssistantBlocks creates a varied set of content blocks for an
// assistant message, cycling through different block types to exercise
// all ContentBlockParamUnion variants.
func buildAssistantBlocks(rng *rand.Rand, step int) []anthropic.ContentBlockParamUnion {
	toolCallID := fmt.Sprintf("toolu_%06d", step)

	switch step % 7 {
	case 0:
		// Text + tool_use (most common agentic pattern).
		toolName := "execute_command"
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock(fmt.Sprintf(
				"I need to run a command. %s", randomString(rng, 200+rng.IntN(300)))),
			anthropic.NewToolUseBlock(toolCallID, generateToolInput(rng, 200), toolName),
		}
	case 1:
		// Thinking + text + tool_use.
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewThinkingBlock(
				randomBase64(rng, 64),
				fmt.Sprintf("Let me think about this... %s", randomString(rng, 300))),
			anthropic.NewTextBlock(randomString(rng, 100)),
			anthropic.NewToolUseBlock(toolCallID, generateToolInput(rng, 150), "read_file"),
		}
	case 2:
		// Redacted thinking + text + tool_use.
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewRedactedThinkingBlock(randomBase64(rng, 128)),
			anthropic.NewTextBlock(randomString(rng, 150)),
			anthropic.NewToolUseBlock(toolCallID, generateToolInput(rng, 100), "write_file"),
		}
	case 3:
		// Multiple tool calls in one message.
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock(randomString(rng, 100)),
			anthropic.NewToolUseBlock(toolCallID, generateToolInput(rng, 200), "execute_command"),
			anthropic.NewToolUseBlock(toolCallID+"b", generateToolInput(rng, 150), "read_file"),
		}
	case 4:
		// Server tool use.
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock(randomString(rng, 200)),
			anthropic.NewServerToolUseBlock(
				toolCallID,
				generateToolInput(rng, 100),
				anthropic.ServerToolUseBlockParamNameWebSearch),
		}
	case 5:
		// Text-only response (no tool calls).
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock(randomString(rng, 500+rng.IntN(500))),
		}
	default:
		// Thinking + text (no tool call).
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewThinkingBlock(randomBase64(rng, 64), randomString(rng, 400)),
			anthropic.NewTextBlock(randomString(rng, 200)),
		}
	}
}

// buildUserBlocks creates user message content blocks, including varied
// tool result types and occasionally images.
func buildUserBlocks(rng *rand.Rand, step int, toolCallIDs []string) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion

	// Add tool results for each tool call.
	for _, id := range toolCallIDs {
		resultSize := 500 + rng.IntN(2000)
		if step%10 == 0 {
			// Every 10th result is a large file read.
			resultSize = 5000 + rng.IntN(10000)
		}
		isError := rng.IntN(20) == 0

		blocks = append(blocks,
			anthropic.NewToolResultBlock(id, generateToolResultText(rng, resultSize), isError))
	}

	// Occasionally add an image block (base64).
	if step%8 == 3 {
		blocks = append(blocks,
			anthropic.NewImageBlockBase64("image/png", randomBase64(rng, 2000+rng.IntN(4000))))
	}

	// Occasionally add a search result block.
	if step%12 == 5 {
		blocks = append(blocks,
			anthropic.NewSearchResultBlock(
				[]anthropic.TextBlockParam{{Text: randomString(rng, 300)}},
				"https://example.com/"+randomString(rng, 30),
				randomString(rng, 50)))
	}

	// Occasionally add text alongside tool results.
	if step%5 == 0 {
		blocks = append(blocks, anthropic.NewTextBlock(randomString(rng, 100)))
	}

	return blocks
}

// extractToolCallIDs pulls tool call IDs from assistant blocks.
func extractToolCallIDs(blocks []anthropic.ContentBlockParamUnion) []string {
	var ids []string
	for _, b := range blocks {
		if b.OfToolUse != nil {
			ids = append(ids, b.OfToolUse.ID)
		}
		if b.OfServerToolUse != nil {
			ids = append(ids, b.OfServerToolUse.ID)
		}
	}
	return ids
}

// buildConversation generates a realistic multi-turn conversation with the
// specified number of message pairs. Each pair consists of an assistant
// message and a user message, exercising a wide variety of content block
// types to benchmark serialization of all ContentBlockParamUnion variants.
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
		// Assistant message with varied block types.
		assistantBlocks := buildAssistantBlocks(rng, i)
		messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))

		// User message with tool results matching the assistant's tool calls.
		toolCallIDs := extractToolCallIDs(assistantBlocks)
		if len(toolCallIDs) == 0 {
			// No tool calls — add a follow-up user text message.
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(randomString(rng, 100+rng.IntN(200)))))
		} else {
			userBlocks := buildUserBlocks(rng, i, toolCallIDs)
			messages = append(messages, anthropic.NewUserMessage(userBlocks...))
		}
	}

	return anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_5,
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
