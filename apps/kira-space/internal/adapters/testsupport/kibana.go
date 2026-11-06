package testsupport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Kibana fixture ids, shared with the conformance suite and the flow test.
const (
	KibanaLogsView   = "kira-logs"
	KibanaNoTimeView = "kira-notime"
	KibanaTitledView = "kira-titled"
	KibanaSpace      = "kira-space"
	KibanaSpaceView  = "space-view"
	// KibanaWindowFrom and KibanaWindowTo bound the seeded kira-logs documents.
	KibanaWindowFrom = "2026-01-01T00:00:00.000Z"
	KibanaWindowTo   = "2026-01-02T00:00:00.000Z"
	KibanaDocCount   = 200

	kibanaRoUser     = "kira"
	kibanaNoneUser   = "kira_none"
	kibanaElasticPW  = FixturePassword
	kibanaEncryption = "kira-fixture-encryption-key-0123456789ab"
)

// KibanaFixture is one Elasticsearch plus one Kibana container on a shared network.
type KibanaFixture struct {
	// Config reaches Kibana as the read-only role user `kira` (fields mode).
	Config model.ResolvedConnectionConfig
	// AdminConfig is the `elastic` superuser; APIKeyConfig authenticates with an encoded API key;
	// NoPrivConfig is a user with no Kibana privileges.
	AdminConfig, APIKeyConfig, NoPrivConfig model.ResolvedConnectionConfig
	BaseURL, ESURL                          string
	Host                                    string
	Port                                    int
	Password                                string
	APIKey                                  string
	es, kibana                              testcontainers.Container
	net                                     *testcontainers.DockerNetwork
}

var kibanaMemo fixture[KibanaFixture]

const (
	kibanaImage        = "kibana:9.5.3"
	elasticsearchImage = "elasticsearch:9.5.3"
)

func KibanaServerMajor() string { return ServerMajor("kibana", kibanaImage) }

// StartKibana starts the shared containers once per test binary (see fixture.go).
func StartKibana(t *testing.T) *KibanaFixture {
	return kibanaMemo.start(t, "kibana containers", startKibana)
}

// StopKibana terminates the memoized containers. Call from TestMain after m.Run().
func StopKibana() {
	kibanaMemo.stop(func(f *KibanaFixture) {
		ctx := context.Background()
		_ = f.kibana.Terminate(ctx)
		_ = f.es.Terminate(ctx)
		_ = f.net.Remove(ctx)
	})
}

func startKibana() (*KibanaFixture, error) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		return nil, err
	}
	es, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          ImageFor("elasticsearch", elasticsearchImage),
			ExposedPorts:   []string{"9200/tcp"},
			Networks:       []string{nw.Name},
			NetworkAliases: map[string][]string{nw.Name: {"es"}},
			Env: map[string]string{
				"discovery.type":                  "single-node",
				"xpack.security.enabled":          "true",
				"xpack.security.http.ssl.enabled": "false",
				"ELASTIC_PASSWORD":                kibanaElasticPW,
				"ES_JAVA_OPTS":                    "-Xms512m -Xmx512m",
				// A near-full disk (CI, sandbox) otherwise leaves every shard unassigned.
				"cluster.routing.allocation.disk.threshold_enabled": "false",
			},
			WaitingFor: wait.ForHTTP("/_cluster/health?wait_for_status=yellow").WithPort("9200/tcp").
				WithBasicAuth("elastic", kibanaElasticPW).WithStartupTimeout(8 * time.Minute),
		},
	})
	if err != nil {
		return nil, err
	}
	esHost, err := es.Host(ctx)
	if err != nil {
		return nil, err
	}
	esPort, err := es.MappedPort(ctx, "9200/tcp")
	if err != nil {
		return nil, err
	}
	esURL := fmt.Sprintf("http://%s:%s", esHost, esPort.Port())
	esc := &kibanaHTTP{base: esURL, user: "elastic", pass: kibanaElasticPW}
	// The security index is created lazily; the first security call can stall while it settles.
	for deadline := time.Now().Add(2 * time.Minute); ; time.Sleep(3 * time.Second) {
		err = esc.do(ctx, "POST", "/_security/user/kibana_system/_password", map[string]any{"password": kibanaElasticPW}, nil)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, err
		}
	}

	kb, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:          ImageFor("kibana", kibanaImage),
			ExposedPorts:   []string{"5601/tcp"},
			Networks:       []string{nw.Name},
			NetworkAliases: map[string][]string{nw.Name: {"kibana"}},
			Env: map[string]string{
				"ELASTICSEARCH_HOSTS":                       "http://es:9200",
				"ELASTICSEARCH_USERNAME":                    "kibana_system",
				"ELASTICSEARCH_PASSWORD":                    kibanaElasticPW,
				"XPACK_ENCRYPTEDSAVEDOBJECTS_ENCRYPTIONKEY": kibanaEncryption,
			},
			WaitingFor: wait.ForHTTP("/api/status").WithPort("5601/tcp").
				WithResponseMatcher(func(r io.Reader) bool {
					b, _ := io.ReadAll(r)
					return strings.Contains(string(b), `"level":"available"`)
				}).WithStartupTimeout(10 * time.Minute).WithPollInterval(2 * time.Second),
		},
	})
	if err != nil {
		return nil, err
	}
	host, err := kb.Host(ctx)
	if err != nil {
		return nil, err
	}
	mapped, err := kb.MappedPort(ctx, "5601/tcp")
	if err != nil {
		return nil, err
	}
	f := &KibanaFixture{
		BaseURL: fmt.Sprintf("http://%s:%s", host, mapped.Port()), ESURL: esURL, Host: host, Port: int(mapped.Num()),
		Password: kibanaElasticPW, es: es, kibana: kb, net: nw,
	}
	if err := f.seed(ctx); err != nil {
		return nil, err
	}
	f.buildConfigs()
	return f, nil
}

func (f *KibanaFixture) buildConfigs() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	host, port := f.Host, f.Port
	base := model.ResolvedConnectionConfig{
		ID: "test-kibana", CreatedAt: now, UpdatedAt: now,
		Name: "Test Kibana", Kind: "kibana", Color: "indigo", Mode: "fields", ReadOnly: true,
		Host: &host, Port: &port, Username: Strp(kibanaRoUser), Options: map[string]any{}, Password: Strp(f.Password),
	}
	f.Config = base
	f.AdminConfig = base
	f.AdminConfig.ID, f.AdminConfig.Username = "test-kibana-admin", Strp("elastic")
	f.APIKeyConfig = base
	f.APIKeyConfig.ID, f.APIKeyConfig.Username, f.APIKeyConfig.Password = "test-kibana-key", Strp(""), Strp(f.APIKey)
	f.NoPrivConfig = base
	f.NoPrivConfig.ID, f.NoPrivConfig.Username = "test-kibana-none", Strp(kibanaNoneUser)
}

type kibanaHTTP struct {
	base, user, pass string
}

func (c *kibanaHTTP) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("kbn-xsrf", "kira-studio")
	resp, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (f *KibanaFixture) seed(ctx context.Context) error {
	es := &kibanaHTTP{base: f.ESURL, user: "elastic", pass: kibanaElasticPW}
	kb := &kibanaHTTP{base: f.BaseURL, user: "elastic", pass: kibanaElasticPW}

	if err := es.do(ctx, "PUT", "/kira-logs", map[string]any{"mappings": map[string]any{"properties": map[string]any{
		"@timestamp": map[string]any{"type": "date"},
		"message":    map[string]any{"type": "text", "fields": map[string]any{"keyword": map[string]any{"type": "keyword", "ignore_above": 256}}},
		"level":      map[string]any{"type": "keyword"},
		"status":     map[string]any{"type": "integer"},
		"client":     map[string]any{"properties": map[string]any{"ip": map[string]any{"type": "ip"}}},
		"ok":         map[string]any{"type": "boolean"},
		"user":       map[string]any{"type": "nested", "properties": map[string]any{"name": map[string]any{"type": "keyword"}}},
		"tags":       map[string]any{"type": "keyword"},
	}}}, nil); err != nil {
		return err
	}
	if err := es.do(ctx, "PUT", "/kira-notime", map[string]any{"mappings": map[string]any{"properties": map[string]any{
		"name": map[string]any{"type": "keyword"}, "count": map[string]any{"type": "integer"},
	}}}, nil); err != nil {
		return err
	}

	var bulk strings.Builder
	start, _ := time.Parse(time.RFC3339, KibanaWindowFrom)
	step := 24 * time.Hour / KibanaDocCount
	levels := []string{"info", "warn", "error", "debug"}
	statuses := []int{200, 201, 404, 500, 503}
	users := []string{"alice", "bob", "carol"}
	tagSets := [][]string{{"a"}, {"b"}, {"a", "b"}, {"c"}, {"a", "c"}}
	for i := 0; i < KibanaDocCount; i++ {
		meta, _ := json.Marshal(map[string]any{"index": map[string]any{"_index": "kira-logs", "_id": fmt.Sprintf("doc-%03d", i)}})
		doc, _ := json.Marshal(map[string]any{
			"@timestamp": start.Add(time.Duration(i) * step).UTC().Format("2006-01-02T15:04:05.000Z"),
			"message":    fmt.Sprintf("request %d handled by worker-%d", i, i%7),
			"level":      levels[i%len(levels)],
			"status":     statuses[i%len(statuses)],
			"client":     map[string]any{"ip": fmt.Sprintf("10.0.0.%d", i%50+1)},
			"ok":         i%2 == 0,
			"user":       []map[string]any{{"name": users[i%len(users)]}},
			"tags":       tagSets[i%len(tagSets)],
		})
		bulk.Write(meta)
		bulk.WriteByte('\n')
		bulk.Write(doc)
		bulk.WriteByte('\n')
	}
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&bulk, `{"index":{"_index":"kira-notime","_id":"n-%02d"}}`+"\n"+`{"name":"item-%d","count":%d}`+"\n", i, i, i)
	}
	if err := f.bulk(ctx, bulk.String()); err != nil {
		return err
	}

	if err := kb.do(ctx, "POST", "/api/spaces/space", map[string]any{"id": KibanaSpace, "name": "Kira Space"}, nil); err != nil {
		return err
	}
	views := []struct {
		space string
		view  map[string]any
	}{
		{"", map[string]any{"id": KibanaLogsView, "title": "kira-logs", "name": "kira-logs", "timeFieldName": "@timestamp",
			"runtimeFieldMap": map[string]any{"status_class": map[string]any{"type": "keyword", "script": map[string]any{
				"source": "emit(doc['status'].value >= 500 ? 'error' : 'ok')"}}}}},
		{"", map[string]any{"id": KibanaNoTimeView, "title": "kira-notime", "name": "kira-notime"}},
		// Kibana rejects two data views with one name, so the name-versus-title case is the one testable.
		{"", map[string]any{"id": KibanaTitledView, "title": "kira-log*", "name": "Wildcard logs", "timeFieldName": "@timestamp"}},
		{"/s/" + KibanaSpace, map[string]any{"id": KibanaSpaceView, "title": "kira-logs", "name": "space-logs", "timeFieldName": "@timestamp"}},
	}
	for _, v := range views {
		if err := kb.do(ctx, "POST", v.space+"/api/data_views/data_view", map[string]any{"data_view": v.view, "override": true}, nil); err != nil {
			return err
		}
	}

	if err := kb.do(ctx, "PUT", "/api/security/role/kira_ro", map[string]any{
		"elasticsearch": map[string]any{"cluster": []string{"manage_own_api_key"}, "indices": []map[string]any{{"names": []string{"kira-*"}, "privileges": []string{"read", "view_index_metadata"}}}},
		"kibana":        []map[string]any{{"base": []string{}, "feature": map[string]any{"discover_v2": []string{"read"}, "indexPatterns": []string{"read"}}, "spaces": []string{"*"}}},
	}, nil); err != nil {
		return err
	}
	for _, u := range []struct {
		name  string
		roles []string
	}{{kibanaRoUser, []string{"kira_ro"}}, {kibanaNoneUser, []string{}}} {
		if err := es.do(ctx, "POST", "/_security/user/"+u.name, map[string]any{"password": kibanaElasticPW, "roles": u.roles}, nil); err != nil {
			return err
		}
	}
	var key struct {
		Encoded string `json:"encoded"`
	}
	if err := (&kibanaHTTP{base: f.ESURL, user: kibanaRoUser, pass: kibanaElasticPW}).do(ctx, "POST", "/_security/api_key", map[string]any{"name": "kira-fixture"}, &key); err != nil {
		return err
	}
	f.APIKey = key.Encoded
	return nil
}

func (f *KibanaFixture) bulk(ctx context.Context, body string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", f.ESURL+"/_bulk?refresh=wait_for", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth("elastic", kibanaElasticPW)
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || strings.Contains(string(data), `"errors":true`) {
		return fmt.Errorf("bulk: status %d: %.400s", resp.StatusCode, data)
	}
	return nil
}
