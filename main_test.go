package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantArgs    []string
		wantHelp    bool
		wantVerbose bool
		wantProfile string
	}{
		{
			name:     "no flags",
			args:     []string{"--", "echo"},
			wantArgs: []string{"--", "echo"},
		},
		{
			name:        "-v before separator",
			args:        []string{"-v", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantVerbose: true,
		},
		{
			name:        "--verbose before separator",
			args:        []string{"--verbose", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantVerbose: true,
		},
		{
			name:     "-v after separator kept",
			args:     []string{"--", "cmd", "-v"},
			wantArgs: []string{"--", "cmd", "-v"},
		},
		{
			name:        "-v with subcommand",
			args:        []string{"-v", "set", "secret/path"},
			wantArgs:    []string{"set", "secret/path"},
			wantVerbose: true,
		},
		{
			name:        "-v after envfile",
			args:        []string{"custom.env", "-v", "--", "echo"},
			wantArgs:    []string{"custom.env", "--", "echo"},
			wantVerbose: true,
		},
		{
			name:     "-h",
			args:     []string{"-h"},
			wantHelp: true,
		},
		{
			name:     "--help",
			args:     []string{"--help"},
			wantHelp: true,
		},
		{
			name:     "-h after subcommand",
			args:     []string{"set", "-h"},
			wantArgs: []string{"set"},
			wantHelp: true,
		},
		{
			name:     "--help after separator ignored",
			args:     []string{"custom.env", "--", "--help"},
			wantArgs: []string{"custom.env", "--", "--help"},
		},
		{
			name:        "combined flags",
			args:        []string{"-v", "--help"},
			wantHelp:    true,
			wantVerbose: true,
		},
		{
			name: "no args",
			args: []string{},
		},
		{
			name:        "--profile=name",
			args:        []string{"--profile=staging", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantProfile: "staging",
		},
		{
			name:        "--profile name",
			args:        []string{"--profile", "staging", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantProfile: "staging",
		},
		{
			name:        "-p=name",
			args:        []string{"-p=staging", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantProfile: "staging",
		},
		{
			name:        "-p name",
			args:        []string{"-p", "staging", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantProfile: "staging",
		},
		{
			name:        "-p with other flags",
			args:        []string{"-v", "-p", "prod", "--", "echo"},
			wantArgs:    []string{"--", "echo"},
			wantVerbose: true,
			wantProfile: "prod",
		},
		{
			name:     "-p after separator kept",
			args:     []string{"--", "cmd", "-p", "val"},
			wantArgs: []string{"--", "cmd", "-p", "val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, got := parseFlags(tt.args)

			if f.Help != tt.wantHelp {
				t.Errorf("Help = %v, want %v", f.Help, tt.wantHelp)
			}

			if f.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v, want %v", f.Verbose, tt.wantVerbose)
			}

			if f.Profile != tt.wantProfile {
				t.Errorf("Profile = %q, want %q", f.Profile, tt.wantProfile)
			}

			if len(got) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", got, tt.wantArgs)
			}

			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		envFile string
		cmdArgs []string
		wantErr bool
	}{
		{
			name:    "default env file",
			args:    []string{"--", "echo", "hello"},
			envFile: "ttrun.env",
			cmdArgs: []string{"echo", "hello"},
		},
		{
			name:    "custom env file",
			args:    []string{"custom.env", "--", "echo"},
			envFile: "custom.env",
			cmdArgs: []string{"echo"},
		},
		{
			name:    "missing separator",
			args:    []string{"echo", "hello"},
			wantErr: true,
		},
		{
			name:    "no command after separator",
			args:    []string{"--"},
			wantErr: true,
		},
		{
			name:    "too many args before separator",
			args:    []string{"a", "b", "--", "echo"},
			wantErr: true,
		},
		{
			name:    "empty args",
			args:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envFile, cmdArgs, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if envFile != tt.envFile {
				t.Errorf("envFile = %q, want %q", envFile, tt.envFile)
			}

			if len(cmdArgs) != len(tt.cmdArgs) {
				t.Fatalf("cmdArgs = %v, want %v", cmdArgs, tt.cmdArgs)
			}

			for i := range cmdArgs {
				if cmdArgs[i] != tt.cmdArgs[i] {
					t.Errorf("cmdArgs[%d] = %q, want %q", i, cmdArgs[i], tt.cmdArgs[i])
				}
			}
		})
	}
}

func TestParseEnvFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		want        []envEntry
		wantProfile string
		wantErr     bool
	}{
		{
			name:    "normal lines",
			content: "FOO=bar\nBAZ=qux\n",
			want: []envEntry{
				{key: "FOO", value: "bar"},
				{key: "BAZ", value: "qux"},
			},
		},
		{
			name:    "comments and blank lines",
			content: "# comment\n\nFOO=bar\n  # indented comment\n\n",
			want: []envEntry{
				{key: "FOO", value: "bar"},
			},
		},
		{
			name:    "equals in value",
			content: "DB_URL=postgres://user:p=w@host/db\n",
			want: []envEntry{
				{key: "DB_URL", value: "postgres://user:p=w@host/db"},
			},
		},
		{
			name:    "template in value",
			content: "SECRET={{path/to/secret}}\n",
			want: []envEntry{
				{key: "SECRET", value: "{{path/to/secret}}"},
			},
		},
		{
			name:    "malformed line",
			content: "NOEQUALS\n",
			wantErr: true,
		},
		{
			name:    "empty file",
			content: "",
			want:    nil,
		},
		{
			name:        "front matter with profile",
			content:     "profile: staging\n---\nFOO=bar\n",
			wantProfile: "staging",
			want: []envEntry{
				{key: "FOO", value: "bar"},
			},
		},
		{
			name:    "front matter with no profile",
			content: "other: value\n---\nFOO=bar\n",
			want: []envEntry{
				{key: "FOO", value: "bar"},
			},
		},
		{
			name:        "front matter only",
			content:     "profile: prod\n---\n",
			wantProfile: "prod",
			want:        nil,
		},
		{
			name:    "no front matter separator",
			content: "FOO=bar\n",
			want: []envEntry{
				{key: "FOO", value: "bar"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.env")

			err := os.WriteFile(path, []byte(tt.content), 0o644)
			if err != nil {
				t.Fatalf("write test file: %v", err)
			}

			got, err := parseEnvFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", got.profile, tt.wantProfile)
			}

			if len(got.entries) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got.entries), len(tt.want))
			}

			for i := range got.entries {
				if got.entries[i] != tt.want[i] {
					t.Errorf("entry[%d] = %+v, want %+v", i, got.entries[i], tt.want[i])
				}
			}
		})
	}
}

func TestVaultAddr(t *testing.T) {
	tests := []struct {
		name    string
		envAddr string
		cfgAddr string
		want    string
		wantErr bool
	}{
		{
			name:    "from environment",
			envAddr: "https://env.vault",
			cfgAddr: "https://cfg.vault",
			want:    "https://env.vault",
		},
		{
			name:    "from config",
			cfgAddr: "https://cfg.vault",
			want:    "https://cfg.vault",
		},
		{
			name:    "neither set",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VAULT_ADDR", tt.envAddr)

			r := newResolver("", config{DefaultVaultAddr: tt.cfgAddr}, false)

			got, err := r.vaultAddr()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseVaultRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		want    vaultRef
		wantErr bool
	}{
		{
			name: "standard ref",
			ref:  "vault://ele000-stage/services/docbrowser/credentials.client_id",
			want: vaultRef{
				mount: "ele000-stage",
				path:  "services/docbrowser/credentials",
				field: "client_id",
			},
		},
		{
			name: "dot in path",
			ref:  "vault://mount/path/secret.name.field",
			want: vaultRef{
				mount: "mount",
				path:  "path/secret.name",
				field: "field",
			},
		},
		{
			name:    "missing path",
			ref:     "vault://mount",
			wantErr: true,
		},
		{
			name:    "missing field",
			ref:     "vault://mount/path/secret",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVaultRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHasPassRefs(t *testing.T) {
	tests := []struct {
		name    string
		entries []envEntry
		want    bool
	}{
		{
			name:    "no refs",
			entries: []envEntry{{key: "A", value: "plain"}},
			want:    false,
		},
		{
			name:    "pass ref",
			entries: []envEntry{{key: "A", value: "{{pass://secret/path}}"}},
			want:    true,
		},
		{
			name:    "vault ref only",
			entries: []envEntry{{key: "A", value: "{{vault://mount/path.field}}"}},
			want:    false,
		},
		{
			name: "mixed refs",
			entries: []envEntry{
				{key: "A", value: "{{vault://mount/path.field}}"},
				{key: "B", value: "{{pass://secret/path}}"},
			},
			want: true,
		},
		{
			name:    "bare path not detected as pass",
			entries: []envEntry{{key: "A", value: "{{secret/path}}"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasPassRefs(tt.entries)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple value",
			input: "hello",
			want:  "'hello'",
		},
		{
			name:  "value with spaces",
			input: "hello world",
			want:  "'hello world'",
		},
		{
			name:  "value with single quotes",
			input: "it's here",
			want:  "'it'\\''s here'",
		},
		{
			name:  "value with double quotes",
			input: `say "hi"`,
			want:  `'say "hi"'`,
		},
		{
			name:  "empty value",
			input: "",
			want:  "''",
		},
		{
			name:  "value with special chars",
			input: "a$b`c\\d",
			want:  "'a$b`c\\d'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInterpolate(t *testing.T) {
	mockResolve := func(path string) (string, error) {
		return "SECRET(" + path + ")", nil
	}

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{
			name:  "no templates",
			value: "plain-value",
			want:  "plain-value",
		},
		{
			name:  "single template",
			value: "{{secret/path}}",
			want:  "SECRET(secret/path)",
		},
		{
			name:  "template with surrounding text",
			value: "prefix-{{secret/path}}-suffix",
			want:  "prefix-SECRET(secret/path)-suffix",
		},
		{
			name:  "multiple templates",
			value: "{{a}}-{{b}}",
			want:  "SECRET(a)-SECRET(b)",
		},
		{
			name:    "unclosed template",
			value:   "{{unclosed",
			wantErr: true,
		},
		{
			name:  "empty value",
			value: "",
			want:  "",
		},
		{
			name:  "literal braces without double",
			value: "{single}",
			want:  "{single}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := interpolate(tt.value, mockResolve)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVaultCachePath(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		ref     string
		want    string
		wantErr bool
	}{
		{
			name: "standard https",
			addr: "https://vault.example.com",
			ref:  "vault://secret/myapp/creds.password",
			want: "__cache/vault/vault.example.com/secret/myapp/creds.password",
		},
		{
			name: "with port",
			addr: "https://vault.example.com:8200",
			ref:  "vault://mount/path.field",
			want: "__cache/vault/vault.example.com:8200/mount/path.field",
		},
		{
			name: "http address",
			addr: "http://localhost:8200",
			ref:  "vault://kv/data.key",
			want: "__cache/vault/localhost:8200/kv/data.key",
		},
		{
			name:    "invalid addr",
			addr:    "://bad",
			ref:     "vault://mount/path.field",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := vaultCachePath(tt.addr, tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectVaultRefs(t *testing.T) {
	tests := []struct {
		name    string
		entries []envEntry
		want    []string
	}{
		{
			name:    "no refs",
			entries: []envEntry{{key: "A", value: "plain"}},
			want:    nil,
		},
		{
			name:    "pass ref only",
			entries: []envEntry{{key: "A", value: "{{pass://secret/path}}"}},
			want:    nil,
		},
		{
			name: "single vault ref",
			entries: []envEntry{
				{key: "A", value: "{{vault://mount/path.field}}"},
			},
			want: []string{"vault://mount/path.field"},
		},
		{
			name: "multiple unique refs",
			entries: []envEntry{
				{key: "A", value: "{{vault://m/p.f1}}"},
				{key: "B", value: "{{vault://m/p.f2}}"},
			},
			want: []string{"vault://m/p.f1", "vault://m/p.f2"},
		},
		{
			name: "deduplicates",
			entries: []envEntry{
				{key: "A", value: "{{vault://m/p.f}}"},
				{key: "B", value: "{{vault://m/p.f}}"},
			},
			want: []string{"vault://m/p.f"},
		},
		{
			name: "mixed refs",
			entries: []envEntry{
				{key: "A", value: "{{pass://secret/pass}}"},
				{key: "B", value: "{{vault://m/p.f}}"},
				{key: "C", value: "prefix-{{vault://m2/p2.f2}}-suffix"},
			},
			want: []string{"vault://m/p.f", "vault://m2/p2.f2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectVaultRefs(tt.entries)

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ref[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSubstituteVars(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		vars    map[string]string
		want    string
		wantErr bool
	}{
		{
			name:  "no variables",
			value: "plain-value",
			vars:  map[string]string{"foo": "bar"},
			want:  "plain-value",
		},
		{
			name:  "single variable",
			value: "${env}",
			vars:  map[string]string{"env": "staging"},
			want:  "staging",
		},
		{
			name:  "variable with surrounding text",
			value: "prefix-${env}-suffix",
			vars:  map[string]string{"env": "prod"},
			want:  "prefix-prod-suffix",
		},
		{
			name:  "multiple variables",
			value: "${a}-${b}",
			vars:  map[string]string{"a": "x", "b": "y"},
			want:  "x-y",
		},
		{
			name:    "undefined variable",
			value:   "${missing}",
			vars:    map[string]string{},
			wantErr: true,
		},
		{
			name:    "unclosed variable",
			value:   "${unclosed",
			vars:    map[string]string{},
			wantErr: true,
		},
		{
			name:  "empty vars no placeholders",
			value: "plain",
			vars:  map[string]string{},
			want:  "plain",
		},
		{
			name:  "variable inside template ref",
			value: "{{pass://${env}/secret}}",
			vars:  map[string]string{"env": "staging"},
			want:  "{{pass://staging/secret}}",
		},
		{
			name:  "literal single dollar",
			value: "$notavar",
			vars:  map[string]string{},
			want:  "$notavar",
		},
		{
			name:  "default used when undefined",
			value: "${missing:fallback}",
			vars:  map[string]string{},
			want:  "fallback",
		},
		{
			name:  "default ignored when defined",
			value: "${env:fallback}",
			vars:  map[string]string{"env": "staging"},
			want:  "staging",
		},
		{
			name:  "empty default",
			value: "${missing:}",
			vars:  map[string]string{},
			want:  "",
		},
		{
			name:  "quoted default",
			value: `${missing:"hello world"}`,
			vars:  map[string]string{},
			want:  "hello world",
		},
		{
			name:  "quoted default with escaped quotes",
			value: `${missing:"This \"should\" be enough {}"}`,
			vars:  map[string]string{},
			want:  `This "should" be enough {}`,
		},
		{
			name:  "quoted default with braces",
			value: `${missing:"a}b"}`,
			vars:  map[string]string{},
			want:  "a}b",
		},
		{
			name:  "numeric default",
			value: "${count:0}",
			vars:  map[string]string{},
			want:  "0",
		},
		{
			name:  "default with surrounding text",
			value: "prefix-${missing:val}-suffix",
			vars:  map[string]string{},
			want:  "prefix-val-suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := substituteVars(tt.value, tt.vars)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubstituteEntryVars(t *testing.T) {
	t.Run("nil vars errors on variable ref", func(t *testing.T) {
		entries := []envEntry{{key: "A", value: "${foo}"}}

		_, err := substituteEntryVars(entries, nil)
		if err == nil {
			t.Fatal("expected error for undefined variable with nil vars")
		}
	})

	t.Run("nil vars passes through plain values", func(t *testing.T) {
		entries := []envEntry{{key: "A", value: "plain"}}

		got, err := substituteEntryVars(entries, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got[0].value != "plain" {
			t.Errorf("expected passthrough, got %q", got[0].value)
		}
	})

	t.Run("substitutes with vars", func(t *testing.T) {
		entries := []envEntry{
			{key: "A", value: "${env}-value"},
			{key: "B", value: "plain"},
		}
		vars := map[string]string{"env": "staging"}

		got, err := substituteEntryVars(entries, vars)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got[0].value != "staging-value" {
			t.Errorf("got %q, want %q", got[0].value, "staging-value")
		}

		if got[1].value != "plain" {
			t.Errorf("got %q, want %q", got[1].value, "plain")
		}
	})
}

func TestResolveProfile(t *testing.T) {
	tests := []struct {
		name         string
		flagProfile  string
		envProfile   string
		fmProfile    string
		want         string
		wantWarning  bool
	}{
		{
			name: "all empty",
			want: "",
		},
		{
			name:      "front matter only",
			fmProfile: "staging",
			want:      "staging",
		},
		{
			name:       "env var only",
			envProfile: "prod",
			want:       "prod",
		},
		{
			name:        "flag only",
			flagProfile: "dev",
			want:        "dev",
		},
		{
			name:        "flag overrides env var",
			flagProfile: "dev",
			envProfile:  "prod",
			want:        "dev",
		},
		{
			name:        "flag overrides front matter",
			flagProfile: "dev",
			fmProfile:   "staging",
			want:        "dev",
			wantWarning: true,
		},
		{
			name:        "env var overrides front matter",
			envProfile:  "prod",
			fmProfile:   "staging",
			want:        "prod",
			wantWarning: true,
		},
		{
			name:        "flag overrides both",
			flagProfile: "dev",
			envProfile:  "prod",
			fmProfile:   "staging",
			want:        "dev",
			wantWarning: true,
		},
		{
			name:       "same env var and front matter no warning",
			envProfile: "staging",
			fmProfile:  "staging",
			want:       "staging",
		},
		{
			name:        "same flag and front matter no warning",
			flagProfile: "staging",
			fmProfile:   "staging",
			want:        "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TTRUN_PROFILE", tt.envProfile)

			got := resolveProfile(tt.flagProfile, tt.fmProfile)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeProfileConfig(t *testing.T) {
	t.Run("override vault addr", func(t *testing.T) {
		base := config{DefaultVaultAddr: "https://base.vault", Cache: false}
		p := profile{Config: profileConfig{VaultAddr: "https://profile.vault"}}

		got := mergeProfileConfig(base, p)

		if got.DefaultVaultAddr != "https://profile.vault" {
			t.Errorf("VaultAddr = %q, want %q", got.DefaultVaultAddr, "https://profile.vault")
		}

		if got.Cache != false {
			t.Error("Cache should remain false")
		}
	})

	t.Run("override cache", func(t *testing.T) {
		base := config{DefaultVaultAddr: "https://base.vault", Cache: false}
		v := true
		p := profile{Config: profileConfig{Cache: &v}}

		got := mergeProfileConfig(base, p)

		if got.DefaultVaultAddr != "https://base.vault" {
			t.Errorf("VaultAddr = %q, want %q", got.DefaultVaultAddr, "https://base.vault")
		}

		if got.Cache != true {
			t.Error("Cache should be true")
		}
	})

	t.Run("no overrides", func(t *testing.T) {
		base := config{DefaultVaultAddr: "https://base.vault", Cache: true}
		p := profile{}

		got := mergeProfileConfig(base, p)

		if got.DefaultVaultAddr != "https://base.vault" {
			t.Errorf("VaultAddr = %q, want %q", got.DefaultVaultAddr, "https://base.vault")
		}

		if got.Cache != true {
			t.Error("Cache should remain true")
		}
	})

	t.Run("cache false override", func(t *testing.T) {
		base := config{Cache: true}
		v := false
		p := profile{Config: profileConfig{Cache: &v}}

		got := mergeProfileConfig(base, p)

		if got.Cache != false {
			t.Error("Cache should be false")
		}
	})
}

func TestPassPrefixMigration(t *testing.T) {
	r := newResolver("/tmp/fake-store", config{}, false)

	t.Run("bare path errors with guidance", func(t *testing.T) {
		_, err := r.resolve("secret/path")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		want := `secret reference "secret/path" uses deprecated format; update to {{pass://secret/path}}`
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
	})

	t.Run("vault prefix passes through", func(t *testing.T) {
		// This will fail for other reasons (no vault), but should not
		// trigger the deprecated format error.
		_, err := r.resolve("vault://mount/path.field")
		if err == nil {
			return
		}

		if got := err.Error(); got == `secret reference "vault://mount/path.field" uses deprecated format; update to {{pass://vault://mount/path.field}}` {
			t.Error("vault:// ref should not trigger deprecated format error")
		}
	})
}
