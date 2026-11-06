package testsupport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// LocalStackImage mirrors packages/db-fixtures/support/{sqs,s3}.ts's own IMAGE — already namespaced (no
// library/ prefix), per CLAUDE.md's Docker section.
const (
	LocalStackImage           = "localstack/localstack:4.14.0"
	localStackPort            = "4566/tcp"
	localStackStartupTimeout  = 120 * time.Second
	LocalStackStaticAccessKey = "test"
	LocalStackStaticSecret    = "test"
	LocalStackRegion          = "us-east-1"
)

// startLocalStack is a bare GenericContainer, never testcontainers-go/modules/localstack
// — that module's Run bind-mounts the Docker socket (a Lambda-only feature neither sqs nor s3
// needs) behind a Must* that panics, and pulls in aws-sdk-go v1 alongside v2 for its own tests.
// A bare container starts here with no Docker socket bind at all. services is a
// comma-separated SERVICES value (e.g. "s3,sqs"); there is also no measurable startup
// difference in this sandbox, but restricting it is still the right default.
func startLocalStack(ctx context.Context, services string) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        ImageFor("localstack", LocalStackImage),
		ExposedPorts: []string{localStackPort},
		Env:          map[string]string{"SERVICES": services},
		WaitingFor:   wait.ForHTTP("/_localstack/health").WithPort(localStackPort).WithStartupTimeout(localStackStartupTimeout),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", err
	}
	host, err := c.Host(ctx)
	if err != nil {
		return c, "", err
	}
	port, err := c.MappedPort(ctx, localStackPort)
	if err != nil {
		return c, "", err
	}
	return c, fmt.Sprintf("http://%s:%s", host, port.Port()), nil
}

// OperationCountingProxy is a reverse proxy in front of LocalStack that counts requests carrying
// a given X-Amz-Target header value, keyed exactly (the header is "AmazonSQS.<Operation>" on the
// wire). Go has no spyOn/prototype-patch equivalent to intercept an
// adapter's own SDK client, and adding a test hook to production code for two scenarios is worse
// than a small proxy in testsupport — this is the only injection point an adapter's own
// options.endpoint already exposes.
type OperationCountingProxy struct {
	Endpoint string // hand this to a fixture's options.endpoint in place of the container's own

	server *httptest.Server
	target string
	mu     sync.Mutex
	count  int
}

// NewOperationCountingProxy starts a reverse proxy in front of backendEndpoint that counts every
// request whose X-Amz-Target header equals target.
func NewOperationCountingProxy(backendEndpoint, target string) (*OperationCountingProxy, error) {
	backend, err := url.Parse(backendEndpoint)
	if err != nil {
		return nil, err
	}
	p := &OperationCountingProxy{target: target}
	proxy := httputil.NewSingleHostReverseProxy(backend)
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		if r.Header.Get("X-Amz-Target") == p.target {
			p.mu.Lock()
			p.count++
			p.mu.Unlock()
		}
	}
	p.server = httptest.NewServer(proxy)
	p.Endpoint = p.server.URL
	return p, nil
}

// Count returns the number of matching requests seen so far.
func (p *OperationCountingProxy) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

// Reset zeroes the counter.
func (p *OperationCountingProxy) Reset() {
	p.mu.Lock()
	p.count = 0
	p.mu.Unlock()
}

// Close shuts the proxy down. Call from the owning fixture's own stop function.
func (p *OperationCountingProxy) Close() { p.server.Close() }
