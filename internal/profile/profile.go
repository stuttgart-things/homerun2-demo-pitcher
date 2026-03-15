package profile

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile defines a message generation profile loaded from YAML.
type Profile struct {
	Name        string                      `yaml:"name"`
	Description string                      `yaml:"description"`
	Severities  map[string]int              `yaml:"severities"`
	Systems     []string                    `yaml:"systems"`
	Authors     []string                    `yaml:"authors"`
	Tags        []string                    `yaml:"tags"`
	Assignees   []Assignee                  `yaml:"assignees"`
	URLs        []string                    `yaml:"urls"`
	Artifacts   []string                    `yaml:"artifacts"`
	Templates   map[string]SeverityTemplate `yaml:"templates"`
}

// SeverityTemplate holds title and message templates for a given severity.
type SeverityTemplate struct {
	Titles   []string `yaml:"titles"`
	Messages []string `yaml:"messages"`
}

// Assignee represents a person who can be assigned to a message.
type Assignee struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

// LoadFromFile loads a single profile from a YAML file.
func LoadFromFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile file %s: %w", path, err)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile file %s: %w", path, err)
	}

	return &p, nil
}

// LoadFromDir loads all .yaml and .yml files from a directory, returning
// a map keyed by profile name.
func LoadFromDir(dir string) (map[string]*Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading profile directory %s: %w", dir, err)
	}

	profiles := make(map[string]*Profile)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		p, err := LoadFromFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("invalid profile %s: %w", entry.Name(), err)
		}

		profiles[p.Name] = p
	}

	return profiles, nil
}

// Validate checks that a profile has the minimum required fields.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if len(p.Severities) == 0 {
		return fmt.Errorf("profile %q must define at least one severity", p.Name)
	}
	if len(p.Systems) == 0 {
		return fmt.Errorf("profile %q must define at least one system", p.Name)
	}
	if len(p.Authors) == 0 {
		return fmt.Errorf("profile %q must define at least one author", p.Name)
	}
	return nil
}

// WeightedSeverity returns a random severity based on the configured weights.
func (p *Profile) WeightedSeverity() string {
	totalWeight := 0
	for _, w := range p.Severities {
		totalWeight += w
	}
	if totalWeight == 0 {
		// fallback: return any key
		for sev := range p.Severities {
			return sev
		}
		return ""
	}

	r := rand.Intn(totalWeight)
	for sev, w := range p.Severities {
		r -= w
		if r < 0 {
			return sev
		}
	}

	// should not reach here, but return first key as fallback
	for sev := range p.Severities {
		return sev
	}
	return ""
}

// DefaultProfile returns a built-in default profile for generic
// monitoring/ops message generation.
func DefaultProfile() *Profile {
	return &Profile{
		Name:        "default",
		Description: "Generic monitoring and ops message profile",
		Severities: map[string]int{
			"error":   10,
			"warning": 20,
			"info":    50,
			"success": 10,
			"debug":   10,
		},
		Systems: []string{
			"k8s-cluster",
			"prometheus",
			"argocd",
			"vault",
			"redis",
		},
		Authors: []string{
			"ops-bot",
			"monitoring",
			"ci-pipeline",
		},
		Tags: []string{
			"infrastructure",
			"deployment",
			"monitoring",
			"security",
			"performance",
		},
		Assignees: []Assignee{
			{Name: "oncall-primary", Address: "oncall@example.com"},
			{Name: "platform-team", Address: "platform@example.com"},
		},
		URLs: []string{
			"https://grafana.example.com/d/{{.System}}",
			"https://argocd.example.com/applications/{{.System}}",
		},
		Artifacts: []string{
			"logs/{{.System}}/{{.Timestamp}}.log",
			"metrics/{{.System}}/snapshot.json",
		},
		Templates: map[string]SeverityTemplate{
			"error": {
				Titles: []string{
					"[ERROR] {{.System}} health check failed",
					"[ERROR] {{.System}} pod crash loop detected",
					"[ERROR] {{.System}} connection timeout",
				},
				Messages: []string{
					"Health check for {{.System}} has been failing for the last 5 minutes. Immediate attention required.",
					"Multiple pods in {{.System}} are in CrashLoopBackOff state. Last restart: {{.Timestamp}}.",
					"Connection to {{.System}} timed out after 30s. Upstream may be unreachable.",
				},
			},
			"warning": {
				Titles: []string{
					"[WARN] {{.System}} high memory usage",
					"[WARN] {{.System}} certificate expiring soon",
					"[WARN] {{.System}} disk usage above 80%",
				},
				Messages: []string{
					"Memory usage on {{.System}} has exceeded 85% threshold. Current: {{.Value}}%.",
					"TLS certificate for {{.System}} expires in 7 days. Renewal recommended.",
					"Disk usage on {{.System}} is at {{.Value}}%. Consider cleanup or expansion.",
				},
			},
			"info": {
				Titles: []string{
					"[INFO] {{.System}} deployment completed",
					"[INFO] {{.System}} scaling event",
					"[INFO] {{.System}} configuration updated",
					"[INFO] {{.System}} backup completed",
				},
				Messages: []string{
					"Deployment to {{.System}} completed successfully. Version: {{.Version}}.",
					"{{.System}} scaled from {{.OldReplicas}} to {{.NewReplicas}} replicas.",
					"Configuration for {{.System}} has been updated by {{.Author}}.",
					"Scheduled backup for {{.System}} completed. Size: {{.Size}}.",
				},
			},
			"success": {
				Titles: []string{
					"[OK] {{.System}} recovery confirmed",
					"[OK] {{.System}} pipeline passed",
				},
				Messages: []string{
					"{{.System}} has recovered and all health checks are passing.",
					"CI/CD pipeline for {{.System}} completed successfully. All tests passed.",
				},
			},
			"debug": {
				Titles: []string{
					"[DEBUG] {{.System}} trace collected",
					"[DEBUG] {{.System}} cache statistics",
				},
				Messages: []string{
					"Trace data collected from {{.System}}. Span count: {{.SpanCount}}.",
					"Cache hit ratio for {{.System}}: {{.HitRatio}}%. Evictions: {{.Evictions}}.",
				},
			},
		},
	}
}
