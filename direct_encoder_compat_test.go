package anthropic_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestMarshalMessageNewParams_ByteIdentical(t *testing.T) {
	// Build conversations of various sizes.
	for _, numPairs := range []int{1, 10, 50} {
		t.Run(fmt.Sprintf("pairs=%d", numPairs), func(t *testing.T) {
			params := buildConversation(numPairs)

			// Marshal twice — output must be deterministic and identical.
			first, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			second, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("Marshal output is not deterministic")
			}

			// Verify output is valid JSON.
			var raw json.RawMessage
			if err := json.Unmarshal(first, &raw); err != nil {
				t.Fatalf("Marshal output is not valid JSON: %v", err)
			}

			// Verify key structure exists.
			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(first, &parsed); err != nil {
				t.Fatal(err)
			}
			if _, ok := parsed["messages"]; !ok {
				t.Fatal("missing 'messages' key in output")
			}
			if _, ok := parsed["model"]; !ok {
				t.Fatal("missing 'model' key in output")
			}
		})
	}
}
