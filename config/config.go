package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hashicorp/vault/api"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v2"
)

const configFile = "../../configs/local-development.yaml"

type Config struct {
	Services Services `yaml:"services"`
}
type CollectionUrls struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}
type Collector struct {
	VaultPath      string           `yaml:"vault-path"`
	Interval       string           `yaml:"interval"`
	CollectionUrls []CollectionUrls `yaml:"collection-urls"`
}
type Kafka struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Topic    string `yaml:"topic"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}
type Redis struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Db       int    `yaml:"db"`
}
type Vault struct {
	Protocol string `yaml:"protocol"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
}
type Jaeger struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}
type Services struct {
	Collector Collector `yaml:"collector"`
	Kafka     Kafka     `yaml:"kafka"`
	Redis     Redis     `yaml:"redis"`
	Vault     Vault     `yaml:"vault"`
	Jaeger    Jaeger    `yaml:"jaeger"`
}

var configKey = attribute.Key("config/config.go")

func NewConfig(ctx context.Context, vaultToken string) (*Config, error) {

	c := &Config{}
	f, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file: %w", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("unable to decode config file: %w", err)
	}

	// Vault connection details
	vaultAddress := "http://192.168.106.2:8200" // Replace with your Vault URL
	vaultToken = "root"                         // Replace with your Vault token
	path := "secret/data/stormsync/development"

	// Create a client to interact with Vault
	config := &api.Config{
		Address: vaultAddress,
	}

	client, err := api.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create Vault client: %v", err)
	}

	// Set the Vault token
	client.SetToken(vaultToken)

	// Verify the client is authenticated
	_, err = client.Auth().Token().LookupSelf()
	if err != nil {
		log.Fatalf("Failed to authenticate to Vault: %v", err)
	}
	c.Services.Redis.User = mustReadFromVault(ctx, client, path, "redis_user")

	c.Services.Redis.Password = mustReadFromVault(ctx, client, path, "redis_password")

	c.Services.Kafka.User = mustReadFromVault(ctx, client, path, "kafka_user")

	c.Services.Kafka.Password = mustReadFromVault(ctx, client, path, "kafka_password")

	return c, nil
}

func mustReadFromVault(ctx context.Context, client *api.Client, path, key string) string {
	secret, err := client.Logical().Read(path)
	if err != nil {
		log.Fatalf("failed to read data from %s: %s", path, err)
	}

	if secret == nil || secret.Data["data"] == nil {
		log.Fatal("no data found at", path)
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {

		log.Fatal("data format error at ", path)
	}

	value, ok := data[key].(string)
	if !ok {
		log.Fatalf("key %s not found at %s ", key, path)
	}

	return value
}
