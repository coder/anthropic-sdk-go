package awsauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// defaultRoleSessionName is the STS role session name used when AWSRoleARN is
// set but no session name is configured. A stable value keeps AssumeRole calls
// identifiable in CloudTrail.
const defaultRoleSessionName = "coder-aigateway"

// ClientConfig holds the configuration for creating an Anthropic client that authenticates
// via AWS credentials. This is the internal representation used by both the aws and bedrock
// mantle packages.
type ClientConfig struct {
	APIKey             string
	AWSAccessKey       string
	AWSSecretAccessKey string
	AWSSessionToken    string
	AWSProfile         string
	AWSRegion          string
	WorkspaceID        string
	BaseURL            string
	SkipAuth           bool

	// AWSRoleARN, when set, is the IAM role assumed via STS before signing
	// requests. The base identity (explicit static keys, AWSProfile, or the
	// default AWS credential chain) signs the AssumeRole call, and the
	// resulting temporary credentials sign API requests.
	AWSRoleARN string

	// AWSExternalID is the STS external ID used when assuming AWSRoleARN. It
	// mitigates the confused-deputy problem for cross-account role assumption
	// and is only meaningful when AWSRoleARN is set.
	AWSExternalID string

	// AWSRoleSessionName is the STS role session name used when assuming
	// AWSRoleARN. It defaults to defaultRoleSessionName when unset.
	AWSRoleSessionName string
}

// ResolveParams customizes config resolution per client (env var names, base URL derivation, service name).
type ResolveParams struct {
	// EnvAPIKey is the primary environment variable for the API key (e.g. "ANTHROPIC_AWS_API_KEY").
	EnvAPIKey string

	// EnvAPIKeyFallback is the fallback environment variable for the API key.
	// Used by Bedrock Mantle to fall back to the AWS API key env var.
	EnvAPIKeyFallback string

	// EnvWorkspaceID is the primary environment variable for the workspace ID.
	EnvWorkspaceID string

	// EnvWorkspaceIDFallback is the fallback environment variable for the workspace ID.
	EnvWorkspaceIDFallback string

	// EnvBaseURL is the environment variable for the base URL override.
	EnvBaseURL string

	// DeriveBaseURL is called with the resolved region to produce a default base URL
	// when neither the config field nor the env var is set.
	DeriveBaseURL func(region string) string

	// ServiceName is the AWS service name used in SigV4 signing.
	ServiceName string

	// UseBearerAuth, when true, sends the API key as an Authorization: Bearer header
	// instead of the default X-Api-Key header.
	UseBearerAuth bool
}

// ResolvedConfig holds the fully resolved configuration after applying defaults and env vars.
type ResolvedConfig struct {
	Region      string
	BaseURL     string
	WorkspaceID string
	APIKey      string
	UseSigV4    bool
	SkipAuth    bool
}

// ResolveConfig resolves all configuration values from ClientConfig fields and
// environment variable fallbacks, parameterized by ResolveParams.
func ResolveConfig(cfg ClientConfig, params ResolveParams) (ResolvedConfig, error) {
	var rc ResolvedConfig

	// Region: arg > AWS_REGION env > AWS_DEFAULT_REGION env
	rc.Region = cfg.AWSRegion
	if rc.Region == "" {
		rc.Region = os.Getenv("AWS_REGION")
	}
	if rc.Region == "" {
		rc.Region = os.Getenv("AWS_DEFAULT_REGION")
	}

	// Base URL: arg > env var > derived from region
	rc.BaseURL = cfg.BaseURL
	if rc.BaseURL == "" && params.EnvBaseURL != "" {
		rc.BaseURL = os.Getenv(params.EnvBaseURL)
	}
	if rc.BaseURL == "" && rc.Region != "" && params.DeriveBaseURL != nil {
		rc.BaseURL = params.DeriveBaseURL(rc.Region)
	}

	rc.SkipAuth = cfg.SkipAuth

	// Workspace ID: arg > primary env > fallback env
	rc.WorkspaceID = cfg.WorkspaceID
	if rc.WorkspaceID == "" && params.EnvWorkspaceID != "" {
		rc.WorkspaceID = os.Getenv(params.EnvWorkspaceID)
	}
	if rc.WorkspaceID == "" && params.EnvWorkspaceIDFallback != "" {
		rc.WorkspaceID = os.Getenv(params.EnvWorkspaceIDFallback)
	}
	// Workspace ID is required when env var names are configured (i.e. the caller expects it)
	if rc.WorkspaceID == "" && !rc.SkipAuth && (params.EnvWorkspaceID != "" || cfg.WorkspaceID != "") {
		envHint := params.EnvWorkspaceID
		if envHint == "" {
			envHint = "ANTHROPIC_AWS_WORKSPACE_ID"
		}
		return rc, fmt.Errorf("no workspace ID found; set WorkspaceID in ClientConfig or set the %s environment variable", envHint)
	}

	if !rc.SkipAuth {
		// Auth mode resolution
		switch {
		case cfg.APIKey != "":
			rc.APIKey = cfg.APIKey
		case (cfg.AWSAccessKey != "" && cfg.AWSSecretAccessKey != "") || cfg.AWSProfile != "" || cfg.AWSRoleARN != "":
			rc.UseSigV4 = true
		default:
			envKey := envLookup(params.EnvAPIKey, params.EnvAPIKeyFallback)
			if envKey != "" {
				rc.APIKey = envKey
			} else {
				rc.UseSigV4 = true
			}
		}

		if rc.UseSigV4 && rc.Region == "" {
			return rc, fmt.Errorf("no AWS region found; set AWSRegion in ClientConfig or set the AWS_REGION environment variable")
		}
	}

	if rc.BaseURL == "" {
		envHint := params.EnvBaseURL
		if envHint == "" {
			envHint = "ANTHROPIC_AWS_BASE_URL"
		}
		return rc, fmt.Errorf("no base URL found; set BaseURL or AWSRegion in ClientConfig, or set the %s or AWS_REGION environment variable", envHint)
	}

	return rc, nil
}

// envLookup returns the first non-empty value from the given environment variable names.
func envLookup(names ...string) string {
	for _, name := range names {
		if name != "" {
			if v := os.Getenv(name); v != "" {
				return v
			}
		}
	}
	return ""
}

// BuildAWSConfig creates an [awssdk.Config] from explicit credentials or the
// default AWS credential chain. When cfg.AWSRoleARN is set, the resolved base
// identity is used to assume that role via STS, and the returned config signs
// requests with the resulting temporary credentials.
func BuildAWSConfig(ctx context.Context, cfg ClientConfig, region string) (awssdk.Config, error) {
	// Resolve role-assumption parameters, honoring the standard AWS environment
	// variables when the corresponding config fields are unset. No env var is
	// honored for the external ID; it must be set explicitly.
	roleARN := cfg.AWSRoleARN
	if roleARN == "" {
		roleARN = os.Getenv("AWS_ROLE_ARN")
	}
	roleSessionName := cfg.AWSRoleSessionName
	if roleSessionName == "" {
		roleSessionName = os.Getenv("AWS_ROLE_SESSION_NAME")
	}
	if roleSessionName == "" {
		roleSessionName = defaultRoleSessionName
	}

	// Resolve the base credential source via the AWS SDK default config loader so
	// that region, shared config/profile, and standard endpoint resolution (e.g.
	// AWS_ENDPOINT_URL_STS) all apply uniformly. Explicit static keys, when
	// provided, override the credential provider but leave the rest of the
	// resolved config intact. The default chain otherwise covers env vars, shared
	// config, IRSA / EKS Pod Identity / EC2 Instance Profile, SSO, and more.
	loadOpts := []func(*config.LoadOptions) error{}
	if region != "" {
		loadOpts = append(loadOpts, config.WithRegion(region))
	}
	if cfg.AWSProfile != "" {
		loadOpts = append(loadOpts, config.WithSharedConfigProfile(cfg.AWSProfile))
	}
	if cfg.AWSAccessKey != "" && cfg.AWSSecretAccessKey != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AWSAccessKey,
				cfg.AWSSecretAccessKey,
				cfg.AWSSessionToken,
			),
		))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return awssdk.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// When a role ARN is configured, the base identity signs the STS AssumeRole
	// call and the resulting temporary credentials sign API requests. The
	// assume-role provider is wrapped in a credentials cache so the role is not
	// re-assumed on every request and is refreshed automatically before expiry.
	if roleARN != "" {
		if awsCfg.Region == "" {
			return awssdk.Config{}, fmt.Errorf("no AWS region found; a region is required to assume role %q", roleARN)
		}
		externalID := cfg.AWSExternalID
		provider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsCfg), roleARN, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = roleSessionName
			if externalID != "" {
				o.ExternalID = &externalID
			}
		})
		awsCfg.Credentials = awssdk.NewCredentialsCache(provider)
	}

	// Eagerly verify that credentials can be resolved so callers get a clear
	// error at setup time rather than on the first request. When a role is
	// assumed, this also surfaces STS misconfiguration (invalid ARN, missing
	// permissions, wrong external ID) at setup.
	if _, err := awsCfg.Credentials.Retrieve(ctx); err != nil {
		return awssdk.Config{}, fmt.Errorf("failed to resolve AWS credentials: %w", err)
	}

	return awsCfg, nil
}

// SigV4Middleware returns an HTTP middleware that signs requests using AWS SigV4
// with the given service name.
func SigV4Middleware(signer *v4.Signer, cfg awssdk.Config, serviceName string) option.Middleware {
	return func(r *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		var body []byte
		var err error

		if r.Body != nil {
			body, err = io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			r.Body.Close()
		}

		// r.Body and GetBody share a single reader. This is safe because the SDK's
		// retry logic calls GetBody (which seeks to 0) before re-reading the body,
		// and these operations are sequential within a single goroutine.
		reader := bytes.NewReader(body)
		r.Body = io.NopCloser(reader)
		r.GetBody = func() (io.ReadCloser, error) {
			_, err := reader.Seek(0, 0)
			return io.NopCloser(reader), err
		}
		r.ContentLength = int64(len(body))

		ctx := r.Context()
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
		}

		hash := sha256.Sum256(body)
		if err = signer.SignHTTP(ctx, creds, r, hex.EncodeToString(hash[:]), serviceName, cfg.Region, time.Now()); err != nil {
			return nil, fmt.Errorf("failed to sign request: %w", err)
		}

		return next(r)
	}
}

// CreateClientOptions returns request options that configure an Anthropic client for use
// with an AWS-based service. The params argument customizes env var names, base URL derivation,
// and SigV4 service name per client type.
//
// Caller-provided userOpts are placed after the base options (so they can
// override the base URL, headers, etc.) but before the SigV4 signing
// middleware: signing must run closest to the wire so the signature covers
// any body or header mutations made by user middleware.
func CreateClientOptions(ctx context.Context, cfg ClientConfig, params ResolveParams, userOpts ...option.RequestOption) ([]option.RequestOption, error) {
	resolved, err := ResolveConfig(cfg, params)
	if err != nil {
		return nil, err
	}

	var opts []option.RequestOption

	if resolved.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(resolved.BaseURL))
	}

	if resolved.SkipAuth {
		return append(opts, userOpts...), nil
	}

	if resolved.APIKey != "" {
		if params.UseBearerAuth {
			opts = append(opts, option.WithHeader("Authorization", "Bearer "+resolved.APIKey))
		} else {
			opts = append(opts, option.WithAPIKey(resolved.APIKey))
		}
	}
	if resolved.WorkspaceID != "" {
		opts = append(opts, option.WithHeader("anthropic-workspace-id", resolved.WorkspaceID))
	}

	opts = append(opts, userOpts...)

	if resolved.UseSigV4 {
		awsCfg, err := BuildAWSConfig(ctx, cfg, resolved.Region)
		if err != nil {
			return nil, err
		}

		signer := v4.NewSigner()
		opts = append(opts, option.WithMiddleware(SigV4Middleware(signer, awsCfg, params.ServiceName)))
	}

	return opts, nil
}
