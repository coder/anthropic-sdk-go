package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// assumeRoleResult is the static set of temporary credentials returned by the
// mock STS server below.
const (
	assumedAccessKeyID = "ASIAASSUMEDEXAMPLE"
	assumedSecretKey   = "assumed-secret"
	assumedSessionTok  = "assumed-session-token"

	// expectedDefaultRoleSessionName mirrors awsauth.defaultRoleSessionName,
	// which is unexported. Keep these in sync.
	expectedDefaultRoleSessionName = "coder-aigateway"
)

// stsCapture records the form values the mock STS server received on the most
// recent AssumeRole call.
type stsCapture struct {
	RoleARN         string
	RoleSessionName string
	ExternalID      string
	Calls           int
}

// newMockSTS starts an HTTP server that mimics the AWS STS AssumeRole API and
// records the request parameters into capture. Tests point the SDK at it via
// the AWS_ENDPOINT_URL_STS environment variable.
//
// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
func newMockSTS(t *testing.T, capture *stsCapture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		capture.Calls++
		capture.RoleARN = r.Form.Get("RoleArn")
		capture.RoleSessionName = r.Form.Get("RoleSessionName")
		capture.ExternalID = r.Form.Get("ExternalId")

		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>` + assumedAccessKeyID + `</AccessKeyId>
      <SecretAccessKey>` + assumedSecretKey + `</SecretAccessKey>
      <SessionToken>` + assumedSessionTok + `</SessionToken>
      <Expiration>2999-01-01T00:00:00Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::123456789012:assumed-role/target/session</Arn>
      <AssumedRoleId>AROAEXAMPLE:session</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
</AssumeRoleResponse>`))
	}))
	t.Cleanup(server.Close)
	return server
}

// isolateAWSEnv clears the ambient AWS and Anthropic environment so tests are
// deterministic regardless of the host's configuration.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE",
		"AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME",
		"AWS_SESSION_TOKEN",
		"ANTHROPIC_AWS_API_KEY", "ANTHROPIC_AWS_WORKSPACE_ID",
		"ANTHROPIC_AWS_BASE_URL", "ANTHROPIC_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

func TestAssumeRoleProducesSignedSTSCredentials(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)

	client, captured := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		WorkspaceID:        "ws-assume",
	})
	sendTestRequest(t, client)

	if capture.Calls == 0 {
		t.Fatal("expected the STS AssumeRole endpoint to be called")
	}
	if capture.RoleARN != "arn:aws:iam::123456789012:role/target" {
		t.Errorf("unexpected RoleArn: %q", capture.RoleARN)
	}

	auth := captured.Headers.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		t.Fatalf("expected SigV4 Authorization header, got: %q", auth)
	}
	if !strings.Contains(auth, defaultServiceName) {
		t.Errorf("expected service %q in Authorization, got: %s", defaultServiceName, auth)
	}
	// The request must be signed with the assumed-role credentials, so the
	// assumed access key id appears in the SigV4 credential scope.
	if !strings.Contains(auth, assumedAccessKeyID) {
		t.Errorf("expected assumed access key id %q in Authorization, got: %s", assumedAccessKeyID, auth)
	}
	if captured.Headers.Get("X-Amz-Security-Token") != assumedSessionTok {
		t.Errorf("expected assumed session token in X-Amz-Security-Token, got: %q", captured.Headers.Get("X-Amz-Security-Token"))
	}
}

func TestAssumeRoleThreadsExternalID(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)

	client, _ := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		AWSExternalID:      "external-id-123",
		WorkspaceID:        "ws-assume",
	})
	sendTestRequest(t, client)

	if capture.ExternalID != "external-id-123" {
		t.Errorf("expected ExternalId %q in AssumeRole request, got %q", "external-id-123", capture.ExternalID)
	}
}

func TestAssumeRoleSessionNameDefault(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)

	client, _ := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		WorkspaceID:        "ws-assume",
	})
	sendTestRequest(t, client)

	if capture.RoleSessionName != expectedDefaultRoleSessionName {
		t.Errorf("expected default RoleSessionName %q, got %q", expectedDefaultRoleSessionName, capture.RoleSessionName)
	}
}

func TestAssumeRoleSessionNameCustom(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)

	client, _ := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		AWSRoleSessionName: "my-session",
		WorkspaceID:        "ws-assume",
	})
	sendTestRequest(t, client)

	if capture.RoleSessionName != "my-session" {
		t.Errorf("expected RoleSessionName %q, got %q", "my-session", capture.RoleSessionName)
	}
}

func TestAssumeRoleSessionNameFromEnv(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_ROLE_SESSION_NAME", "env-session")

	client, _ := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		WorkspaceID:        "ws-assume",
	})
	sendTestRequest(t, client)

	if capture.RoleSessionName != "env-session" {
		t.Errorf("expected RoleSessionName from env %q, got %q", "env-session", capture.RoleSessionName)
	}
}

func TestAssumeRoleFromEnvRoleARN(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/env-role")

	client, captured := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		WorkspaceID:        "ws-assume",
	})
	sendTestRequest(t, client)

	if capture.RoleARN != "arn:aws:iam::123456789012:role/env-role" {
		t.Errorf("expected RoleArn from AWS_ROLE_ARN env, got %q", capture.RoleARN)
	}
	if !strings.Contains(captured.Headers.Get("Authorization"), assumedAccessKeyID) {
		t.Error("expected request to be signed with assumed-role credentials resolved via AWS_ROLE_ARN")
	}
}

func TestAssumeRoleCachesCredentials(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)

	client, _ := newTestClient(t, ClientConfig{
		AWSRegion:          "us-east-1",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		WorkspaceID:        "ws-assume",
	})

	// The eager Retrieve during NewClient assumes the role once. Subsequent
	// requests must be served from the credentials cache rather than re-assuming.
	callsAfterSetup := capture.Calls
	if callsAfterSetup == 0 {
		t.Fatal("expected the role to be assumed during client setup")
	}
	sendTestRequest(t, client)
	sendTestRequest(t, client)

	if capture.Calls != callsAfterSetup {
		t.Errorf("expected no additional AssumeRole calls (cached), got %d extra", capture.Calls-callsAfterSetup)
	}
}

func TestAssumeRoleRequiresRegion(t *testing.T) {
	isolateAWSEnv(t)
	var capture stsCapture
	sts := newMockSTS(t, &capture)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)

	_, err := NewClient(context.Background(), ClientConfig{
		BaseURL:            "https://gateway.example.com",
		AWSAccessKey:       "base-access-key",
		AWSSecretAccessKey: "base-secret-key",
		AWSRoleARN:         "arn:aws:iam::123456789012:role/target",
		WorkspaceID:        "ws-assume",
	})
	if err == nil {
		t.Fatal("expected error when assuming a role without a region")
	}
}
