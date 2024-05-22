package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Services struct {
		Collector struct {
			VaultPath string `yaml:"vault-path"`
			Interval  string `yaml:"interval"`
		} `yaml:"collector"`
		Kafka struct {
			Host  string `yaml:"host"`
			Port  string `yaml:"port"`
			Topic string `yaml:"topic"`
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
		} `yaml:"vault"`
	}
}

func NewConfig(path string) (*Config, error) {
	config := &Config{}
	f, err := os.Open("configs/" + path + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to open confiuration file: %w", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)

	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("unable to decode config file: %w", err)
	}
	return config, nil
}

func (c *Config) FetchVaultData(vaultToken string) error {
	vaultAddr := fmt.Sprintf("%s://%s:%s", c.Services.Vault.Protocol, c.Services.Vault.Port, c.Services.Vault.Port)
	ctx := context.Background()
	client, err := vault.New(
		vault.WithAddress(vaultAddr),
		vault.WithRequestTimeout(30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("unable to access vault address: %w", err)
	}

	if err := client.SetToken(vaultToken); err != nil {
		return fmt.Errorf("failed to auth with vault token: %w", err)
	}

	if err := client.SetNamespace(c.Services.Collector.VaultPath); err != nil {
		return fmt.Errorf("failed to set namespace: %w", err)
	}

	ru, err := getSecret(ctx, c.Services.Redis.User, client)
	if err != nil {
		return fmt.Errorf("failed to fetch vault data: %w", err)
	}
	c.Services.Redis.User = ru

	rp, err := getSecret(ctx, c.Services.Redis.Password, client)
	if err != nil {
		return fmt.Errorf("failed to fetch vault data: %w", err)
	}
	c.Services.Redis.User = rp

	// ku, err := getSecret(ctx, c.Services.Kafka.User, client)
	// if err != nil {
	// 	return fmt.Errorf("failed to fetch vault data: %w", err)
	// }
	// c.Services.Redis.User = ku
	//
	// kp, err := getSecret(ctx, c.Services.Kafka.Password, client)
	// if err != nil {
	// 	return fmt.Errorf("failed to fetch vault data: %w", err)
	// }
	// c.Services.Redis.User = kp

	return nil
}

func getSecret(ctx context.Context, key string, client *vault.Client) (string, error) {
	rr, err := client.Secrets.KvV2Read(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to grab redis user: %w", err)
	}

	val, err := extractString(key, rr.Data)
	if err != nil {
		return "", fmt.Errorf("unable to get secret for key: %w", err)
	}
	return val, nil
}
func extractString(key string, rr schema.KvV2ReadResponse) (string, error) {
	x, ok := rr.Data[key]
	if !ok || x == nil {
		return "", errors.New("failed to get password for redis user")
	}
	return x.(string), nil

}
