package config

import (
	"context"
	"errors"
	"fmt"
	"os"

	vault "github.com/hashicorp/vault/api"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Services struct {
		Collector struct {
			VaultPath      string   `yaml:"vault-path"`
			Interval       string   `yaml:"interval"`
			CollectionURLs []string `yaml:"collection-urls"`
		} `yaml:"collector"`
		Kafka struct {
			Host     string `yaml:"host"`
			Port     string `yaml:"port"`
			Topic    string `yaml:"topic"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
		} `yaml:"kafka"`
		Redis struct {
			Host     string `yaml:"host"`
			Port     string `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			DB       int    `yaml:db"`
		} `yaml:"redis"`
		Vault struct {
			Host     string `yaml:"host"`
			Port     string `yaml:"port"`
			Protocol string `yaml:"protocol"`
			Path     string `yaml:"path"`
		} `yaml:"vault"`
	}
}

func NewConfig(ctx context.Context, vaultToken string) (*Config, error) {

	c := &Config{}

	// TODO remoge this hack
	var fn string
	if os.Getenv("ENV") != "dev" {
		fn = "/etc/collector-config.yml"
	} else {
		fn = "../../configs/upstash-development.yaml"
	}
	f, err := os.Open(fn)

	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file: %w", err)
	}

	defer f.Close()

	dec := yaml.NewDecoder(f)

	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("unable to decode config file: %w", err)
	}

	vaultAddr := fmt.Sprintf("%s://%s:%s", c.Services.Vault.Protocol, c.Services.Vault.Host, c.Services.Vault.Port)

	vConfig := vault.DefaultConfig()
	vConfig.Address = vaultAddr
	client, err := vault.NewClient(vConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to access vault address: %w", err)
	}

	client.SetToken(vaultToken)

	secret, err := client.KVv2("secret").Get(ctx, "stormsync/collector/upstash-development")
	if err != nil {
		return nil, fmt.Errorf("Unable to read the secret from  vault: %w", err)
	}

	ku, ok := secret.Data["kafka-user"].(string)
	if !ok {
		return nil, errors.New("unable to read the secret for the key value")
	}
	c.Services.Kafka.User = ku

	kp, ok := secret.Data["kafka-password"].(string)
	if !ok {
		return nil, errors.New("unable to read the secret for the key value")
	}
	c.Services.Kafka.Password = kp

	ru, ok := secret.Data["redis-user"].(string)
	if !ok {
		return nil, errors.New("unable to read the secret for the key value")
	}
	c.Services.Redis.User = ru
	rp, ok := secret.Data["redis-password"].(string)
	if !ok {
		return nil, errors.New("unable to read the secret for the key value")
	}
	c.Services.Redis.Password = rp

	return c, nil
}
