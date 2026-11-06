package kibana

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

const (
	dialTimeout         = 10 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
	defaultSpace        = "default"
	// maxResultWindow is Elasticsearch's default index.max_result_window.
	maxResultWindow = 10_000
)

// Handle talks to one Kibana. No Client.Timeout: the op context bounds a request, so a long search
// is not cut short client-side while Kibana keeps running it.
type Handle struct {
	Client *http.Client
	// Base is scheme://host:port plus Kibana's server.basePath, when it has one.
	Base string
	// Space is the Kibana space id; "" is the default space.
	Space string
	// Authorization is the full header value, or "" for a Kibana with security off.
	Authorization string
	ReadOnly      bool
}

type target struct {
	scheme, host string
	port         int
	basePath     string
	space        string
	username     string
	password     string
}

// spacePathRE splits a Kibana URL path into its base path and a trailing /s/<space>.
var spacePathRE = regexp.MustCompile(`^(.*)/s/([^/]+)/?$`)

func resolveTarget(cfg model.ResolvedConnectionConfig) (target, error) {
	t := target{scheme: "http", port: 5601}
	var host, space, username, password *string
	var port *int
	if cfg.Mode == "uri" && cfg.URI != nil && *cfg.URI != "" {
		u, err := url.Parse(*cfg.URI)
		if err != nil || u.Hostname() == "" {
			return t, adapters.T(adapters.CodeConnect, "backend.studio.adapter.uriParseFailed.body", nil, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return t, adapters.T(adapters.CodeConnect, "backend.studio.adapter.kibanaUriScheme.body", nil, nil)
		}
		t.scheme = u.Scheme
		h := u.Hostname()
		host = &h
		if p := u.Port(); p != "" {
			n, err := strconv.Atoi(p)
			if err != nil {
				return t, adapters.T(adapters.CodeConnect, "backend.studio.adapter.portInvalid.body", nil, err)
			}
			port = &n
		}
		path := strings.TrimSuffix(u.Path, "/")
		if m := spacePathRE.FindStringSubmatch(path); m != nil {
			path, space = m[1], &m[2]
		}
		t.basePath = path
		if u.User != nil {
			un := u.User.Username()
			username = &un
			if pw, ok := u.User.Password(); ok {
				password = &pw
			}
		}
	} else {
		host, port, space, username, password = cfg.Host, cfg.Port, cfg.Database, cfg.Username, cfg.Password
		_, sslEnabled, err := adapters.ParseSSLMode(cfg.Options, "kibana", "require", "prefer", "verify-full")
		if err != nil {
			return t, err
		}
		if sslEnabled {
			t.scheme = "https"
		}
	}
	if host == nil || *host == "" {
		return t, adapters.T(adapters.CodeConnect, "backend.studio.adapter.hostMissing.body", nil, nil)
	}
	t.host = *host
	t.fill(port, space, username, password)
	return t, nil
}

// fill sets the optional parts, leaving the defaults where unset.
func (t *target) fill(port *int, space, username, password *string) {
	if port != nil {
		t.port = *port
	}
	if space != nil && *space != "" && *space != defaultSpace {
		t.space = *space
	}
	if username != nil {
		t.username = *username
	}
	if password != nil {
		t.password = *password
	}
}

// authorization: a username means Basic; a password alone is an encoded API key, as Kibana shows it.
func (t target) authorization() string {
	switch {
	case t.username != "":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(t.username+":"+t.password))
	case t.password != "":
		return "ApiKey " + t.password
	}
	return ""
}

// OpenClient builds the HTTP client for cfg; it does not touch the network.
func OpenClient(cfg model.ResolvedConnectionConfig) (*Handle, error) {
	t, err := resolveTarget(cfg)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 4,
		DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
	}
	return &Handle{
		Client:        &http.Client{Transport: transport},
		Base:          t.scheme + "://" + net.JoinHostPort(t.host, strconv.Itoa(t.port)) + t.basePath,
		Space:         t.space,
		Authorization: t.authorization(),
		ReadOnly:      cfg.ReadOnly,
	}, nil
}

func (h *Handle) spacePrefix() string {
	if h.Space == "" {
		return ""
	}
	return "/s/" + url.PathEscape(h.Space)
}

// maxResponseBytes bounds one reply body to the 64 MiB adapter host data frame.
const maxResponseBytes = 64 << 20

// request sends one call. internal routes need the version and origin headers Kibana demands of
// them; the space prefix applies to every route but the space lookup itself.
func (h *Handle) request(ctx context.Context, method, path string, internal, spaced bool, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return adapters.Internal("encode kibana request", err)
		}
		rd = bytes.NewReader(b)
	}
	full := h.Base
	if spaced {
		full += h.spacePrefix()
	}
	req, err := http.NewRequestWithContext(ctx, method, full+path, rd)
	if err != nil {
		return adapters.New(adapters.CodeConnect, err.Error(), err)
	}
	req.Header.Set("kbn-xsrf", "kira-studio")
	req.Header.Set("Content-Type", "application/json")
	if internal {
		req.Header.Set("elastic-api-version", "1")
		req.Header.Set("x-elastic-internal-origin", "Kibana")
	}
	if h.Authorization != "" {
		req.Header.Set("Authorization", h.Authorization)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return mapTransport(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return mapTransport(err)
	}
	if len(raw) > maxResponseBytes {
		return adapters.T(adapters.CodeQuery, "backend.studio.adapter.kibanaResponseTooLarge.body", nil, nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapStatus(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return adapters.New(adapters.CodeContract, "unexpected Kibana response from "+path+": "+err.Error(), err)
	}
	return nil
}

func (h *Handle) getJSON(ctx context.Context, path string, out any) error {
	return h.request(ctx, http.MethodGet, path, false, true, nil, out)
}

// search runs one Elasticsearch search through Kibana's es strategy and returns the raw response.
func (h *Handle) search(ctx context.Context, index string, body map[string]any) (searchResponse, error) {
	var out struct {
		RawResponse searchResponse `json:"rawResponse"`
	}
	err := h.request(ctx, http.MethodPost, "/internal/search/es", true, true,
		map[string]any{"params": map[string]any{"index": index, "body": body}}, &out)
	return out.RawResponse, err
}

// searchResponse is the part of an Elasticsearch search response this adapter reads.
type searchResponse struct {
	TimedOut        bool `json:"timed_out"`
	TerminatedEarly bool `json:"terminated_early"`
	Hits            struct {
		Total json.RawMessage `json:"total"`
		Hits  []searchHit     `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]json.RawMessage `json:"aggregations"`
}

type searchHit struct {
	Index  string                       `json:"_index"`
	ID     string                       `json:"_id"`
	Source json.RawMessage              `json:"_source"`
	Fields map[string][]json.RawMessage `json:"fields"`
}

// total reads hits.total, which Kibana's es strategy returns as a bare integer and a raw
// Elasticsearch response as {value, relation}.
func (r searchResponse) total() (n int64, relation string) {
	var asInt int64
	if json.Unmarshal(r.Hits.Total, &asInt) == nil {
		return asInt, "eq"
	}
	var obj struct {
		Value    int64  `json:"value"`
		Relation string `json:"relation"`
	}
	if json.Unmarshal(r.Hits.Total, &obj) == nil {
		return obj.Value, obj.Relation
	}
	return 0, "eq"
}
