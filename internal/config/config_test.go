package config

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestSetDefaultConfig(t *testing.T) {
	Properties = Config{}
	SetDefaultConfig()

	checks := map[string]bool{
		"DefaultConfigFile":    Properties.DefaultConfigFile == ".updateGit.yaml",
		"Debug":                !Properties.Debug,
		"Git.BaseDir":          Properties.Git.BaseDir == "./git_repos",
		"Git.Parallel":         Properties.Git.Parallel,
		"Git.MaxConcurrent":    Properties.Git.MaxConcurrent == 10,
		"Git.Timeout":          Properties.Git.Timeout == 300,
		"Git.MaxDepth":         Properties.Git.MaxDepth == 2,
		"Git.ExcludeDirs":      len(Properties.Git.ExcludeDirs) == 4 && Properties.Git.ExcludeDirs[0] == "node_modules",
		"Git.SkipDirty":        Properties.Git.SkipDirty,
		"Backup.Enabled":       !Properties.Backup.Enabled,
		"Backup.Directory":     Properties.Backup.Directory == "./backups",
		"Backup.Strategy":      Properties.Backup.Strategy == "copy",
		"Backup.MaxConcurrent": Properties.Backup.MaxConcurrent == 4,
		"Filter.SkipRepos":     Properties.Filter.SkipRepos != nil && len(Properties.Filter.SkipRepos) == 0,
		"Log.Level":            Properties.Log.Level == "info",
		"Log.Color":            Properties.Log.Color == "auto",
		"Log.Progress":         Properties.Log.Progress == "auto",
		"Log.File.Enabled":     !Properties.Log.File.Enabled,
		"Log.File.Directory":   Properties.Log.File.Directory == "/tmp",
		"Log.File.Level":       Properties.Log.File.Level == "",
	}
	for field, ok := range checks {
		if !ok {
			t.Errorf("unexpected default value of %s: %+v", field, Properties)
		}
	}
}

func TestDefaultConfigIsValid(t *testing.T) {
	Properties = Config{}
	SetDefaultConfig()

	if err := validator.New().Struct(Properties); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}

func TestBackupStrategyValidation(t *testing.T) {
	tests := []struct {
		strategy string
		wantErr  bool
	}{
		{"copy", false},
		{"stash", false},
		{"", false}, // omitempty
		{"rsync", true},
		{"COPY", true},
	}

	validate := validator.New()
	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Backup.Strategy = tt.strategy
			err := validate.Struct(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("strategy %q: err = %v, wantErr %v", tt.strategy, err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*Config)
		wantErr bool
	}{
		{"max_concurrent 0", func(c *Config) { c.Git.MaxConcurrent = 0 }, true},
		{"max_concurrent 101", func(c *Config) { c.Git.MaxConcurrent = 101 }, true},
		{"max_concurrent 1", func(c *Config) { c.Git.MaxConcurrent = 1 }, false},
		{"max_concurrent 100", func(c *Config) { c.Git.MaxConcurrent = 100 }, false},
		{"timeout 0", func(c *Config) { c.Git.Timeout = 0 }, true},
		{"timeout 1", func(c *Config) { c.Git.Timeout = 1 }, false},
		{"log.level verbose", func(c *Config) { c.Log.Level = "verbose" }, true},
		{"log.level warning", func(c *Config) { c.Log.Level = "warning" }, false},
		{"log.color rainbow", func(c *Config) { c.Log.Color = "rainbow" }, true},
		{"log.color never", func(c *Config) { c.Log.Color = "never" }, false},
		{"log.progress maybe", func(c *Config) { c.Log.Progress = "maybe" }, true},
		{"log.file.level verbose", func(c *Config) { c.Log.File.Level = "verbose" }, true},
		{"log.file.level debug", func(c *Config) { c.Log.File.Level = "debug" }, false},
		{"log.file enabled without directory", func(c *Config) { c.Log.File.Enabled = true; c.Log.File.Directory = "" }, true},
		{"backup.max_concurrent 0", func(c *Config) { c.Backup.MaxConcurrent = 0 }, true},
		{"backup.max_concurrent 101", func(c *Config) { c.Backup.MaxConcurrent = 101 }, true},
	}
	validate := validator.New(validator.WithRequiredStructEnabled())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.change(&cfg)
			if err := validate.Struct(cfg); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaxDepthValidation(t *testing.T) {
	validate := validator.New()
	for depth, wantErr := range map[int]bool{0: true, 1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: true} {
		cfg := DefaultConfig()
		cfg.Git.MaxDepth = depth
		if err := validate.Struct(cfg); (err != nil) != wantErr {
			t.Errorf("max_depth %d: err = %v, wantErr %v", depth, err, wantErr)
		}
	}
	if DefaultConfig().Git.MaxDepth != 2 {
		t.Error("the default max_depth must be 2")
	}
}

func TestCLIVersionIsSemver(t *testing.T) {
	if !SemverRegex.MatchString(CLIVersion) {
		t.Errorf("CLIVersion %q is not a semantic version without 'v' prefix", CLIVersion)
	}
	for version, valid := range map[string]bool{
		"0.2.0": true, "1.0.0": true, "0.3.0-rc.1": true,
		"v0.2.0": false, "0.2": false, "01.2.3": false, "0.2.0-": false,
	} {
		if SemverRegex.MatchString(version) != valid {
			t.Errorf("SemverRegex(%q) = %v, want %v", version, !valid, valid)
		}
	}
}

func TestNoUnderscores(t *testing.T) {
	type sample struct {
		Name string `validate:"nounderscores"`
	}

	validate := validator.New()
	if err := validate.RegisterValidation("nounderscores", NoUnderscores); err != nil {
		t.Fatalf("RegisterValidation: %v", err)
	}

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"my-repo", false},
		{"myrepo", false},
		{"my_repo", true},
		{"_", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := validate.Struct(sample{Name: tt.value})
			if (err != nil) != tt.wantErr {
				t.Errorf("value %q: err = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}
