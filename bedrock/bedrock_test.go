package bedrock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func TestBedrockURLEncoding(t *testing.T) {
	testCases := []struct {
		name            string
		model           string
		stream          bool
		expectedPath    string
		expectedRawPath string
	}{
		{
			name:            "regular model name",
			model:           "claude-3-sonnet",
			stream:          false,
			expectedPath:    "/model/claude-3-sonnet/invoke",
			expectedRawPath: "/model/claude-3-sonnet/invoke",
		},
		{
			name:            "regular model name with streaming",
			model:           "claude-3-sonnet",
			stream:          true,
			expectedPath:    "/model/claude-3-sonnet/invoke-with-response-stream",
			expectedRawPath: "/model/claude-3-sonnet/invoke-with-response-stream",
		},
		{
			name:            "inference profile ARN with slashes",
			model:           "arn:aws:bedrock:us-east-1:947123456126:application-inference-profile/xv9example4b",
			stream:          false,
			expectedPath:    "/model/arn:aws:bedrock:us-east-1:947123456126:application-inference-profile/xv9example4b/invoke",
			expectedRawPath: "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A947123456126%3Aapplication-inference-profile%2Fxv9example4b/invoke",
		},
		{
			name:            "inference profile ARN with streaming",
			model:           "arn:aws:bedrock:us-east-1:947123456126:application-inference-profile/xv9example4b",
			stream:          true,
			expectedPath:    "/model/arn:aws:bedrock:us-east-1:947123456126:application-inference-profile/xv9example4b/invoke-with-response-stream",
			expectedRawPath: "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A947123456126%3Aapplication-inference-profile%2Fxv9example4b/invoke-with-response-stream",
		},
		{
			name:            "foundation model ARN with colons",
			model:           "arn:aws:bedrock:us-east-1:123456789012:foundation-model/anthropic.claude-3-sonnet-20240229-v1:0",
			stream:          false,
			expectedPath:    "/model/arn:aws:bedrock:us-east-1:123456789012:foundation-model/anthropic.claude-3-sonnet-20240229-v1:0/invoke",
			expectedRawPath: "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Afoundation-model%2Fanthropic.claude-3-sonnet-20240229-v1%3A0/invoke",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock AWS config
			cfg := aws.Config{
				Region: "us-east-1",
				Credentials: credentials.StaticCredentialsProvider{
					Value: aws.Credentials{
						AccessKeyID:     "test-access-key",
						SecretAccessKey: "test-secret-key",
					},
				},
			}

			signer := v4.NewSigner()
			middleware := bedrockMiddleware(signer, cfg)

			// Create request body
			requestBody := map[string]any{
				"model":  tc.model,
				"stream": tc.stream,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			}

			bodyBytes, err := json.Marshal(requestBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}

			// Create HTTP request
			req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/v1/messages", bytes.NewReader(bodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			// Apply middleware
			_, err = middleware(req, func(r *http.Request) (*http.Response, error) {
				// Verify the URL paths are set correctly
				if r.URL.Path != tc.expectedPath {
					t.Errorf("Expected Path %q, got %q", tc.expectedPath, r.URL.Path)
				}

				if r.URL.RawPath != tc.expectedRawPath {
					t.Errorf("Expected RawPath %q, got %q", tc.expectedRawPath, r.URL.RawPath)
				}

				// Verify that the URL string contains the properly encoded path
				urlString := r.URL.String()
				expectedURL := fmt.Sprintf("https://bedrock-runtime.us-east-1.amazonaws.com%s", tc.expectedRawPath)
				if urlString != expectedURL {
					t.Errorf("Expected URL %q, got %q", expectedURL, urlString)
				}

				// Return a dummy response
				return &http.Response{
					StatusCode: 200,
					Body:       http.NoBody,
				}, nil
			})

			if err != nil {
				t.Fatalf("Middleware failed: %v", err)
			}
		})
	}
}

func TestBedrockStreamingHeaders(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
			},
		},
	}

	signer := v4.NewSigner()
	middleware := bedrockMiddleware(signer, cfg)

	testCases := []struct {
		name              string
		stream            bool
		wantAccept        string
		wantBedrockAccept string
	}{
		{
			name:       "non-streaming keeps JSON accept",
			stream:     false,
			wantAccept: "application/json",
		},
		{
			name:              "streaming uses Bedrock accept header",
			stream:            true,
			wantBedrockAccept: "application/json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestBody := map[string]any{
				"model":  "claude-3-sonnet",
				"stream": tc.stream,
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			}

			bodyBytes, err := json.Marshal(requestBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}

			req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/v1/messages", bytes.NewReader(bodyBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")

			_, err = middleware(req, func(r *http.Request) (*http.Response, error) {
				if got := r.Header.Get("Accept"); got != tc.wantAccept {
					t.Errorf("Expected Accept %q, got %q", tc.wantAccept, got)
				}
				if got := r.Header.Get("X-Amzn-Bedrock-Accept"); got != tc.wantBedrockAccept {
					t.Errorf("Expected X-Amzn-Bedrock-Accept %q, got %q", tc.wantBedrockAccept, got)
				}

				return &http.Response{
					StatusCode: 200,
					Body:       http.NoBody,
				}, nil
			})
			if err != nil {
				t.Fatalf("Middleware failed: %v", err)
			}
		})
	}
}

func TestBedrockBetaHeadersReRoutedThroughBody(t *testing.T) {
	// Create a mock AWS config
	cfg := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
			},
		},
	}

	signer := v4.NewSigner()
	middleware := bedrockMiddleware(signer, cfg)

	// Create HTTP request with beta headers
	type fakeRequest struct {
		Model         string              `json:"model"`
		AnthropicBeta []string            `json:"anthropic_beta,omitempty"`
		Messages      []map[string]string `json:"messages"`
	}
	reqBody := fakeRequest{
		Model: "fake-model",
		Messages: []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	requestBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/v1/messages", bytes.NewReader(requestBodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("anthropic-beta", "beta-feature-1")
	req.Header.Add("anthropic-beta", "beta-feature-2")

	// Apply middleware
	_, err = middleware(req, func(r *http.Request) (*http.Response, error) {
		// Read the modified body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}
		var modifiedBody fakeRequest
		err = json.Unmarshal(bodyBytes, &modifiedBody)
		if err != nil {
			t.Fatalf("Failed to unmarshal modified body: %v", err)
		}

		// Verify that the anthropic_beta field is present in the body
		expectedBetas := []string{"beta-feature-1", "beta-feature-2"}
		if len(modifiedBody.AnthropicBeta) != len(expectedBetas) {
			t.Fatalf("Expected %d beta features, got %d", len(expectedBetas), len(modifiedBody.AnthropicBeta))
		}
		for i, beta := range expectedBetas {
			if modifiedBody.AnthropicBeta[i] != beta {
				t.Errorf("Expected beta feature %q, got %q", beta, modifiedBody.AnthropicBeta[i])
			}
		}

		// Return a dummy response
		return &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
		}, nil
	})

	if err != nil {
		t.Fatalf("Middleware failed: %v", err)
	}
}

func TestBedrockStripsAnthropicAPIHeaders(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")

	cfg := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
			},
		},
	}

	signer := v4.NewSigner()
	middleware := bedrockMiddleware(signer, cfg)

	requestBody := map[string]any{
		"model": "claude-3-sonnet",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "test-anthropic-key")
	req.Header.Set("Anthropic-Version", "2023-06-01")

	_, err = middleware(req, func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("X-Api-Key"); got != "" {
			t.Errorf("Expected X-Api-Key to be stripped, got %q", got)
		}
		if got := r.Header.Get("Anthropic-Version"); got != "" {
			t.Errorf("Expected Anthropic-Version to be stripped, got %q", got)
		}

		authorization := r.Header.Get("Authorization")
		if !bytes.Contains([]byte(authorization), []byte("AWS4-HMAC-SHA256")) {
			t.Errorf("Expected SigV4 Authorization header, got %q", authorization)
		}
		if bytes.Contains([]byte(authorization), []byte("x-api-key")) {
			t.Errorf("Expected x-api-key not to be signed, got %q", authorization)
		}
		if bytes.Contains([]byte(authorization), []byte("anthropic-version")) {
			t.Errorf("Expected anthropic-version not to be signed, got %q", authorization)
		}

		return &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
		}, nil
	})
	if err != nil {
		t.Fatalf("Middleware failed: %v", err)
	}
}

func TestBedrockBearerToken(t *testing.T) {
	token := "test-bearer-token"
	region := "us-west-2"

	cfg := aws.Config{
		Region:                  region,
		BearerAuthTokenProvider: NewStaticBearerTokenProvider(token),
	}
	middleware := bedrockMiddleware(nil, cfg)

	requestBody := map[string]any{
		"model": "claude-3-sonnet",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-west-2.amazonaws.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = middleware(req, func(r *http.Request) (*http.Response, error) {
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + token
		if authHeader != expectedAuth {
			t.Errorf("Expected Authorization header %q, got %q", expectedAuth, authHeader)
		}

		if r.Header.Get("X-Amz-Date") != "" {
			t.Error("Expected no AWS SigV4 headers when using bearer token")
		}

		return &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
		}, nil
	})

	if err != nil {
		t.Fatalf("Middleware failed: %v", err)
	}
}
