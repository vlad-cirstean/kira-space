package testsupport

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"

	mobycontainer "github.com/moby/moby/api/types/container"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// The one genuine exception to "one container per adapter": SASL is a *listener* property
// fixed at broker boot, so a PLAINTEXT broker (kafka.go's StartKafka) cannot be made to require
// SASL at runtime the way every other adapter's least-privilege principals are created inside an
// already-running container. This is a second, purpose-built single-node KRaft broker whose only
// client listener is SASL_PLAINTEXT/PLAIN, with one user baked in at boot.
const (
	kafkaSaslUsername       = "kira"
	kafkaSaslPassword       = FixturePassword
	kafkaSaslClusterID      = "kira-sasl-test-cluster"
	kafkaSaslContainerPort  = "9095/tcp"
	kafkaSaslControllerPort = "9094/tcp"

	// KafkaSaslNoAclUsername/Password: a second principal baked into the same
	// PLAIN JAAS config, granted no ACLs at all once the authorizer below is enabled — the
	// SASL-authenticated-but-authorization-refused principal clickhouse/mongo/redis's own matrices
	// already have and kafka's did not. Static config, same as kira/kira, since PLAIN's credential
	// table is boot-time only (no dynamic-user API the way SCRAM has).
	//
	// The password must differ from the username (matrix.go's own
	// FixturePassword comment states the rule this used to violate — a fixture password must never
	// collide as a substring with any other fixture identifier, since the leak-assertion does a
	// substring check). Currently inert since this row expects success, but fragile for a future
	// case that reuses it.
	KafkaSaslNoAclUsername = "kira_noacl"
	KafkaSaslNoAclPassword = "kira-noacl-pw"
)

// KafkaSaslFixture is the SASL_PLAINTEXT/PLAIN counterpart to KafkaFixture — used only by the
// complete matrix's kafka auth rows (§2.7). Config is fields-mode with no credentials; a case sets
// Username/Password itself.
type KafkaSaslFixture struct {
	Config    model.ResolvedConnectionConfig
	Host      string
	Port      int
	container testcontainers.Container
}

var kafkaSaslMemo fixture[KafkaSaslFixture]

// StartKafkaSasl skips the test when Docker is unreachable, exactly like every other fixture in
// this package.
func StartKafkaSasl(t *testing.T) *KafkaSaslFixture {
	return kafkaSaslMemo.start(t, "kafka sasl container", startKafkaSasl)
}

// StopKafkaSasl terminates the memoized container, if one was ever started. Call once, from the
// test binary's own TestMain, after m.Run() returns — never from an individual test.
func StopKafkaSasl() {
	kafkaSaslMemo.stop(func(f *KafkaSaslFixture) { _ = f.container.Terminate(context.Background()) })
}

// pickFreeHostPort finds a currently-unused TCP port on the loopback interface. A small race
// window exists between closing this listener and the container binding the same port, the same
// one every "reserve a free port for later" pattern in Go accepts — acceptable here since this
// runs once per test binary, not per case.
func pickFreeHostPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func startKafkaSasl() (*KafkaSaslFixture, error) {
	ctx := context.Background()

	// The advertised listener has to name a port that is known *before* the broker's own
	// server.properties is generated from its env, which testcontainers' own random port mapping
	// cannot give us in time. Reserving a host port
	// ourselves and binding it explicitly via HostConfigModifier — rather than kafka.go's own
	// module's start-blocked-then-rewrite-and-launch script — sidesteps that entirely: the
	// advertised address is known up front, so the broker boots directly with its real
	// configuration on the first try.
	hostPort, err := pickFreeHostPort()
	if err != nil {
		return nil, err
	}

	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return nil, err
	}
	daemonHost, err := provider.DaemonHost(ctx)
	_ = provider.Close()
	if err != nil {
		return nil, err
	}

	saslPort, err := mobynetwork.ParsePort(kafkaSaslContainerPort)
	if err != nil {
		return nil, err
	}

	// A single mechanism entry serves three roles at once, per PlainLoginModule's own contract:
	// username/password is this broker's own identity when it dials itself for inter-broker
	// traffic, and each user_<name>=<password> is one entry in the table of credentials it accepts
	// from an incoming client. kira is also this cluster's sole super user (KAFKA_SUPER_USERS
	// below) so enabling the authorizer doesn't change its own behaviour or break inter-broker
	// traffic; kira_noacl authenticates the same way but is granted no ACLs at all, for the
	// authorization-refusal pin.
	jaas := fmt.Sprintf(
		`org.apache.kafka.common.security.plain.PlainLoginModule required username="%[1]s" password="%[2]s" user_%[1]s="%[2]s" user_%[3]s="%[4]s";`,
		kafkaSaslUsername, kafkaSaslPassword, KafkaSaslNoAclUsername, KafkaSaslNoAclPassword,
	)

	req := testcontainers.ContainerRequest{
		Image:        ImageFor("kafka", kafkaImage),
		ExposedPorts: []string{kafkaSaslContainerPort, kafkaSaslControllerPort},
		HostConfigModifier: func(hc *mobycontainer.HostConfig) {
			hc.PortBindings = mobynetwork.PortMap{
				saslPort: []mobynetwork.PortBinding{{HostIP: netip.IPv4Unspecified(), HostPort: strconv.Itoa(hostPort)}},
			}
		},
		Env: map[string]string{
			// The image's own /etc/confluent/docker/configure script hard-fails ("dub ensure
			// KAFKA_OPTS") the instant KAFKA_ADVERTISED_LISTENERS contains a SASL_ scheme, unless
			// KAFKA_OPTS is set to *something* — the per-listener
			// KAFKA_LISTENER_NAME_..._SASL_JAAS_CONFIG below is what configures PLAIN, so this only
			// has to satisfy that check, not
			// name a real login-config file.
			"KAFKA_OPTS":                                 "-Dkira.kafka.matrix.sasl=true",
			"CLUSTER_ID":                                 kafkaSaslClusterID,
			"KAFKA_NODE_ID":                              "1",
			"KAFKA_PROCESS_ROLES":                        "broker,controller",
			"KAFKA_CONTROLLER_QUORUM_VOTERS":             "1@localhost:9094",
			"KAFKA_CONTROLLER_LISTENER_NAMES":            "CONTROLLER",
			"KAFKA_LISTENERS":                            "SASL_PLAINTEXT://0.0.0.0:9095,CONTROLLER://0.0.0.0:9094",
			"KAFKA_ADVERTISED_LISTENERS":                 fmt.Sprintf("SASL_PLAINTEXT://%s:%d", daemonHost, hostPort),
			"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":       "SASL_PLAINTEXT:SASL_PLAINTEXT,CONTROLLER:PLAINTEXT",
			"KAFKA_INTER_BROKER_LISTENER_NAME":           "SASL_PLAINTEXT",
			"KAFKA_SASL_ENABLED_MECHANISMS":              "PLAIN",
			"KAFKA_SASL_MECHANISM_INTER_BROKER_PROTOCOL": "PLAIN",
			// dub's own env_to_props (docker_utils/dub.py) turns a single '_' into '.' and a
			// double '__' into a literal '_' — the listener name SASL_PLAINTEXT has to survive as
			// one dotted segment ("sasl_plaintext"), so the underscore inside it is doubled here,
			// or this silently becomes "sasl.plaintext" (two segments) and the broker never finds
			// its per-listener JAAS config at all (confirmed against a real container: it fails at
			// boot with "Could not find a 'KafkaServer' ... entry in the JAAS configuration").
			"KAFKA_LISTENER_NAME_SASL__PLAINTEXT_PLAIN_SASL_JAAS_CONFIG": jaas,
			// KF-4(a)/(d)'s own findings, same as the PLAINTEXT fixture: a single-node cluster needs
			// its own replication/ISR floors, and auto-created topics would pollute later tests.
			"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
			"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
			"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
			"KAFKA_AUTO_CREATE_TOPICS_ENABLE":                "false",
			// KRaft's own ACL authorizer — StandardAuthorizer, not the ZooKeeper-era
			// AclAuthorizer, since this cluster has no ZooKeeper at all. kira is the sole
			// SASL-authenticated super user, so it bypasses every ACL check exactly as it did with
			// no authorizer configured at all (both its own client use and its inter-broker
			// identity, see the jaas comment above); every other SASL principal (kira_noacl) is
			// denied by default, since Kafka's own allow.everyone.if.no.acl.found defaults to
			// false. User:ANONYMOUS also has to be a super user: the CONTROLLER listener is plain
			// PLAINTEXT (KAFKA_LISTENER_SECURITY_PROTOCOL_MAP above), so KRaft's own
			// controller-to-controller quorum traffic authenticates as ANONYMOUS — without this,
			// the broker fails to boot at all ("ClusterAuthorizationException ... is not
			// authorized" on the CONTROLLER listener, confirmed against a real container). The
			// CONTROLLER listener is never exposed to a client, so this carries no real privilege
			// escalation.
			"KAFKA_AUTHORIZER_CLASS_NAME": "org.apache.kafka.metadata.authorizer.StandardAuthorizer",
			"KAFKA_SUPER_USERS":           "User:" + kafkaSaslUsername + ";User:ANONYMOUS",
		},
		WaitingFor: wait.ForLog("(?i)Kafka Server started").AsRegexp().WithStartupTimeout(kafkaStartupTimeout),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start kafka sasl: %w", err)
	}

	// The log line precedes the advertised listener serving; see waitKafkaBrokerReady.
	client, err := kgo.NewClient(
		kgo.SeedBrokers(fmt.Sprintf("%s:%d", daemonHost, hostPort)),
		kgo.SASL(plain.Auth{User: kafkaSaslUsername, Pass: kafkaSaslPassword}.AsMechanism()),
	)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("kafka sasl readiness client: %w", err)
	}
	err = waitKafkaBrokerReady(ctx, client)
	client.Close()
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("kafka sasl broker not ready: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-kafka-sasl", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test Kafka SASL", Kind: "kafka", Color: "orange", Mode: "fields", ReadOnly: false,
		Host: Strp(daemonHost), Port: intp(hostPort), Options: map[string]any{},
	}
	return &KafkaSaslFixture{Config: cfg, Host: daemonHost, Port: hostPort, container: container}, nil
}

// CreateTopicSasl is CreateTopic's (kafka.go) sibling for the SASL broker — kafka.go's
// own CreateTopic takes a *KafkaFixture and reaches its already-built Admin field directly, which
// this fixture has no equivalent of (its Config carries no credentials of its own, a case sets them
// — kafka_sasl.go's own doc comment), so this dials a short-lived, correctly-authenticated kadm
// client rather than widening KafkaFixture/KafkaSaslFixture into one shared shape for two callers.
func CreateTopicSasl(t *testing.T, f *KafkaSaslFixture, name string) {
	t.Helper()
	client, err := kgo.NewClient(
		kgo.SeedBrokers(fmt.Sprintf("%s:%d", f.Host, f.Port)),
		kgo.SASL(plain.Auth{User: kafkaSaslUsername, Pass: kafkaSaslPassword}.AsMechanism()),
	)
	if err != nil {
		t.Fatalf("CreateTopicSasl: dial: %v", err)
	}
	defer client.Close()
	admin := kadm.NewClient(client)
	if _, err := admin.CreateTopics(context.Background(), 1, 1, nil, name); err != nil {
		t.Fatalf("CreateTopicSasl %s: %v", name, err)
	}
}
