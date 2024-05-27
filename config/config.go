// Copyright 2024 SAP SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hashicorp/vault/api"
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
	DB       int    `yaml:"db"`
}
type Vault struct {
	Protocol string `yaml:"protocol"`
	Host     string `yaml:"host"`
	Address  string `yaml:"address"`
	Path     string `yaml:"path"`
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

func NewConfig(ctx context.Context, vaultToken string) (*Config, error) {
	c := &Config{}
	f, err := os.Open(configFile)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to open configuration file: %w", err)
	}

	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&c); err != nil {
		f.Close()
		return nil, fmt.Errorf("unable to decode config file: %w", err)
	}

	addr := fmt.Sprintf("%s://%s:%d", c.Services.Vault.Protocol, c.Services.Vault.Address, c.Services.Vault.Port)
	// Create a client to interact with Vault
	config := &api.Config{
		Address: addr,
	}

	client, err := api.NewClient(config)
	if err != nil {
		f.Close()
		log.Fatalf("Failed to create Vault client: %v", err)
	}

	// Set the Vault token
	client.SetToken(vaultToken)

	// Verify the client is authenticated
	_, err = client.Auth().Token().LookupSelf()
	if err != nil {
		f.Close()
		log.Fatalf("Failed to authenticate to Vault: %v", err)
	}
	c.Services.Redis.User = mustReadFromVault(client, c.Services.Vault.Path, "redis_user")

	c.Services.Redis.Password = mustReadFromVault(client, c.Services.Vault.Path, "redis_password")

	c.Services.Kafka.User = mustReadFromVault(client, c.Services.Vault.Path, "kafka_user")

	c.Services.Kafka.Password = mustReadFromVault(client, c.Services.Vault.Path, "kafka_password")

	f.Close()
	return c, nil
}

func mustReadFromVault(client *api.Client, path, key string) string {
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
