// Package apns sends alert notifications through Apple's HTTP/2 provider API.
// It is independent of the MyGo application loop and can run in desktop apps
// and servers. Signing keys remain with the provider, never in the mobile app.
package apns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Environment string

const (
	Sandbox    Environment = "sandbox"
	Production Environment = "production"
)

// Config describes an APNs signing key. HTTPClient may customize networking,
// but must retain TLS verification and HTTP/2 support. MaxAttempts defaults to
// three; retries honor context cancellation and Retry-After (up to one minute).
type Config struct {
	TeamID, KeyID string
	PrivateKey    []byte // PEM PKCS#8 .p8, ECDSA P-256
	Environment   Environment
	HTTPClient    *http.Client
	MaxAttempts   int
}

type Client struct {
	config   Config
	key      *ecdsa.PrivateKey
	http     *http.Client
	endpoint string
	mu       sync.Mutex
	bearer   string
	issued   time.Time
}

func New(config Config) (*Client, error) {
	if !validID(config.TeamID) || !validID(config.KeyID) {
		return nil, errors.New("apns: TeamID and KeyID must be ten uppercase letters or digits")
	}
	if config.Environment == "" {
		config.Environment = Production
	}
	host := "https://api.push.apple.com"
	switch config.Environment {
	case Sandbox:
		host = "https://api.sandbox.push.apple.com"
	case Production:
	default:
		return nil, errors.New("apns: invalid environment")
	}
	block, rest := pem.Decode(config.PrivateKey)
	if block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("apns: expected a single PKCS#8 PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("apns: invalid PKCS#8 key")
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("apns: signing key must be ECDSA P-256")
	}
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 3
	}
	if config.MaxAttempts < 1 || config.MaxAttempts > 5 {
		return nil, errors.New("apns: MaxAttempts must be between one and five")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{ForceAttemptHTTP2: true, MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second}}
	}
	// Reject redirects so device tokens and Authorization never reach another host.
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	config.PrivateKey = nil
	return &Client{config: config, key: key, http: &copyClient, endpoint: host}, nil
}

func validID(value string) bool {
	if len(value) != 10 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func (c *Client) token(force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if !force && c.bearer != "" && now.Sub(c.issued) < 50*time.Minute && !now.Before(c.issued) {
		return c.bearer, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": c.config.KeyID})
	claims, _ := json.Marshal(map[string]any{"iss": c.config.TeamID, "iat": now.Unix()})
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, c.key, digest[:])
	if err != nil {
		return "", errors.New("apns: unable to sign provider token")
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	c.bearer = signing + "." + base64.RawURLEncoding.EncodeToString(sig)
	c.issued = now
	return c.bearer, nil
}

// Notification is an alert push. Expiration defaults to fifteen minutes;
// an explicit Unix epoch requests immediate-only delivery. CollapseID coalesces
// updates to the same reminder and must contain at most 64 UTF-8 bytes.
type Notification struct {
	DeviceToken, Topic string
	Payload            Payload
	CollapseID         string
	Expiration         time.Time
}

// Payload uses the same ID and custom string data as mygo.NotificationOptions,
// so App.OnNotification handles local and remote notifications uniformly.
type Payload struct {
	ID, Title, Subtitle, Body, Group string
	Data                             map[string]string
	Silent                           bool
	Badge                            *int
}

func (p Payload) MarshalJSON() ([]byte, error) {
	if p.Badge != nil && *p.Badge < 0 {
		return nil, errors.New("apns: negative badge")
	}
	aps := map[string]any{"alert": map[string]string{"title": p.Title, "subtitle": p.Subtitle, "body": p.Body}}
	if p.Group != "" {
		aps["thread-id"] = p.Group
	}
	if !p.Silent {
		aps["sound"] = "default"
	}
	if p.Badge != nil {
		aps["badge"] = *p.Badge
	}
	return json.Marshal(map[string]any{"aps": aps, "mygoID": p.ID, "mygo": p.Data})
}

type Response struct{ ID string }

// Error is a sanitized provider response. Unregistered means this device token
// must be removed, unless it was refreshed after the response's Timestamp.
type Error struct {
	Status     int
	Reason, ID string
	Timestamp  time.Time
	RetryAfter time.Duration
}

func (e *Error) Error() string      { return fmt.Sprintf("apns: HTTP %d (%s)", e.Status, e.Reason) }
func (e *Error) Unregistered() bool { return e.Status == 410 && e.Reason == "Unregistered" }
func (e *Error) Retryable() bool    { return e.Status == 429 || e.Status == 500 || e.Status == 503 }

func (c *Client) Send(ctx context.Context, n Notification) (Response, error) {
	// Device tokens are variable length; don't assume a fixed 32-byte value.
	if len(n.DeviceToken) == 0 || len(n.DeviceToken) > 512 || len(n.DeviceToken)%2 != 0 {
		return Response{}, errors.New("apns: invalid device token")
	}
	if _, err := hex.DecodeString(n.DeviceToken); err != nil {
		return Response{}, errors.New("apns: invalid device token")
	}
	if strings.TrimSpace(n.Topic) == "" || strings.ContainsAny(n.Topic, "\r\n\x00") {
		return Response{}, errors.New("apns: missing or invalid topic")
	}
	if len(n.CollapseID) > 64 || strings.ContainsAny(n.CollapseID, "\r\n\x00") {
		return Response{}, errors.New("apns: invalid collapse ID")
	}
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return Response{}, err
	}
	if len(payload) > 4096 {
		return Response{}, errors.New("apns: payload exceeds 4096 bytes")
	}
	if n.Expiration.IsZero() {
		n.Expiration = time.Now().Add(15 * time.Minute)
	}
	refresh := false
	for attempt := 0; attempt < c.config.MaxAttempts; attempt++ {
		result, sendErr := c.send(ctx, n, payload, refresh)
		if sendErr == nil {
			return result, nil
		}
		var provider *Error
		if !errors.As(sendErr, &provider) {
			return result, sendErr
		} // ambiguous transport failures are not blindly repeated
		refresh = provider.Reason == "ExpiredProviderToken" && !refresh
		if attempt+1 >= c.config.MaxAttempts || (!provider.Retryable() && !refresh) {
			return result, sendErr
		}
		delay := time.Duration(1<<attempt) * time.Second
		if refresh {
			delay = 0
		}
		if provider.RetryAfter > delay {
			delay = provider.RetryAfter
		}
		delay = min(delay, time.Minute)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{}, ctx.Err()
		case <-timer.C:
		}
	}
	return Response{}, errors.New("apns: attempts exhausted")
}

func (c *Client) send(ctx context.Context, n Notification, payload []byte, refresh bool) (Response, error) {
	bearer, err := c.token(refresh)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/3/device/"+n.DeviceToken, strings.NewReader(string(payload)))
	if err != nil {
		return Response{}, errors.New("apns: invalid request")
	}
	req.Header.Set("authorization", "bearer "+bearer)
	req.Header.Set("apns-topic", n.Topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-expiration", strconv.FormatInt(max(0, n.Expiration.Unix()), 10))
	req.Header.Set("content-type", "application/json")
	if n.CollapseID != "" {
		req.Header.Set("apns-collapse-id", n.CollapseID)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		// http errors include the URL containing the private token: never return it.
		return Response{}, errors.New("apns: provider transport failed")
	}
	defer res.Body.Close()
	result := Response{ID: res.Header.Get("apns-id")}
	if res.ProtoMajor != 2 {
		return result, errors.New("apns: provider requires HTTP/2")
	}
	if res.StatusCode == http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return result, nil
	}
	var body struct {
		Reason    string `json:"reason"`
		Timestamp int64  `json:"timestamp"`
	}
	json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&body)
	if body.Reason == "" {
		body.Reason = http.StatusText(res.StatusCode)
	}
	provider := &Error{Status: res.StatusCode, Reason: body.Reason, ID: result.ID}
	if body.Timestamp > 0 {
		provider.Timestamp = time.UnixMilli(body.Timestamp)
	}
	if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil {
		provider.RetryAfter = time.Duration(max(0, min(seconds, 60))) * time.Second
	} else if at, err := http.ParseTime(res.Header.Get("Retry-After")); err == nil {
		provider.RetryAfter = max(0, min(time.Until(at), time.Minute))
	}
	return result, provider
}
