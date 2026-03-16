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
ttrun [envfile] -- command [args...]
ttrun set <secret-path>
ttrun get <secret-path>
ttrun ls [subfolder]
ttrun configure <key> <value>
ttrun direnv [envfile]
ttrun direnv hook
```

Global options (can appear anywhere before `--`):

- `-h`, `--help` -- show usage information
- `-v`, `--verbose` -- print each subprocess command to stderr and show their stderr output

If no env file is specified, `ttrun` reads `ttrun.env` in the current directory.

### Env file format

The env file contains `KEY=value` lines. Empty lines and lines starting with `#` are ignored.

Values can contain `{{...}}` references that are resolved before the command is started. Everything else is passed through as-is.

```shell
# Plain values
ADDR=:4280
PROFILE_ADDR=:4281
REPOSITORY_ENDPOINT=http://localhost:1080

# Secrets from the local pass store
GEMINI_API_KEY={{external/gemini_api_key}}
DEEPL_API_KEY={{external/deepl_api_key}}

# Secrets from Vault
CLIENT_ID={{vault://mount/services/docbrowser/credentials.client_id}}
CLIENT_SECRET={{vault://mount/services/docbrowser/credentials.client_secret}}

# Plain values can contain = signs
DB_URL=postgres://user:pass@localhost/db?sslmode=disable
```

### Secret references

#### pass secrets

A reference like `{{external/gemini_api_key}}` is resolved from ttrun's dedicated pass store at `~/.local/share/ttrun/password-store`.

If the secret doesn't exist, ttrun prompts you to enter a value and stores it for future use.

#### Vault secrets

A reference like `{{vault://mount/path/to/secret.field}}` fetches a secret from Vault:

- `mount` -- the KV secrets engine mount (e.g. `ele000-stage`)
- `path/to/secret` -- the secret path within the mount
- `field` -- the field to extract from the secret (after the last `.`)

If multiple references point to the same secret (same mount and path), it is only fetched once.

Vault authentication is handled by the `vault` CLI (e.g. via `VAULT_TOKEN` or a prior `vault login`). The Vault address is resolved from `VAULT_ADDR` or the configured default (see below). If neither is set, ttrun exits with an error.

### Configuration

ttrun stores configuration in `~/.config/ttrun/config.json`. Use `ttrun configure` to set values:

```
ttrun configure default-vault-addr https://vault.example.com
```

Available configuration keys:

- `default-vault-addr` -- Vault server address to use when `VAULT_ADDR` is not set

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
