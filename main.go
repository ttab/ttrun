package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"
)

type config struct {
	DefaultVaultAddr string `json:"default_vault_addr,omitempty"`
}

func configPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "ttrun", "config.json")
}

func loadConfig() (config, error) {
	data, err := os.ReadFile(configPath())
	if errors.Is(err, os.ErrNotExist) {
		return config{}, nil
	}

	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg config

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func saveConfig(cfg config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(configPath())

	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	err = os.WriteFile(configPath(), append(data, '\n'), 0o644)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttrun: %v\n", err)
		os.Exit(1)
	}
}

func storeDir() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "ttrun", "password-store")
}

func run() error {
	if len(os.Args) >= 2 && os.Args[1] == "set" {
		return runSet(os.Args[2:])
	}

	if len(os.Args) >= 2 && os.Args[1] == "configure" {
		return runConfigure(os.Args[2:])
	}

	if len(os.Args) >= 2 && os.Args[1] == "direnv" {
		return runDirenv(os.Args[2:])
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	envFile, cmdArgs, err := parseArgs(os.Args[1:])
	if err != nil {
		return err
	}

	entries, err := parseEnvFile(envFile)
	if err != nil {
		return err
	}

	dir := storeDir()

	if hasPassRefs(entries) {
		err = ensureStore(dir)
		if err != nil {
			return err
		}
	}

	resolver := newResolver(dir, cfg)

	resolved, err := resolveSecrets(entries, resolver)
	if err != nil {
		return err
	}

	return execCommand(cmdArgs, resolved)
}

func runSet(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ttrun set <secret-path>")
	}

	dir := storeDir()

	err := ensureStore(dir)
	if err != nil {
		return err
	}

	return setSecret(args[0], dir)
}

func runConfigure(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: ttrun configure <key> <value>")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	switch args[0] {
	case "default-vault-addr":
		cfg.DefaultVaultAddr = args[1]
	default:
		return fmt.Errorf("unknown configuration key: %q", args[0])
	}

	return saveConfig(cfg)
}

func runDirenv(args []string) error {
	if len(args) > 0 && args[0] == "hook" {
		return runDirenvHook()
	}

	envFile := "ttrun.env"

	if len(args) == 1 {
		envFile = args[0]
	} else if len(args) > 1 {
		return errors.New("usage: ttrun direnv [envfile]")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	entries, err := parseEnvFile(envFile)
	if err != nil {
		return err
	}

	dir := storeDir()

	if hasPassRefs(entries) {
		err = ensureStore(dir)
		if err != nil {
			return err
		}
	}

	resolver := newResolver(dir, cfg)

	resolved, err := resolveSecrets(entries, resolver)
	if err != nil {
		return err
	}

	for _, env := range resolved {
		key, value, _ := strings.Cut(env, "=")

		fmt.Printf("export %s=%s\n", key, shellQuote(value))
	}

	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func runDirenvHook() error {
	fmt.Print(`use_ttrun() {
  watch_file "${1:-ttrun.env}"
  eval "$(ttrun direnv "$@")"
}
`)

	return nil
}

func hasPassRefs(entries []envEntry) bool {
	for _, e := range entries {
		if !strings.Contains(e.value, "{{") {
			continue
		}

		rest := e.value

		for {
			i := strings.Index(rest, "{{")
			if i == -1 {
				break
			}

			rest = rest[i+2:]

			j := strings.Index(rest, "}}")
			if j == -1 {
				break
			}

			ref := rest[:j]
			rest = rest[j+2:]

			if !strings.HasPrefix(ref, "vault://") {
				return true
			}
		}
	}

	return false
}

type envEntry struct {
	key   string
	value string
}

func parseArgs(args []string) (envFile string, cmdArgs []string, err error) {
	envFile = "ttrun.env"

	sepIdx := -1

	for i, arg := range args {
		if arg == "--" {
			sepIdx = i

			break
		}
	}

	if sepIdx == -1 {
		return "", nil, errors.New("missing '--' separator; usage: ttrun [envfile] -- command [args...]")
	}

	before := args[:sepIdx]
	after := args[sepIdx+1:]

	if len(before) > 1 {
		return "", nil, fmt.Errorf("too many arguments before '--': %v", before)
	}

	if len(before) == 1 {
		envFile = before[0]
	}

	if len(after) == 0 {
		return "", nil, errors.New("no command specified after '--'")
	}

	return envFile, after, nil
}

func parseEnvFile(path string) (entries []envEntry, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}

	defer func() {
		err := f.Close()
		if err != nil && retErr == nil {
			retErr = fmt.Errorf("close env file: %w", err)
		}
	}()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("malformed line (no '='): %q", line)
		}

		entries = append(entries, envEntry{key: key, value: value})
	}

	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}

	return entries, nil
}

func interpolate(value string, resolve func(string) (string, error)) (string, error) {
	var result strings.Builder

	rest := value

	for {
		openIdx := strings.Index(rest, "{{")
		if openIdx == -1 {
			result.WriteString(rest)

			break
		}

		result.WriteString(rest[:openIdx])

		rest = rest[openIdx+2:]

		closeIdx := strings.Index(rest, "}}")
		if closeIdx == -1 {
			return "", fmt.Errorf("unclosed '{{' in value: %q", value)
		}

		path := rest[:closeIdx]
		rest = rest[closeIdx+2:]

		secret, err := resolve(path)
		if err != nil {
			return "", fmt.Errorf("resolve %q: %w", path, err)
		}

		result.WriteString(secret)
	}

	return result.String(), nil
}

type resolver struct {
	passDir    string
	cfg        config
	vaultCache map[string]map[string]string
}

func newResolver(passDir string, cfg config) *resolver {
	return &resolver{
		passDir:    passDir,
		cfg:        cfg,
		vaultCache: make(map[string]map[string]string),
	}
}

func (r *resolver) resolve(ref string) (string, error) {
	if strings.HasPrefix(ref, "vault://") {
		return r.resolveVault(ref)
	}

	return getOrCreateSecret(ref, r.passDir)
}

type vaultRef struct {
	mount string
	path  string
	field string
}

func parseVaultRef(ref string) (vaultRef, error) {
	// vault://mount/path/to/secret.field
	trimmed := strings.TrimPrefix(ref, "vault://")

	slashIdx := strings.Index(trimmed, "/")
	if slashIdx == -1 {
		return vaultRef{}, fmt.Errorf("invalid vault reference %q: missing path", ref)
	}

	mount := trimmed[:slashIdx]
	rest := trimmed[slashIdx+1:]

	dotIdx := strings.LastIndex(rest, ".")
	if dotIdx == -1 {
		return vaultRef{}, fmt.Errorf("invalid vault reference %q: missing field (use path.field)", ref)
	}

	return vaultRef{
		mount: mount,
		path:  rest[:dotIdx],
		field: rest[dotIdx+1:],
	}, nil
}

func (r *resolver) vaultAddr() (string, error) {
	if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		return addr, nil
	}

	if r.cfg.DefaultVaultAddr != "" {
		return r.cfg.DefaultVaultAddr, nil
	}

	return "", errors.New("VAULT_ADDR is not set and no default is configured\n" +
		"  Set it with: export VAULT_ADDR=https://your-vault.server\n" +
		"  Or configure a default: ttrun configure default-vault-addr https://your-vault.server")
}

func (r *resolver) resolveVault(ref string) (string, error) {
	v, err := parseVaultRef(ref)
	if err != nil {
		return "", err
	}

	cacheKey := v.mount + "/" + v.path

	fields, ok := r.vaultCache[cacheKey]
	if !ok {
		addr, err := r.vaultAddr()
		if err != nil {
			return "", err
		}

		fields, err = vaultGet(v.mount, v.path, addr)
		if err != nil {
			return "", err
		}

		r.vaultCache[cacheKey] = fields
	}

	value, ok := fields[v.field]
	if !ok {
		return "", fmt.Errorf("vault secret %s/%s has no field %q", v.mount, v.path, v.field)
	}

	return value, nil
}

func vaultGet(mount, path, addr string) (map[string]string, error) {
	args := []string{"kv", "get", "-mount=" + mount, "-format=json", path}

	cmd := exec.Command("vault", args...)
	cmd.Env = append(os.Environ(), "VAULT_ADDR="+addr)

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("vault kv get %s/%s: %s", mount, path, strings.TrimSpace(string(exitErr.Stderr)))
		}

		return nil, fmt.Errorf("vault kv get %s/%s: %w", mount, path, err)
	}

	var response struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}

	err = json.Unmarshal(out, &response)
	if err != nil {
		return nil, fmt.Errorf("parse vault response for %s/%s: %w", mount, path, err)
	}

	fields := make(map[string]string, len(response.Data.Data))

	for k, v := range response.Data.Data {
		fields[k] = fmt.Sprint(v)
	}

	return fields, nil
}

func resolveSecrets(entries []envEntry, r *resolver) ([]string, error) {
	var env []string

	for _, e := range entries {
		resolved, err := interpolate(e.value, r.resolve)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", e.key, err)
		}

		env = append(env, e.key+"="+resolved)
	}

	return env, nil
}

type gpgKey struct {
	fingerprint string
	uid         string
}

func listSecretKeys() ([]gpgKey, error) {
	cmd := exec.Command("gpg", "--list-secret-keys", "--with-colons")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list GPG secret keys: %w", err)
	}

	var keys []gpgKey

	var currentFpr string

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ":")

		switch fields[0] {
		case "fpr":
			if currentFpr == "" {
				currentFpr = fields[9]
			}
		case "uid":
			if currentFpr != "" {
				keys = append(keys, gpgKey{
					fingerprint: currentFpr,
					uid:         fields[9],
				})

				currentFpr = ""
			}
		case "sec":
			currentFpr = ""
		}
	}

	return keys, nil
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)

	pw, err := term.ReadPassword(int(os.Stdin.Fd()))

	fmt.Fprintln(os.Stderr)

	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	return string(pw), nil
}

func generateKey() (string, error) {
	passphrase, err := readPassword("Enter passphrase for new GPG key: ")
	if err != nil {
		return "", err
	}

	confirm, err := readPassword("Confirm passphrase: ")
	if err != nil {
		return "", err
	}

	if passphrase != confirm {
		return "", errors.New("passphrases do not match")
	}

	fmt.Fprintln(os.Stderr, "Generating a new GPG key for ttrun...")

	cmd := exec.Command("gpg", "--batch",
		"--pinentry-mode", "loopback",
		"--passphrase-fd", "0",
		"--quick-gen-key", "ttrun local secrets", "default", "default", "never")
	cmd.Stdin = strings.NewReader(passphrase)
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("generate GPG key: %w", err)
	}

	keys, err := listSecretKeys()
	if err != nil {
		return "", err
	}

	for _, k := range keys {
		if k.uid == "ttrun local secrets" {
			return k.fingerprint, nil
		}
	}

	return "", errors.New("could not find newly generated GPG key")
}

func promptKey() (string, error) {
	keys, err := listSecretKeys()
	if err != nil {
		return "", err
	}

	if len(keys) == 0 {
		fmt.Fprintln(os.Stderr, "No GPG keys found on this system.")
		fmt.Fprint(os.Stderr, "Generate a new key for ttrun? [Y/n] ")

		answer, err := readLine()
		if err != nil {
			return "", fmt.Errorf("read response: %w", err)
		}

		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "" && answer != "y" && answer != "yes" {
			return "", errors.New("cannot initialise store without a GPG key")
		}

		return generateKey()
	}

	if len(keys) == 1 {
		fmt.Fprintf(os.Stderr, "Found GPG key: %s\n", keys[0].uid)
		fmt.Fprint(os.Stderr, "Use this key for ttrun? [Y/n] ")

		answer, err := readLine()
		if err != nil {
			return "", fmt.Errorf("read response: %w", err)
		}

		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "" && answer != "y" && answer != "yes" {
			return "", errors.New("cannot initialise store without a GPG key")
		}

		return keys[0].fingerprint, nil
	}

	fmt.Fprintln(os.Stderr, "Available GPG keys:")

	for i, k := range keys {
		fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, k.uid)
	}

	fmt.Fprint(os.Stderr, "Select a key (number): ")

	answer, err := readLine()
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var choice int

	_, err = fmt.Sscanf(strings.TrimSpace(answer), "%d", &choice)
	if err != nil || choice < 1 || choice > len(keys) {
		return "", fmt.Errorf("invalid selection: %q", answer)
	}

	return keys[choice-1].fingerprint, nil
}

func readLine() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)

	if !scanner.Scan() {
		err := scanner.Err()
		if err != nil {
			return "", err
		}

		return "", errors.New("no input")
	}

	return scanner.Text(), nil
}

func ensureStore(storeDir string) error {
	_, err := os.Stat(storeDir)
	if err == nil {
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check store directory: %w", err)
	}

	fmt.Fprintf(os.Stderr, "ttrun: initialising password store at %s\n", storeDir)

	gpgID, err := promptKey()
	if err != nil {
		return err
	}

	cmd := passCmd(storeDir, "init", gpgID)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("initialise password store: %w", err)
	}

	return nil
}

func setSecret(path, storeDir string) error {
	value, err := readPassword(fmt.Sprintf("Enter secret for %s: ", path))
	if err != nil {
		return err
	}

	confirm, err := readPassword(fmt.Sprintf("Confirm secret for %s: ", path))
	if err != nil {
		return err
	}

	if value != confirm {
		return errors.New("secrets do not match")
	}

	return passInsert(storeDir, path, value)
}

func passInsert(storeDir, path, value string) error {
	cmd := passCmd(storeDir, "insert", "--multiline", "--force", path)
	cmd.Stdin = strings.NewReader(value + "\n")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("insert secret %q: %w", path, err)
	}

	return nil
}

func getOrCreateSecret(path, storeDir string) (string, error) {
	secret, err := passShow(storeDir, path)
	if err == nil {
		return secret, nil
	}

	fmt.Fprintf(os.Stderr, "ttrun: secret %q not found\n", path)

	value, err := readPassword(fmt.Sprintf("Enter secret for %s: ", path))
	if err != nil {
		return "", err
	}

	err = passInsert(storeDir, path, value)
	if err != nil {
		return "", err
	}

	return passShow(storeDir, path)
}

func passShow(storeDir, path string) (string, error) {
	cmd := passCmd(storeDir, "show", path)

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(out), "\n"), nil
}

func passCmd(storeDir string, args ...string) *exec.Cmd {
	cmd := exec.Command("pass", args...)
	cmd.Env = append(os.Environ(), "PASSWORD_STORE_DIR="+storeDir)

	return cmd
}

func execCommand(cmdArgs []string, env []string) error {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	)

	go func() {
		for sig := range sigCh {
			_ = cmd.Process.Signal(sig)
		}
	}()

	err = cmd.Wait()

	signal.Stop(sigCh)
	close(sigCh)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		return fmt.Errorf("wait for command: %w", err)
	}

	return nil
}
