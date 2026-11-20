package kafka

import (
	"sort"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type ConfigEntry struct {
	Name      string  `json:"name"`
	Value     *string `json:"value"`
	Source    string  `json:"source"`
	IsDefault bool    `json:"isDefault"`
	Sensitive bool    `json:"sensitive"`
}

func toConfigEntries(cs []kadm.Config) []ConfigEntry {
	out := make([]ConfigEntry, 0, len(cs))
	for _, c := range cs {
		out = append(out, ConfigEntry{
			Name:      c.Key,
			Value:     c.Value,
			Source:    configSource(c.Source),
			IsDefault: c.Source == kmsg.ConfigSourceDefaultConfig,
			Sensitive: c.Sensitive,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func configSource(s kmsg.ConfigSource) string {
	switch s {
	case kmsg.ConfigSourceDynamicTopicConfig:
		return "TOPIC"
	case kmsg.ConfigSourceDynamicBrokerConfig:
		return "DYNAMIC_BROKER"
	case kmsg.ConfigSourceDynamicDefaultBrokerConfig:
		return "DYNAMIC_DEFAULT_BROKER"
	case kmsg.ConfigSourceStaticBrokerConfig:
		return "STATIC_BROKER"
	case kmsg.ConfigSourceDefaultConfig:
		return "DEFAULT"
	case kmsg.ConfigSourceDynamicBrokerLoggerConfig:
		return "BROKER_LOGGER"
	default:
		return "UNKNOWN"
	}
}
