package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Backend struct {
	BaseURL string `yaml:"baseUrl"`

	Auth struct {
		Header map[string]string `yaml:"header"`
		Basic  struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"basic"`
		TLS struct {
			Certificate string `yaml:"certificate"`
			Key         string `yaml:"key"`
		} `yaml:"tls"`
	}
}

type Path struct {
	Path    string `yaml:"path"`
	Backend struct {
		Slug string `yaml:"slug"`
		Path string `yaml:"path"`
	} `yaml:"backend"`
	LogBackend string `yaml:"logBackend"`
	Filter     string `yaml:"filter"`
}

type LogBackend struct {
	BaseURL string `yaml:"baseUrl"`
}

type Config struct {
	ListenAddress           string                `yaml:"listenAddress"`
	AuthorizationServiceURL string                `yaml:"authorizationServiceUrl"`
	JwksURL                 string                `yaml:"jwksUrl"`
	Paths                   []Path                `yaml:"paths"`
	Backends                map[string]Backend    `yaml:"backends"`
	LogBackends             map[string]LogBackend `yaml:"logBackends"`
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
