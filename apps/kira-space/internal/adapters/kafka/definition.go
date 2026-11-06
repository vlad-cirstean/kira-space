package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

func joinInts(vals []int32) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatInt(int64(v), 10)
	}
	return strings.Join(parts, ",")
}

type topicPartitionDoc struct {
	ID       int32   `json:"id"`
	Leader   int32   `json:"leader"`
	Replicas []int32 `json:"replicas"`
	ISR      []int32 `json:"isr"`
}

type topicConfigDoc struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Default bool   `json:"default"`
}

// buildTopicDefinition is definition.ts's buildTopicDefinition (:35-97). The
// Configuration section is populated via kadm.DescribeTopicConfigs — the capability the
// TypeScript adapter's own comment recorded as permanently lost, because "this binding simply
// does not wrap [DescribeConfigs]" — replacing the permanent "not available" note entirely.
func buildTopicDefinition(ctx context.Context, adm *kadm.Client, topic string) (model.ObjectDefinition, error) {
	meta, err := adm.Metadata(ctx, topic)
	if err != nil {
		return model.ObjectDefinition{}, mapError(err)
	}
	var partitions []kadm.PartitionDetail
	if detail, ok := meta.Topics[topic]; ok {
		partitions = detail.Partitions.Sorted()
	}

	partitionsSection := model.DefinitionSection{Title: i18n.T("backend.definition.section.partitions.title", nil), Rows: make([]model.DefinitionSectionRow, len(partitions))}
	partitionDocs := make([]topicPartitionDoc, len(partitions))
	for i, p := range partitions {
		detail := fmt.Sprintf("replicas %s · isr %s", joinInts(p.Replicas), joinInts(p.ISR))
		partitionsSection.Rows[i] = model.DefinitionSectionRow{
			Name: strconv.FormatInt(int64(p.Partition), 10), Value: fmt.Sprintf("leader %d", p.Leader), Detail: &detail,
		}
		partitionDocs[i] = topicPartitionDoc{ID: p.Partition, Leader: p.Leader, Replicas: p.Replicas, ISR: p.ISR}
	}

	configSection := model.DefinitionSection{Title: i18n.T("backend.definition.section.configuration.title", nil), Rows: []model.DefinitionSectionRow{}}
	var configDocs []topicConfigDoc
	var notes []string

	resourceConfigs, cfgErr := adm.DescribeTopicConfigs(ctx, topic)
	rc, onErr := resourceConfigs.On(topic, nil)
	switch {
	case cfgErr != nil, onErr != nil, rc.Err != nil:
		// A missing section must not fail the whole tab — the same degradation shape the
		// ACL-denied case already needs, just no longer the permanent state.
		notes = append(notes, "Topic configuration could not be read.")
	default:
		configs := append([]kadm.Config(nil), rc.Configs...)
		sort.Slice(configs, func(i, j int) bool { return configs[i].Key < configs[j].Key })
		configSection.Rows = make([]model.DefinitionSectionRow, len(configs))
		configDocs = make([]topicConfigDoc, len(configs))
		for i, c := range configs {
			isDefault := c.Source == kmsg.ConfigSourceDefaultConfig
			detail := c.Source.String()
			if isDefault {
				detail = "default"
			}
			value := c.MaybeValue()
			configSection.Rows[i] = model.DefinitionSectionRow{Name: c.Key, Value: value, Detail: &detail}
			configDocs[i] = topicConfigDoc{Name: c.Key, Value: value, Default: isDefault}
		}
	}

	doc := struct {
		Partitions []topicPartitionDoc `json:"partitions"`
		Config     []topicConfigDoc    `json:"config"`
	}{Partitions: partitionDocs, Config: configDocs}
	statement, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return model.ObjectDefinition{}, mapError(err)
	}

	return model.ObjectDefinition{
		Path:           model.EncodePath([]model.PathSegment{{Kind: "topic", Name: topic}}),
		Kind:           "topic",
		QualifiedName:  topic,
		Language:       "json",
		Statements:     []string{string(statement)},
		Origin:         "server",
		Notes:          notes,
		Constraints:    []model.ConstraintMeta{},
		DocumentSchema: nil,
		Sections:       []model.DefinitionSection{partitionsSection, configSection},
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

type groupMemberDoc struct {
	ClientID   string `json:"clientId"`
	ClientHost string `json:"clientHost"`
	MemberID   string `json:"memberId"`
}

type groupOffsetDoc struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// buildGroupDefinition is definition.ts's buildGroupDefinition (:99-195).
// kadm.DescribedGroup has no Type field and no PartitionAssignor field — the group definition
// drops its `type` row entirely (there is nothing in kadm to reverse-lookup a numeric enum from,
// unlike definition.ts:12-33's whole apparatus) and merges `partitionAssignor` into `protocol`,
// since kadm's own doc names Protocol "the partition assignor strategy this group is using" — the
// same value under a second name. State is already a string on this client, so the group section
// drops from seven rows to five: state, protocolType, protocol, coordinator, members.
func buildGroupDefinition(ctx context.Context, adm *kadm.Client, groupID string) (model.ObjectDefinition, error) {
	groups, err := adm.DescribeGroups(ctx, groupID)
	if err != nil {
		return model.ObjectDefinition{}, mapError(err)
	}
	group, ok := groups[groupID]
	if !ok || group.Err != nil {
		return model.ObjectDefinition{}, adapters.New(adapters.CodeNotFound, "consumer group not found: "+groupID, nil)
	}

	orEmDash := func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	}
	coordinator := fmt.Sprintf("%s:%d", group.Coordinator.Host, group.Coordinator.Port)
	groupSection := model.DefinitionSection{
		Title: i18n.T("backend.definition.section.group.title", nil),
		Rows: []model.DefinitionSectionRow{
			{Name: "state", Value: orEmDash(group.State), Detail: nil},
			{Name: "protocolType", Value: orEmDash(group.ProtocolType), Detail: nil},
			{Name: "protocol", Value: orEmDash(group.Protocol), Detail: nil},
			{Name: "coordinator", Value: coordinator, Detail: nil},
			{Name: "members", Value: strconv.Itoa(len(group.Members)), Detail: nil},
		},
	}

	membersSection := model.DefinitionSection{Title: i18n.T("backend.definition.section.members.title", nil), Rows: make([]model.DefinitionSectionRow, len(group.Members))}
	memberDocs := make([]groupMemberDoc, len(group.Members))
	for i, m := range group.Members {
		memberID := m.MemberID
		membersSection.Rows[i] = model.DefinitionSectionRow{Name: m.ClientID, Value: m.ClientHost, Detail: &memberID}
		memberDocs[i] = groupMemberDoc{ClientID: m.ClientID, ClientHost: m.ClientHost, MemberID: m.MemberID}
	}

	// A second, independent call — a group with read access but no offset-fetch permission still
	// shows its Group/Members sections rather than failing the whole load.
	offsetsSection := model.DefinitionSection{Title: i18n.T("backend.definition.section.committedOffsets.title", nil), Rows: []model.DefinitionSectionRow{}}
	var offsetDocs []groupOffsetDoc
	var notes []string
	if offsets, err := adm.FetchOffsets(ctx, groupID); err != nil {
		notes = append(notes, "Committed offsets could not be read.")
	} else {
		sorted := offsets.Sorted()
		offsetsSection.Rows = make([]model.DefinitionSectionRow, len(sorted))
		offsetDocs = make([]groupOffsetDoc, len(sorted))
		for i, o := range sorted {
			name := fmt.Sprintf("%s[%d]", o.Topic, o.Partition)
			value := strconv.FormatInt(o.At, 10)
			offsetsSection.Rows[i] = model.DefinitionSectionRow{Name: name, Value: value, Detail: nil}
			offsetDocs[i] = groupOffsetDoc{Name: name, Value: value}
		}
	}

	doc := struct {
		State        string `json:"state"`
		ProtocolType string `json:"protocolType"`
		Protocol     string `json:"protocol"`
		Coordinator  struct {
			Host string `json:"host"`
			Port int32  `json:"port"`
		} `json:"coordinator"`
		Members []groupMemberDoc `json:"members"`
		Offsets []groupOffsetDoc `json:"offsets"`
	}{
		State: group.State, ProtocolType: group.ProtocolType, Protocol: group.Protocol,
		Members: memberDocs, Offsets: offsetDocs,
	}
	doc.Coordinator.Host = group.Coordinator.Host
	doc.Coordinator.Port = group.Coordinator.Port
	statement, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return model.ObjectDefinition{}, mapError(err)
	}

	return model.ObjectDefinition{
		Path:           model.EncodePath([]model.PathSegment{{Kind: "consumerGroup", Name: groupID}}),
		Kind:           "consumerGroup",
		QualifiedName:  groupID,
		Language:       "json",
		Statements:     []string{string(statement)},
		Origin:         "server",
		Notes:          notes,
		Constraints:    []model.ConstraintMeta{},
		DocumentSchema: nil,
		Sections:       []model.DefinitionSection{groupSection, membersSection, offsetsSection},
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
