package testsupport

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// 0005_kafka_seed.ts's own constants, re-expressed in Go (P58e E25).
const (
	KafkaOrdersTopic          = "orders"
	KafkaEmptyTopic           = "empty-topic" // exercises a topic with zero messages
	KafkaOrdersPartitionCount = 2
	KafkaOrdersMessageCount   = 6 // > one partition's worth, so browsing genuinely spans both
	KafkaConsumerGroup        = "kira-test-group"
)

const (
	// kafkaImage mirrors packages/db-fixtures/support/kafka.ts's own IMAGE (P32 D25) — already namespaced (no
	// library/ prefix), per CLAUDE.md's Docker section.
	kafkaImage = "confluentinc/cp-kafka:8.0.7"
	// kafkaPublicPort is the module's own exposed PLAINTEXT listener (kafka.go's publicPort,
	// unexported by testcontainers-go/modules/kafka).
	kafkaPublicPort     = "9093/tcp"
	kafkaStartupTimeout = 180 * time.Second
	// kafkaClusterID satisfies KF-4(a)'s finding: kafka.Run fails outright ("CLUSTER_ID is
	// required") unless kafka.WithClusterID is passed explicitly — the module has no default.
	kafkaClusterID = "kira-test-cluster"
)

// KafkaFixture is support/kafka.ts's KafkaFixture, plus a side kgo/kadm client pair — the admin
// half seeds the fixture and gives every producing test its own topic (P58e E27), and the kgo
// half is what P58e E25's seeder produces the orders messages with.
type KafkaFixture struct {
	Config    model.ResolvedConnectionConfig // ready to hand to the adapter
	Client    *kgo.Client                    // a side client, for seeding and per-test setup only
	Admin     *kadm.Client
	container testcontainers.Container
}

// ConnectionID satisfies hasConnectionID (P107 I2-28, FixtureNodePath's own constraint).
func (f KafkaFixture) ConnectionID() string { return f.Config.ID }

var kafkaMemo = fixture[KafkaFixture]{reset: resetKafka}

// StartKafka is support/kafka.ts's startKafka. Skips the test when Docker is unreachable.
func StartKafka(t *testing.T) *KafkaFixture {
	return kafkaMemo.start(t, "kafka container", startKafka)
}

// StopKafka terminates the memoized container and closes its side client, if ever started. Call
// once, from the test binary's own TestMain, after m.Run() returns — never from an individual
// test's t.Cleanup, per fixture.go's own doc comment (P58b B15).
func StopKafka() {
	kafkaMemo.stop(func(f *KafkaFixture) {
		f.Client.Close()
		_ = f.container.Terminate(context.Background())
	})
}

const (
	kafkaReadyPollInterval = 250 * time.Millisecond
	kafkaReadyTimeout      = 60 * time.Second
)

// waitKafkaBrokerReady polls a real request until the broker serves it. The module's own
// log-based readiness hook (tckafka.Run's PostStarts wait strategy) fires on a log line the broker
// writes before its advertised listener is accepting client traffic — under load, the
// window between the two is wide enough that the first real request (seedKafka's CreateTopics) can
// land mid-ApiVersions negotiation and see the connection closed. client.Ping issues a real
// Metadata request through the same negotiation path a production caller would use, so it fails
// exactly the way that first request would, until the broker is genuinely ready.
func waitKafkaBrokerReady(ctx context.Context, client *kgo.Client) error {
	deadline := time.Now().Add(kafkaReadyTimeout)
	var lastErr error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, kafkaReadyPollInterval)
		lastErr = client.Ping(pingCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("kafka broker not ready after %s: %w", kafkaReadyTimeout, lastErr)
		}
		time.Sleep(kafkaReadyPollInterval)
	}
}

func startKafka() (*KafkaFixture, error) {
	// The module's own readiness wait (a log-regex match inside a PostStarts hook, kafka.go's own
	// Run) takes no configurable timeout option — it is bounded only by the ctx passed to Run
	// itself, unlike testcontainers.WithWaitStrategyAndDeadline's own deadline parameter. Scoped to
	// the Run call alone, not the seeding that follows.
	startupCtx, cancel := context.WithTimeout(context.Background(), kafkaStartupTimeout)
	defer cancel()

	// kafka.WithClusterID is not optional — Run fails outright with "CLUSTER_ID is required"
	// without it. KAFKA_AUTO_CREATE_TOPICS_ENABLE=false: without it at the container level, a
	// nonexistent-topic read auto-creates the topic broker-side (independent of any client-side
	// AllowAutoTopicCreation setting) and pollutes every test that runs after it in the same
	// container.
	c, err := tckafka.Run(startupCtx, ImageFor("kafka", kafkaImage),
		tckafka.WithClusterID(kafkaClusterID),
		testcontainers.WithEnv(map[string]string{"KAFKA_AUTO_CREATE_TOPICS_ENABLE": "false"}),
	)
	if err != nil {
		return nil, fmt.Errorf("start kafka: %w", err)
	}

	ctx := context.Background()
	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	mapped, err := c.MappedPort(ctx, kafkaPublicPort)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	port := int(mapped.Num())

	client, err := kgo.NewClient(kgo.SeedBrokers(fmt.Sprintf("%s:%d", host, port)))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	admin := kadm.NewClient(client)

	if err := waitKafkaBrokerReady(ctx, client); err != nil {
		client.Close()
		_ = c.Terminate(ctx)
		return nil, err
	}

	if err := seedKafka(ctx, admin, client); err != nil {
		client.Close()
		_ = c.Terminate(ctx)
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-kafka", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test Kafka", Kind: "kafka", Color: "orange", Mode: "fields", ReadOnly: false,
		Host: Strp(host), Port: intp(port), Options: map[string]any{},
	}
	return &KafkaFixture{Config: cfg, Client: client, Admin: admin, container: c}, nil
}

// resetKafka deletes every user topic and consumer group, waits until the broker lists none, then
// reseeds. It uses a fresh client: the shared client answers metadata from a 5 s cache, so it would
// keep listing deleted topics and hide the new ones.
func resetKafka(f *KafkaFixture) error {
	ctx := context.Background()
	client, err := kgo.NewClient(kgo.SeedBrokers(fmt.Sprintf("%s:%d", *f.Config.Host, *f.Config.Port)))
	if err != nil {
		return err
	}
	defer client.Close()
	admin := kadm.NewClient(client)
	if err := deleteKafkaGroups(ctx, admin); err != nil {
		return err
	}
	if err := deleteKafkaTopics(ctx, admin); err != nil {
		return err
	}
	return seedKafka(ctx, admin, client)
}

func deleteKafkaGroups(ctx context.Context, admin *kadm.Client) error {
	groups, err := admin.ListGroups(ctx)
	if err != nil {
		return err
	}
	names := groups.Groups()
	if len(names) == 0 {
		return nil
	}
	resp, err := admin.DeleteGroups(ctx, names...)
	if err != nil {
		return err
	}
	for _, g := range resp.Sorted() {
		if g.Err != nil {
			return fmt.Errorf("delete group %s: %w", g.Group, g.Err)
		}
	}
	return nil
}

func userKafkaTopics(ctx context.Context, admin *kadm.Client) ([]string, error) {
	td, err := admin.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range td.Names() {
		if !strings.HasPrefix(n, "__") {
			out = append(out, n)
		}
	}
	return out, nil
}

// deleteKafkaTopics returns once the broker lists no user topic: deletion is asynchronous, and
// recreating a name too early answers "already exists".
func deleteKafkaTopics(ctx context.Context, admin *kadm.Client) error {
	topics, err := userKafkaTopics(ctx, admin)
	if err != nil {
		return err
	}
	if len(topics) > 0 {
		resp, err := admin.DeleteTopics(ctx, topics...)
		if err != nil {
			return err
		}
		for _, r := range resp.Sorted() {
			if r.Err != nil {
				return fmt.Errorf("delete topic %s: %w", r.Topic, r.Err)
			}
		}
	}
	deadline := time.Now().Add(kafkaReadyTimeout)
	for {
		left, err := userKafkaTopics(ctx, admin)
		if err != nil {
			return err
		}
		if len(left) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("topics still listed after delete: %v", left)
		}
		time.Sleep(kafkaReadyPollInterval)
	}
}

// createSeedTopic reports the broker's per-topic answer, which CreateTopics returns beside a nil error.
func createSeedTopic(ctx context.Context, admin *kadm.Client, partitions int32, name string) error {
	resp, err := admin.CreateTopics(ctx, partitions, 1, nil, name)
	if err != nil {
		return fmt.Errorf("create topic %s: %w", name, err)
	}
	for _, r := range resp.Sorted() {
		if r.Err != nil {
			return fmt.Errorf("create topic %s: %w (%s)", r.Topic, r.Err, r.ErrMessage)
		}
	}
	return nil
}

// seedKafka is the Go re-expression of 0005_kafka_seed.ts, seeded from the host with
// kadm + kgo rather than the broker's own CLI run inside the container. Neither of the
// TypeScript's own two reasons for the in-container CLI survives translation: this test binary
// already links a Kafka client, so there is no client to keep out of the process, and
// kadm.CommitOffsets commits a group's offsets "outside the context of a Kafka
// group" (kgo/config.go's own doc for ConsumePartitions points at exactly this use) — the same
// committed-offsets-with-no-members state --reset-offsets --to-earliest --execute produced.
func seedKafka(ctx context.Context, admin *kadm.Client, client *kgo.Client) error {
	if err := createSeedTopic(ctx, admin, KafkaOrdersPartitionCount, KafkaOrdersTopic); err != nil {
		return err
	}
	if err := createSeedTopic(ctx, admin, 1, KafkaEmptyTopic); err != nil {
		return err
	}

	records := make([]*kgo.Record, KafkaOrdersMessageCount)
	for i := 0; i < KafkaOrdersMessageCount; i++ {
		records[i] = &kgo.Record{
			Topic:   KafkaOrdersTopic,
			Key:     []byte(fmt.Sprintf("key-%d", i)),
			Value:   []byte(fmt.Sprintf(`{"seq":%d}`, i)),
			Headers: []kgo.RecordHeader{{Key: "source", Value: []byte("seed")}},
		}
	}
	if err := client.ProduceSync(ctx, records...).FirstErr(); err != nil {
		return fmt.Errorf("seed %s: %w", KafkaOrdersTopic, err)
	}

	// Registers KafkaConsumerGroup with committed offsets and no members — the same state
	// scenario 6 (kafka.spec.ts) needs, and the same state --reset-offsets --to-earliest --execute
	// against a group that has never existed produces.
	starts, err := admin.ListStartOffsets(ctx, KafkaOrdersTopic)
	if err != nil {
		return fmt.Errorf("list start offsets: %w", err)
	}
	if err := starts.Error(); err != nil {
		return fmt.Errorf("list start offsets: %w", err)
	}
	if _, err := admin.CommitOffsets(ctx, KafkaConsumerGroup, starts.Offsets()); err != nil {
		return fmt.Errorf("commit offsets for %s: %w", KafkaConsumerGroup, err)
	}
	return nil
}

// CreateTopic creates a fresh, dedicated topic for one producing test (P58e E27) — orders and
// empty-topic stay read-only fixtures, so scenarios asserting their message counts never depend
// on test execution order the way packages/db-fixtures/kafka.spec.ts's own top-to-bottom bun:test run did.
func CreateTopic(t *testing.T, f *KafkaFixture, name string) {
	t.Helper()
	if _, err := f.Admin.CreateTopics(context.Background(), 1, 1, nil, name); err != nil {
		t.Fatalf("CreateTopic %s: %v", name, err)
	}
}
