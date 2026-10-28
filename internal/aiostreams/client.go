package aiostreams

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	BaseURL      string
	HTTP         *http.Client
	operator     *OperatorCredentials
	sessionMu    sync.Mutex
	sessionReady bool
}

// OperatorCredentials authenticate the server-side session used by
// AIOStreams when CONFIG_ACCESS_KEY protects configuration creation. They are
// held only in the provider process and are never part of a resource model.
type OperatorCredentials struct {
	Username string
	Password string
}

func New(baseURL string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("AIOStreams endpoint must be an origin without credentials or query parameters")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isPrivateHTTPHost(u.Hostname())) {
		return nil, errors.New("AIOStreams endpoint must use HTTPS (HTTP is allowed only for loopback or Kubernetes Service qualification)")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create AIOStreams cookie jar: %w", err)
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 20 * time.Second, Jar: jar},
	}, nil
}

// SetOperatorAuth enables the in-memory login session required by protected
// AIOStreams instances. The caller must provide an already separated pair.
func (c *Client) SetOperatorAuth(username, password string) error {
	if strings.TrimSpace(username) == "" || password == "" {
		return errors.New("AIOStreams operator credentials require a username and password")
	}
	c.operator = &OperatorCredentials{Username: username, Password: password}
	return nil
}

func isPrivateHTTPHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".svc.cluster.local")
}

type response struct {
	Success bool            `json:"success"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}

type Credentials struct {
	UUID     string
	Password string
}

type Created struct {
	UUID string `json:"uuid"`
}

type Retrieved struct {
	UserData json.RawMessage `json:"userData"`
}

type HTTPError struct {
	Method string
	Status int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("AIOStreams %s returned HTTP %d (response suppressed)", e.Method, e.Status)
}

func IsNotFound(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound
}

func (c *Client) request(ctx context.Context, method string, cred *Credentials, payload any, out any) error {
	return c.requestPath(ctx, method, "/api/v1/user", cred, payload, out)
}

func (c *Client) requestPath(ctx context.Context, method, path string, cred *Credentials, payload any, out any) error {
	if err := c.ensureOperatorSession(ctx); err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cred != nil {
		req.SetBasicAuth(cred.UUID, cred.Password)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("AIOStreams request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 8192))
		return &HTTPError{Method: method, Status: res.StatusCode}
	}
	var envelope response
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("invalid AIOStreams JSON response: %w", err)
	}
	if !envelope.Success {
		return errors.New("AIOStreams request returned unsuccessful envelope (detail suppressed)")
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("invalid AIOStreams response data: %w", err)
		}
	}
	return nil
}

func (c *Client) ensureOperatorSession(ctx context.Context) error {
	if c.operator == nil {
		return nil
	}
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.sessionReady {
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"username": c.operator.Username,
		"password": c.operator.Password,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("AIOStreams operator login failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 8192))
		return &HTTPError{Method: http.MethodPost + " /auth/login", Status: res.StatusCode}
	}
	var envelope response
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("invalid AIOStreams login response: %w", err)
	}
	if !envelope.Success {
		return errors.New("AIOStreams operator login returned unsuccessful envelope (detail suppressed)")
	}
	c.sessionReady = true
	return nil
}

func (c *Client) Create(ctx context.Context, password string, config json.RawMessage) (Created, error) {
	var out Created
	if !json.Valid(config) || len(config) == 0 || config[0] != '{' {
		return out, errors.New("config must be a JSON object")
	}
	err := c.request(ctx, http.MethodPost, nil, map[string]any{"password": password, "config": config}, &out)
	if err == nil && out.UUID == "" {
		err = errors.New("AIOStreams did not return UUID")
	}
	return out, err
}

func (c *Client) Get(ctx context.Context, cred Credentials) (json.RawMessage, error) {
	var out Retrieved
	err := c.requestPath(ctx, http.MethodGet, "/api/v1/user?raw=true", &cred, nil, &out)
	return out.UserData, err
}

func (c *Client) Update(ctx context.Context, cred Credentials, config json.RawMessage) error {
	if !json.Valid(config) || len(config) == 0 || config[0] != '{' {
		return errors.New("config must be a JSON object")
	}
	return c.request(ctx, http.MethodPut, &cred, map[string]any{"config": config}, nil)
}
