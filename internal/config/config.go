package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const FileName = "pantograph.yaml"

type Config struct {
	Kinds   map[string]string `yaml:"kinds"`
	Domains map[string]Domain `yaml:"domains"`
}

type Domain struct {
	Flows []string `yaml:"flows"`
	Note  string   `yaml:"note"`
}

func (d *Domain) UnmarshalYAML(value *yaml.Node) error {
	var flows []string
	if err := value.Decode(&flows); err == nil {
		d.Flows = flows

		return nil
	}

	type raw Domain

	if err := value.Decode((*raw)(d)); err != nil {
		return fmt.Errorf("decode domain: %w", err)
	}

	return nil
}

//nolint:gochecknoglobals // the zero-config fallback vocabulary
var defaultKinds = map[string]string{
	"event":    "page",
	"decision": "diamond",
	"store":    "cylinder",
	"process":  "",
	"backstop": "",
}

func Load(root string) (*Config, error) {
	path := filepath.Join(root, FileName)

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Kinds: defaultKinds}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read %s: %w", FileName, err)
	}

	var c Config

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", FileName, err)
	}

	if len(c.Kinds) == 0 {
		c.Kinds = defaultKinds
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Config) validate() error {
	for name, shape := range c.Kinds {
		if name == "" {
			return fmt.Errorf("%s: kinds has an empty kind name", FileName)
		}

		if !validShape(shape) {
			return fmt.Errorf("%s: kind %q has unknown shape %q (want one of %s)",
				FileName, name, shape, strings.Join(shapeNames(), ", "))
		}
	}

	for name, d := range c.Domains {
		if len(d.Flows) == 0 {
			return fmt.Errorf("%s: domain %q lists no flows", FileName, name)
		}
	}

	return nil
}

func (c *Config) KnownKind(k string) bool {
	if k == "" {
		return true
	}

	_, ok := c.Kinds[k]

	return ok
}

type DomainDecl struct {
	Domain string
	Flows  []string
	Note   string
}

func (c *Config) DomainDecls() []DomainDecl {
	names := make([]string, 0, len(c.Domains))
	for name := range c.Domains {
		names = append(names, name)
	}

	sort.Strings(names)

	decls := make([]DomainDecl, 0, len(names))
	for _, name := range names {
		d := c.Domains[name]
		decls = append(decls, DomainDecl{Domain: name, Flows: d.Flows, Note: d.Note})
	}

	return decls
}
