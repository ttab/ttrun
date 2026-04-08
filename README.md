## ttrun

`ttrun` runs a command with environment variables loaded from a file, resolving secret references along the way. It supports two secret backends:

- **pass** -- secrets stored in a local [pass](https://www.passwordstore.org/) password store
- **Vault** -- secrets fetched from HashiCorp Vault using the `vault` CLI

### Installation

```
go install github.com/ttab/ttrun@latest
```

For local pass secrets, `gpg` and `pass` must be installed. For Vault secrets, the `vault` CLI must be available. If you only use one backend, you don't need the other.

### Usage

```
ttrun [envfile] [--profile=name] -- command [args...]
ttrun set <secret-path>
ttrun get <secret-path>
ttrun ls [subfolder]
ttrun pull [envfile]
ttrun configure [--profile=name] <key> <value>
ttrun profile set <name> <key> <value>
ttrun direnv [envfile]
ttrun direnv hook
```

Global options (can appear anywhere before `--`):

- `-h`, `--help` -- show usage information
- `-v`, `--verbose` -- print each subprocess command to stderr and show their stderr output
- `-p`, `--profile <name>` -- use a named profile for config overrides and variables

`run`, `direnv`, and `pull` resolve the active profile from (highest priority first):

1. `--profile`/`-p` CLI flag
2. `TTRUN_PROFILE` environment variable
3. Front matter in the env file (see below)

A warning is printed when the CLI flag or environment variable overrides a front matter profile.

`configure` only targets a profile when `--profile` is explicitly passed on the command line; `TTRUN_PROFILE` and front matter are ignored.

If no env file is specified, `ttrun` reads `ttrun.env` in the current directory.

### Env file format

The env file contains `KEY=value` lines. Empty lines and lines starting with `#` are ignored.

An optional YAML front matter block can appear at the top of the file, terminated by a `---` line. The front matter can specify a default profile:

```
profile: staging
---
KEY=value
```

Values can contain `{{...}}` references that are resolved before the command is started. Everything else is passed through as-is.

```shell
# Plain values
ADDR=:4280
PROFILE_ADDR=:4281
REPOSITORY_ENDPOINT=http://localhost:1080

# Secrets from the local pass store
GEMINI_API_KEY={{pass://external/gemini_api_key}}
DEEPL_API_KEY={{pass://external/deepl_api_key}}

# Secrets from Vault
CLIENT_ID={{vault://mount/services/docbrowser/credentials.client_id}}
CLIENT_SECRET={{vault://mount/services/docbrowser/credentials.client_secret}}

# Plain values can contain = signs
DB_URL=postgres://user:pass@localhost/db?sslmode=disable
```

#### Variable substitution

`${name}` references are resolved from the active profile's variables before secret interpolation. Without a default, an undefined variable is an error.

Default values can be provided after a `:`:

| Syntax | Result when undefined |
|---|---|
| `${name}` | error |
| `${name:fallback}` | `fallback` |
| `${name:}` | empty string |
| `${name:"text with spaces"}` | `text with spaces` |
| `${name:"escaped \"quotes\" and {}"}` | `escaped "quotes" and {}` |

Quoted defaults (`"..."`) support `\"` for literal quotes and allow `}` inside the value without ending the expression.

This makes it easy to switch between environments with a single env file:

``` shell
profile: customer0
---
SOME_VAR=${some_var}
GREETING=${greeting:"Hello, world!"}
COUNT=${count:0}

# Secrets from Vault
CLIENT_ID={{vault://ele-${customer}-stage/services/docbrowser/credentials.client_id}}
CLIENT_SECRET={{vault://ele-${customer}-stage/services/docbrowser/credentials.client_secret}}
```

Variables are defined in the profiles configuration file (`$XDG_CONFIG_HOME/ttrun/profiles.yaml`):

```
ttrun profile set customer0 some_var hello
ttrun profile set customer0 customer 000
```

Or edit the file directly:

``` yaml
customer0:
  variables:
    some_var: "hello"
    customer: "000"
customer1:
  variables:
    some_var: "world"
    customer: "001"
otherprofile:
  config:
    vault_addr: "https://other.vault.example.com"
    cache: false
  variables:
    some_var: "out of"
    customer: "333"
```

### Secret references

#### pass secrets

A reference like `{{pass://external/gemini_api_key}}` is resolved from ttrun's dedicated pass store at `~/.local/share/ttrun/password-store`.

If the secret doesn't exist, ttrun prompts you to enter a value and stores it for future use.

#### Vault secrets

A reference like `{{vault://mount/path/to/secret.field}}` fetches a secret from Vault:

- `mount` -- the KV secrets engine mount (e.g. `ele000-stage`)
- `path/to/secret` -- the secret path within the mount
- `field` -- the field to extract from the secret (after the last `.`)

If multiple references point to the same secret (same mount and path), it is only fetched once.

Vault authentication is handled by the `vault` CLI (e.g. via `VAULT_TOKEN` or a prior `vault login`). The Vault address is resolved from `VAULT_ADDR`, the profile's `vault_addr`, or the global `default-vault-addr` (in that order). If none is set, ttrun exits with an error.

### Environment variables

- `TTRUN_PROFILE` -- default profile name; overridden by `--profile`, overrides env file front matter
- `VAULT_ADDR` -- Vault server address (takes precedence over the configured default)

### Configuration

ttrun stores global configuration in `~/.config/ttrun/config.json`. Per-profile configuration is stored in `$XDG_CONFIG_HOME/ttrun/profiles.yaml` (defaults to `~/.config/ttrun/profiles.yaml`).

Use `ttrun configure` to set values. Without `--profile` the change applies globally; with `--profile` it applies to that profile only:

```
ttrun configure default-vault-addr https://vault.example.com
ttrun configure --profile=other default-vault-addr https://other-vault.example.com
```

Available configuration keys:

- `default-vault-addr` -- Vault server address to use when `VAULT_ADDR` is not set
- `cache` -- enable persistent caching of vault secrets in the local pass store (`true`/`false`, default `false`)

### Vault caching

Vault lookups require network access and authentication. You can enable persistent caching to resolve vault secrets offline after an initial fetch:

```
ttrun configure cache true
```

When caching is enabled, resolved vault secrets are stored in the local pass store under `__cache/vault/<host>/...`. On subsequent runs, cached values are used without contacting Vault.

#### Pre-populating the cache

The `pull` command fetches and caches all vault secrets referenced in the env file in one go:

```
ttrun pull
ttrun pull myapp.env
```

This is useful for going offline or for populating a fresh machine. It fetches each unique vault path once and caches all fields returned, so even fields not yet referenced in the env file are available.

**Note:** `pull` always fetches from Vault regardless of what's already cached, so it can also be used to refresh stale cache entries.

### Updating secrets

To set or update a secret in the local pass store:

```
ttrun set client_secrets/testing
```

This prompts for the value (with confirmation) and stores it. Use this to rotate secrets without having to delete and re-run.

### Listing secrets

To list secrets in the local pass store:

```
ttrun ls
ttrun ls external
```

### Reading secrets

To print a secret from the local pass store:

```
ttrun get external/gemini_api_key
```

### First run

The first time ttrun encounters a pass secret reference, it initialises its password store. It will:

1. Look for GPG keys on your system
2. If you have one key, ask to use it
3. If you have multiple keys, let you choose
4. If you have no keys, offer to generate one (with a passphrase you choose)

After that, the store is ready and you won't be prompted again.

### direnv integration

ttrun can be used as a [direnv](https://direnv.net/) extension, so that your environment variables (with resolved secrets) are automatically loaded when you `cd` into a project directory.

#### 1. Install the hook

Run this once to add the `use_ttrun` function to direnv's library:

```
mkdir -p ~/.config/direnv/lib
ttrun direnv hook > ~/.config/direnv/lib/ttrun.sh
```

#### 2. Add `use ttrun` to your `.envrc`

In any project directory that has a `ttrun.env` file:

```
echo "use ttrun" >> .envrc
direnv allow
```

To use a different env file:

```
echo 'use ttrun myapp.env' >> .envrc
direnv allow
```

#### 3. That's it

direnv will now resolve your secrets and export the environment variables whenever you enter the directory. It automatically re-evaluates when the env file changes.

**Note:** Since direnv runs non-interactively, `ttrun direnv` cannot prompt for missing secrets. Any variable whose secrets can't be resolved is skipped, and a message is logged to stderr. Populate missing pass secrets with `ttrun set` or by running `ttrun` directly once.

### How it works

ttrun reads the env file, resolves all secret references, then starts the specified command with the resolved environment variables added to the current environment (env file values override any existing variables with the same name).

Signals (INT, TERM, HUP, QUIT, USR1, USR2) are forwarded to the child process, and ttrun exits with the child's exit code.
