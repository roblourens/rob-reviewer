package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion         = 1
	DefaultModel           = "gpt-5.6-sol"
	DefaultStateBranch     = "reviewer-state"
	DefaultStatePath       = ".rob-reviewer/state.json"
	DefaultMaxPerRun       = 5
	DefaultMaxPerDay       = 25
	DefaultQuietMinutes    = 5
	DefaultScanHours       = 168
	DefaultMaxPendingHours = 24
	DefaultMaxFindings     = 10
	DefaultMinConfidence   = 0.85
)

type Config struct {
	Version     int               `yaml:"version"`
	Target      TargetConfig      `yaml:"target"`
	Poll        PollConfig        `yaml:"poll"`
	Automation  AutomationConfig  `yaml:"automation"`
	Publication PublicationConfig `yaml:"publication"`
	Review      ReviewConfig      `yaml:"review"`
	State       StateConfig       `yaml:"state"`
}

type TargetConfig struct {
	Owner string `yaml:"owner"`
	Repo  string `yaml:"repo"`
}

type PollConfig struct {
	MaxPerRun       int `yaml:"maxPerRun"`
	MaxPerDay       int `yaml:"maxPerDay"`
	QuietMinutes    int `yaml:"quietPeriodMinutes"`
	ScanWindowHours int `yaml:"scanWindowHours"`
	MaxPendingHours int `yaml:"maxPendingAgeHours"`
}

type AutomationConfig struct {
	Enabled    bool     `yaml:"enabled"`
	SkipLabels []string `yaml:"skipLabels"`
}

type PublicationConfig struct {
	Mode string `yaml:"mode"`
}

type ReviewConfig struct {
	Model           string   `yaml:"model"`
	ReasoningEffort string   `yaml:"reasoningEffort"`
	MinConfidence   float64  `yaml:"minConfidence"`
	MaxFindings     int      `yaml:"maxFindings"`
	Focuses         []string `yaml:"focuses"`
}

type StateConfig struct {
	Branch string `yaml:"branch"`
	Path   string `yaml:"path"`
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	cfg, err := Decode(file)
	if err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}

	if model := strings.TrimSpace(os.Getenv("COPILOT_MODEL")); model != "" {
		cfg.Review.Model = model
	}

	return cfg, nil
}

func Decode(reader io.Reader) (Config, error) {
	cfg := Config{
		Version: CurrentVersion,
		Poll: PollConfig{
			MaxPerRun:       DefaultMaxPerRun,
			MaxPerDay:       DefaultMaxPerDay,
			QuietMinutes:    DefaultQuietMinutes,
			ScanWindowHours: DefaultScanHours,
			MaxPendingHours: DefaultMaxPendingHours,
		},
		Automation:  AutomationConfig{SkipLabels: []string{"performance-reviewer:skip"}},
		Publication: PublicationConfig{Mode: "approval"},
		Review: ReviewConfig{
			Model:           DefaultModel,
			ReasoningEffort: "high",
			MinConfidence:   DefaultMinConfidence,
			MaxFindings:     DefaultMaxFindings,
		},
		State: StateConfig{
			Branch: DefaultStateBranch,
			Path:   DefaultStatePath,
		},
	}

	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	var validationErrors []error
	if cfg.Version != CurrentVersion {
		validationErrors = append(validationErrors, fmt.Errorf("version must be %d", CurrentVersion))
	}
	if strings.TrimSpace(cfg.Target.Owner) == "" {
		validationErrors = append(validationErrors, errors.New("target.owner is required"))
	}
	if strings.TrimSpace(cfg.Target.Repo) == "" {
		validationErrors = append(validationErrors, errors.New("target.repo is required"))
	}
	if cfg.Poll.MaxPerRun < 1 {
		validationErrors = append(validationErrors, errors.New("poll.maxPerRun must be at least 1"))
	}
	if cfg.Poll.MaxPerDay < cfg.Poll.MaxPerRun {
		validationErrors = append(validationErrors, errors.New("poll.maxPerDay must be at least poll.maxPerRun"))
	}
	if cfg.Poll.QuietMinutes < 1 {
		validationErrors = append(validationErrors, errors.New("poll.quietPeriodMinutes must be at least 1"))
	}
	if cfg.Poll.ScanWindowHours < 1 || cfg.Poll.ScanWindowHours*60 < cfg.Poll.QuietMinutes {
		validationErrors = append(validationErrors, errors.New("poll.scanWindowHours must cover the quiet period"))
	}
	if cfg.Poll.MaxPendingHours < 1 || cfg.Poll.MaxPendingHours*60 < cfg.Poll.QuietMinutes {
		validationErrors = append(validationErrors, errors.New("poll.maxPendingAgeHours must cover the quiet period"))
	}
	seenSkipLabels := make(map[string]struct{}, len(cfg.Automation.SkipLabels))
	for _, label := range cfg.Automation.SkipLabels {
		label = strings.TrimSpace(label)
		if label == "" {
			validationErrors = append(validationErrors, errors.New("automation.skipLabels cannot contain an empty label"))
			continue
		}
		if _, exists := seenSkipLabels[label]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("automation.skipLabels contains duplicate %q", label))
		}
		seenSkipLabels[label] = struct{}{}
	}
	if !slices.Contains([]string{"approval", "automatic"}, cfg.Publication.Mode) {
		validationErrors = append(validationErrors, errors.New("publication.mode must be approval or automatic"))
	}
	if strings.TrimSpace(cfg.Review.Model) == "" {
		validationErrors = append(validationErrors, errors.New("review.model is required"))
	}
	if !slices.Contains([]string{"low", "medium", "high", "xhigh", "max"}, cfg.Review.ReasoningEffort) {
		validationErrors = append(validationErrors, errors.New("review.reasoningEffort must be low, medium, high, xhigh, or max"))
	}
	if cfg.Review.MinConfidence < 0 || cfg.Review.MinConfidence > 1 {
		validationErrors = append(validationErrors, errors.New("review.minConfidence must be between 0 and 1"))
	}
	if cfg.Review.MaxFindings < 1 || cfg.Review.MaxFindings > 10 {
		validationErrors = append(validationErrors, errors.New("review.maxFindings must be between 1 and 10"))
	}
	if len(cfg.Review.Focuses) == 0 {
		validationErrors = append(validationErrors, errors.New("review.focuses must contain at least one focus"))
	}
	seenFocuses := make(map[string]struct{}, len(cfg.Review.Focuses))
	for _, focus := range cfg.Review.Focuses {
		focus = strings.TrimSpace(focus)
		if focus == "" {
			validationErrors = append(validationErrors, errors.New("review.focuses cannot contain an empty focus"))
			continue
		}
		if _, exists := seenFocuses[focus]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("review.focuses contains duplicate %q", focus))
		}
		seenFocuses[focus] = struct{}{}
	}
	if strings.TrimSpace(cfg.State.Branch) == "" {
		validationErrors = append(validationErrors, errors.New("state.branch is required"))
	}
	if strings.TrimSpace(cfg.State.Path) == "" {
		validationErrors = append(validationErrors, errors.New("state.path is required"))
	}
	return errors.Join(validationErrors...)
}
