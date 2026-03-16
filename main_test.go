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
		name    string
		content string
		want    []envEntry
		wantErr bool
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

			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry[%d] = %+v, want %+v", i, got[i], tt.want[i])
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
			entries: []envEntry{{key: "A", value: "{{secret/path}}"}},
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
				{key: "B", value: "{{secret/path}}"},
			},
			want: true,
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
			entries: []envEntry{{key: "A", value: "{{secret/path}}"}},
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
				{key: "A", value: "{{secret/pass}}"},
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
