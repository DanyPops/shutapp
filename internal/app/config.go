package app

import (
	"fmt"
	"os"
	"strings"

	"go.mau.fi/whatsmeow/types"
	"gopkg.in/yaml.v3"
)

type TargetConfig struct {
	Group string `yaml:"group"`
	Phone string `yaml:"phone"`
}

func LoadTargetConfig(path string) (*TargetConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg TargetConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &cfg, nil
}

func PhoneToUserJID(phone string) types.JID {
	phone = strings.TrimPrefix(phone, "+")
	return types.JID{User: phone, Server: types.DefaultUserServer} // @c.us
}
