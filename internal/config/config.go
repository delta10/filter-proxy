package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Backend struct {
	Type    string `yaml:"type"`
	BaseURL string `yaml:"baseUrl"`

	Auth struct {
		Header map[string]string `yaml:"header"`
		Basic  struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"basic"`
		TLS struct {
			RootCertificates string `yaml:"rootCertificates"`
			Certificate      string `yaml:"certificate"`
			Key              string `yaml:"key"`
		} `yaml:"tls"`
	}
}

type Path struct {
	Path           string   `yaml:"path"`
	AllowedMethods []string `yaml:"allowedMethods"`
	Passthrough    bool     `yaml:"passthrough"`
	Backend        struct {
		Slug string `yaml:"slug"`
		Path string `yaml:"path"`
	} `yaml:"backend"`
	RequestRewrite  string `yaml:"requestRewrite"`
	ResponseRewrite string `yaml:"responseRewrite"`
}

type Config struct {
	ListenAddress string `yaml:"listenAddress"`
	ListenTLS     struct {
		Certificate string `yaml:"certificate"`
		Key         string `yaml:"key"`
	} `yaml:"listenTls"`
	AuthorizationServiceURL string             `yaml:"authorizationServiceUrl"`
	JwksURL                 string             `yaml:"jwksUrl"`
	Paths                   []Path             `yaml:"paths"`
	Backends                map[string]Backend `yaml:"backends"`
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
