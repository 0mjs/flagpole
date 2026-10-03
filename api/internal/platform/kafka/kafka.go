// Package kafka names Flagpole's topics and creates clients for them.
package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	// TopicFlagChanges carries one event per applied flag config change,
	// keyed by environment so an environment's changes stay in order.
	TopicFlagChanges = "flagpole.flag-changes"
	// TopicExposures carries batches of SDK exposures, keyed by environment.
	TopicExposures = "flagpole.exposures"
)

// NewClient returns a producer, or a consumer when opts add a group.
func NewClient(brokers []string, opts ...kgo.Opt) (*kgo.Client, error) {
	client, err := kgo.NewClient(append([]kgo.Opt{kgo.SeedBrokers(brokers...)}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("kafka: %w", err)
	}
	return client, nil
}

// EnsureTopics creates the topics if they don't exist.
func EnsureTopics(ctx context.Context, client *kgo.Client) error {
	admin := kadm.NewClient(client)
	resp, err := admin.CreateTopics(ctx, 3, 1, nil, TopicFlagChanges, TopicExposures)
	if err != nil {
		return fmt.Errorf("kafka: create topics: %w", err)
	}
	for _, t := range resp.Sorted() {
		if t.Err != nil && t.ErrMessage != "" && !isExists(t.Err) {
			return fmt.Errorf("kafka: create topic %s: %w", t.Topic, t.Err)
		}
	}
	return nil
}

func isExists(err error) bool {
	return err != nil && err.Error() == "TOPIC_ALREADY_EXISTS: Topic with this name already exists."
}
