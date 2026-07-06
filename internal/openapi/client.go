package openapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type APIError struct {
	Action     string
	StatusCode int
	RequestID  string
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("openapi %s failed: http %d requestId=%s: %s", e.Action, e.StatusCode, e.RequestID, e.Message)
	}
	return fmt.Sprintf("openapi %s failed: http %d requestId=%s", e.Action, e.StatusCode, e.RequestID)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusNotFound {
			return true
		}
		switch apiErr.Code {
		case "TemplateNotFound", "SandboxNotFound", "InstanceNotFound", "NotFound":
			return true
		}
	}
	return false
}

type Interface interface {
	CreateTemplate(ctx context.Context, cred Credential, req CreateTemplateRequest) (*CreateTemplateResponse, error)
	UpdateTemplate(ctx context.Context, cred Credential, req UpdateTemplateRequest) error
	DeleteTemplate(ctx context.Context, cred Credential, templateID string) error
	GetTemplate(ctx context.Context, cred Credential, templateID string) (*Template, error)
	ListTemplates(ctx context.Context, cred Credential, req ListTemplatesRequest) (*TemplateList, error)

	StartSandbox(ctx context.Context, cred Credential, req StartSandboxRequest) (*StartSandboxResponse, error)
	UpdateSandbox(ctx context.Context, cred Credential, req UpdateSandboxRequest) error
	DeleteSandbox(ctx context.Context, cred Credential, instanceIDs []string) error
	GetSandbox(ctx context.Context, cred Credential, instanceID string) (*Sandbox, error)
	ListSandboxes(ctx context.Context, cred Credential, req ListSandboxesRequest) (*SandboxList, error)
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Service    string
	Version    string
	AuthMode   string
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Service:    "aicp",
		Version:    "2026-04-01",
		AuthMode:   "kop-sigv4",
	}
}

type baseResponse struct {
	RequestID string          `json:"RequestId"`
	RequestId string          `json:"requestId"`
	Code      string          `json:"Code,omitempty"`
	Message   string          `json:"Message,omitempty"`
	Data      json.RawMessage `json:"Data"`
	Obj       json.RawMessage `json:"Obj"`
	Error     json.RawMessage `json:"Error,omitempty"`
}

func (c *Client) action(ctx context.Context, cred Credential, action string, payload any, out any) error {
	logger := log.FromContext(ctx)
	reqID := requestID()

	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	q := endpoint.Query()
	q.Set("Action", action)
	q.Set("Version", c.Version)
	if c.AuthMode == "kop-sigv4" {
		q.Set("Service", c.Service)
		q.Set("AccountId", cred.AccountID)
	} else {
		q.Set("Region", cred.Region)
	}
	endpoint.RawQuery = q.Encode()

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-KSC-REQUEST-ID", reqID)
	req.Header.Set("X-KSC-REGION", cred.Region)
	req.Header.Set("X-KSC-ACCOUNT-ID", cred.AccountID)
	req.Header.Set("X-KSC-SOURCE", "sandbox-operator")
	req.Header.Set("x-inner-api-call", "1")

	logger.V(1).Info("openapi request",
		"action", action,
		"requestID", reqID,
		"url", req.URL.String(),
		"accountID", cred.AccountID,
		"region", cred.Region,
	)

	if c.AuthMode == "kop-sigv4" {
		if err := c.signV4(req, body, cred); err != nil {
			logger.Error(err, "openapi sign v4 failed", "action", action, "requestID", reqID)
			return err
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		logger.Error(err, "openapi request failed", "action", action, "requestID", reqID)
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "openapi read response body failed", "action", action, "requestID", reqID)
		return err
	}
	var envelope baseResponse
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &envelope)
	}

	respReqID := responseRequestID(envelope)
	if respReqID == "" {
		respReqID = reqID
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message := responseError(envelope, responseBody)
		logger.Error(fmt.Errorf("openapi %s failed", action), "openapi error",
			"action", action,
			"requestID", respReqID,
			"statusCode", resp.StatusCode,
			"code", code,
			"message", message,
			"responseBody", string(responseBody),
		)
		return &APIError{Action: action, StatusCode: resp.StatusCode, RequestID: respReqID, Code: code, Message: message}
	}

	logger.V(1).Info("openapi response",
		"action", action,
		"requestID", respReqID,
		"statusCode", resp.StatusCode,
		"bodySize", len(responseBody),
	)

	if out == nil {
		return nil
	}
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &envelope); err != nil {
			logger.Error(err, "openapi decode response envelope failed", "action", action, "requestID", respReqID)
			return err
		}
	}
	data := envelope.Data
	if len(data) == 0 {
		data = envelope.Obj
	}
	if len(data) == 0 {
		data = responseBody
	}
	if err := json.Unmarshal(data, out); err != nil {
		logger.Error(err, "openapi decode response data failed", "action", action, "requestID", respReqID, "body", string(responseBody))
		return fmt.Errorf("decode openapi %s response: %w", action, err)
	}
	return nil
}

func responseRequestID(envelope baseResponse) string {
	if envelope.RequestID != "" {
		return envelope.RequestID
	}
	return envelope.RequestId
}

func responseError(envelope baseResponse, raw []byte) (string, string) {
	if envelope.Code != "" || envelope.Message != "" {
		return envelope.Code, envelope.Message
	}
	if len(envelope.Error) > 0 {
		var nested struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		if err := json.Unmarshal(envelope.Error, &nested); err == nil && (nested.Code != "" || nested.Message != "") {
			return nested.Code, nested.Message
		}
		return "", string(envelope.Error)
	}
	var direct struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil && (direct.Code != "" || direct.Message != "") {
		return direct.Code, direct.Message
	}
	if len(raw) > 0 {
		return "", string(raw)
	}
	return "", ""
}

func requestID() string {
	return fmt.Sprintf("sandbox-operator-%d", time.Now().UnixNano())
}

func (c *Client) signV4(req *http.Request, body []byte, cred Credential) error {
	if cred.AccessKeyID == "" || cred.SecretAccessKey == "" {
		return fmt.Errorf("openapi credential requires accessKeyId and secretAccessKey")
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-KSC-SERVICE", c.Service)
	req.Header.Set("X-KSC-REGION", cred.Region)

	payloadHash := sha256Hex(body)
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalQueryString(req.URL.Query())
	canonicalHeaders, signedHeaders := canonicalHeaders(req.Header)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := strings.Join([]string{dateStamp, cred.Region, c.Service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := sigV4SigningKey(cred.SecretAccessKey, dateStamp, cred.Region, c.Service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", cred.AccessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
	return nil
}

func canonicalQueryString(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0)
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, value := range vals {
			parts = append(parts, awsEscape(key)+"="+awsEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func canonicalHeaders(headers http.Header) (string, string) {
	collapsed := map[string]string{}
	for key, values := range headers {
		lower := strings.ToLower(key)
		trimmed := make([]string, 0, len(values))
		for _, value := range values {
			trimmed = append(trimmed, strings.Join(strings.Fields(value), " "))
		}
		collapsed[lower] = strings.Join(trimmed, ",")
	}
	keys := make([]string, 0, len(collapsed))
	for key := range collapsed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte(':')
		b.WriteString(collapsed[key])
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(keys, ";")
}

func awsEscape(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func sigV4SigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func (c *Client) CreateTemplate(ctx context.Context, cred Credential, req CreateTemplateRequest) (*CreateTemplateResponse, error) {
	var out CreateTemplateResponse
	return &out, c.action(ctx, cred, "CreateSandboxTemplate", req, &out)
}

func (c *Client) UpdateTemplate(ctx context.Context, cred Credential, req UpdateTemplateRequest) error {
	return c.action(ctx, cred, "UpdateSandboxTemplate", req, nil)
}

func (c *Client) DeleteTemplate(ctx context.Context, cred Credential, templateID string) error {
	return c.action(ctx, cred, "DeleteSandboxTemplate", map[string]string{"TemplateId": templateID}, nil)
}

func (c *Client) GetTemplate(ctx context.Context, cred Credential, templateID string) (*Template, error) {
	var out TemplateResponse
	if err := c.action(ctx, cred, "GetSandboxTemplate", map[string]string{"TemplateId": templateID}, &out); err != nil {
		return nil, err
	}
	value := out.Value()
	return &value, nil
}

func (c *Client) ListTemplates(ctx context.Context, cred Credential, req ListTemplatesRequest) (*TemplateList, error) {
	var out TemplateList
	return &out, c.action(ctx, cred, "GetSandboxTemplateList", req, &out)
}

func (c *Client) StartSandbox(ctx context.Context, cred Credential, req StartSandboxRequest) (*StartSandboxResponse, error) {
	var out StartSandboxResponse
	return &out, c.action(ctx, cred, "StartSandboxInstance", req, &out)
}

func (c *Client) UpdateSandbox(ctx context.Context, cred Credential, req UpdateSandboxRequest) error {
	return c.action(ctx, cred, "UpdateSandboxInstance", req, nil)
}

func (c *Client) DeleteSandbox(ctx context.Context, cred Credential, instanceIDs []string) error {
	return c.action(ctx, cred, "DeleteSandboxInstance", map[string][]string{"InstanceIds": instanceIDs}, nil)
}

func (c *Client) GetSandbox(ctx context.Context, cred Credential, instanceID string) (*Sandbox, error) {
	var out SandboxResponse
	if err := c.action(ctx, cred, "GetSandboxInstance", map[string]string{"InstanceId": instanceID}, &out); err != nil {
		return nil, err
	}
	value := out.Value()
	return &value, nil
}

func (c *Client) ListSandboxes(ctx context.Context, cred Credential, req ListSandboxesRequest) (*SandboxList, error) {
	var out SandboxList
	return &out, c.action(ctx, cred, "GetSandboxInstanceList", req, &out)
}
