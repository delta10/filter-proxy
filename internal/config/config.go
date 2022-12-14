package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Backend struct {
	URL            string `yaml:"url"`
	TLSCertificate string `yaml:"tlsCertificate"`
	TLSKey         string `yaml:"tlsKey"`
}

type Authorization struct {
	Groups []string `yaml:"groups"`
}

type Path struct {
	Path          string        `yaml:"path"`
	Backend       Backend       `yaml:"backend"`
	Filter        string        `yaml:"filter"`
	Authorization Authorization `yaml:"authorization"`
}

type Config struct {
	ListenAddress string `yaml:"listenAddress"`
	JwksUrl       string `yaml:"jwksUrl"`
	Paths         []Path `yaml:"paths"`
}

// NewConfig returns a new decoded Config struct
func NewConfig(configPath string) (*Config, error) {
	// Create config structure
	config := &Config{}

	// Open config file
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Init new YAML decode
	d := yaml.NewDecoder(file)

	// Start YAML decoding from file
	if err := d.Decode(&config); err != nil {
		return nil, err
	}

	return config, nil
}
