// Package runner_test drives the run.sh bootstrap end to end.
//
// Every fixture installs shell stubs for curl, uname, and the downloaded
// asset, so the scenarios run without network access or release assets: run.sh
// downloads the stub asset through the stub curl, verifies its SHA-256
// checksum, and executes it in place. The suite ports run.test.ts from the
// pre-rewrite pipeline, minus the cases for the removed musl/glibc refusal and
// the macOS script(1) wrapper.
package runner_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const repository = "johnnymayodev/Global-AGENTS.md"

// systemPath is the trusted tail of every fixture PATH; the stub bin directory
// is prepended to it.
const systemPath = "/usr/bin:/bin"

var (
	repoRoot = mustRepoRoot()
	runSh    = filepath.Join(repoRoot, "run.sh")
	version  = mustReadVersion(runSh)

	// hashToolAvailable mirrors run.sh's sha256sum/shasum requirement; tests
	// that exercise the full bootstrap skip without it.
	hashToolAvailable = lookPathIn(systemPath, "sha256sum") || lookPathIn(systemPath, "shasum")
)

func mustRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot locate runner_test.go")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// mustReadVersion reads the pinned version from run.sh, the single version
// source since package.json is retired.
func mustReadVersion(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	match := regexp.MustCompile(`(?m)^version=(.+)$`).FindStringSubmatch(string(contents))
	if match == nil {
		panic("run.sh has no version= line")
	}
	return strings.TrimSpace(match[1])
}

func lookPathIn(path, name string) bool {
	for _, dir := range filepath.SplitList(path) {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// executableIn returns the first executable named name under path.
func executableIn(path, name string) string {
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

func requireHashTool(t *testing.T) {
	t.Helper()
	if !hashToolAvailable {
		t.Skip("sha256sum and shasum are unavailable")
	}
}

const curlStub = `#!/bin/sh
set -eu
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      output=$2
      shift 2
      ;;
    https://*)
      url=$1
      shift
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s\n' "$url" >> "$CURL_URL_CAPTURE"
if [ -n "${CURL_FAIL:-}" ]; then
  exit "${CURL_FAIL:-22}"
fi
if [ -n "${CURL_PARTIAL:-}" ]; then
  printf 'partial' > "$output"
  exit 18
fi
# Choose the payload by URL, not by the output name: run.sh downloads into
# private mktemp files whose names intentionally do not look like release
# assets.
case "$url" in
  *checksums-*.txt)
    # The test pre-writes the checksum file next to the fixture; prefer an
    # explicit override, else fall back to the file the test already staged.
    if [ -n "${CURL_CHECKSUM_FILE:-}" ]; then
      cp "$CURL_CHECKSUM_FILE" "$output"
    else
      cp "$TMPDIR/$(basename "$url")" "$output"
    fi
    ;;
  *)
    cp "$CURL_SOURCE_FILE" "$output"
    ;;
esac
`

const unameStub = `#!/bin/sh
set -eu
case "$1" in
  -s) printf '%s\n' "$UNAME_OS" ;;
  -m) printf '%s\n' "$UNAME_MACHINE" ;;
  *) exit 1 ;;
esac
`

const assetStub = `#!/bin/sh
set -eu
printf '%s\n' "$0" > "$ASSET_EXE_CAPTURE"
printf '%s\n' "$PWD" > "$PWD_CAPTURE"
if [ -n "${UMASK_CAPTURE:-}" ]; then
  umask > "$UMASK_CAPTURE"
fi
: > "$ARGS_CAPTURE"
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$ARGS_CAPTURE"
done
if [ -n "${ASSET_STDIN_TTY:-}" ]; then
  if [ -t 0 ]; then
    printf 'tty' > "$ASSET_STDIN_TTY"
  else
    printf 'not-tty' > "$ASSET_STDIN_TTY"
  fi
fi
exit "${ASSET_EXIT:-0}"
`

type fixture struct {
	t        *testing.T
	tempDir  string
	binDir   string
	homeDir  string
	curlURLs string
	argsPath string
	pwdPath  string
	assetExe string
	assetSrc string
	env      map[string]string
}

type result struct {
	status int
	stdout string
	stderr string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	tempDir := t.TempDir()
	f := &fixture{
		t:        t,
		tempDir:  tempDir,
		binDir:   filepath.Join(tempDir, "bin"),
		homeDir:  filepath.Join(tempDir, "home"),
		curlURLs: filepath.Join(tempDir, "curl-urls"),
		argsPath: filepath.Join(tempDir, "asset-args"),
		pwdPath:  filepath.Join(tempDir, "asset-pwd"),
		assetExe: filepath.Join(tempDir, "asset-exe"),
		assetSrc: filepath.Join(tempDir, "asset-source"),
	}
	for _, dir := range []string{f.binDir, f.homeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	f.writeExecutable(filepath.Join(f.binDir, "curl"), curlStub)
	f.writeExecutable(filepath.Join(f.binDir, "uname"), unameStub)
	if err := os.WriteFile(f.assetSrc, []byte(assetStub), 0o644); err != nil {
		t.Fatalf("write asset source: %v", err)
	}

	env := make(map[string]string, len(os.Environ())+10)
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	env["PATH"] = f.binDir + ":" + systemPath
	env["TMPDIR"] = f.tempDir
	env["HOME"] = f.homeDir
	env["CURL_URL_CAPTURE"] = f.curlURLs
	env["CURL_SOURCE_FILE"] = f.assetSrc
	env["ASSET_EXE_CAPTURE"] = f.assetExe
	env["ARGS_CAPTURE"] = f.argsPath
	env["PWD_CAPTURE"] = f.pwdPath
	env["UNAME_OS"] = unameOS(runtime.GOOS)
	env["UNAME_MACHINE"] = unameMachine(runtime.GOARCH)
	// Keep a developer's real cache out of every fixture; cache tests set
	// XDG_CACHE_HOME explicitly.
	delete(env, "XDG_CACHE_HOME")
	f.env = env
	return f
}

// cacheLeaf is the default version-scoped cache directory for this fixture.
func (f *fixture) cacheLeaf() string {
	return filepath.Join(f.homeDir, ".cache", "agent-links", "v"+version, hostTarget())
}

func (f *fixture) cacheAsset() string {
	return filepath.Join(f.cacheLeaf(), "agent-links-"+hostTarget())
}

func (f *fixture) cacheSums() string {
	return filepath.Join(f.cacheLeaf(), "checksums-"+hostTarget()+".txt")
}

// downloadCalls counts the asset and checksum URLs fetched so far.
func (f *fixture) downloadCalls() int {
	f.t.Helper()
	contents := strings.TrimSpace(f.readCapture(f.curlURLs))
	if contents == "" {
		return 0
	}
	return len(strings.Split(contents, "\n"))
}

// resetDownloads clears the curl URL capture and the executed-asset marker,
// so a second run can prove it neither downloaded nor skipped execution.
func (f *fixture) resetDownloads() {
	f.t.Helper()
	if err := os.WriteFile(f.curlURLs, nil, 0o644); err != nil {
		f.t.Fatalf("reset curl capture: %v", err)
	}
	if err := os.Remove(f.assetExe); err != nil && !errors.Is(err, os.ErrNotExist) {
		f.t.Fatalf("reset asset capture: %v", err)
	}
}

// assertModes checks the cache directory and file permissions.
func (f *fixture) assertModes() {
	f.t.Helper()
	for path, want := range map[string]os.FileMode{
		f.cacheLeaf():  0o700,
		f.cacheAsset(): 0o700,
		f.cacheSums():  0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			f.t.Errorf("stat %s: %v", path, err)
			continue
		}
		if got := info.Mode().Perm(); got != want {
			f.t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}

// assertNoCacheFiles checks that no partial or executable cache entry was
// left behind. The leaf directory itself may exist.
func (f *fixture) assertNoCacheFiles() {
	f.t.Helper()
	entries, err := os.ReadDir(f.cacheLeaf())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		f.t.Fatalf("read cache leaf: %v", err)
	}
	for _, entry := range entries {
		f.t.Errorf("cache contains an unexpected entry: %s", entry.Name())
	}
}

func unameOS(goos string) string {
	if goos == "darwin" {
		return "Darwin"
	}
	return "Linux"
}

func unameMachine(goarch string) string {
	if goarch == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

func (f *fixture) writeExecutable(path, contents string) {
	f.t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		f.t.Fatalf("write %s: %v", path, err)
	}
}

// fakeUname overrides the platform reported by the uname stub.
func (f *fixture) fakeUname(osName, machine string) {
	f.writeExecutable(filepath.Join(f.binDir, "uname"), unameStub)
	f.env["UNAME_OS"] = osName
	f.env["UNAME_MACHINE"] = machine
}

// withChecksum stages the release checksum file for assetName and points the
// curl stub at it.
func (f *fixture) withChecksum(assetName string) {
	f.t.Helper()
	contents, err := os.ReadFile(f.assetSrc)
	if err != nil {
		f.t.Fatalf("read asset source: %v", err)
	}
	sum := sha256.Sum256(contents)
	line := hex.EncodeToString(sum[:]) + "  " + assetName + "\n"
	path := filepath.Join(f.tempDir, "checksums-"+strings.TrimPrefix(assetName, "agent-links-")+".txt")
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		f.t.Fatalf("write checksum file: %v", err)
	}
	f.env["CURL_CHECKSUM_FILE"] = path
}

// run executes run.sh through bash with the fixture environment. dir defaults
// to the temp directory.
func (f *fixture) run(args []string, overrides map[string]string, dir string) result {
	f.t.Helper()
	if dir == "" {
		dir = f.tempDir
	}
	cmd := exec.Command("bash", append([]string{runSh}, args...)...)
	cmd.Dir = dir
	cmd.Env = f.envWith(overrides)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return result{status: exitStatus(err), stdout: stdout.String(), stderr: stderr.String()}
}

func (f *fixture) envWith(overrides map[string]string) []string {
	merged := make(map[string]string, len(f.env)+len(overrides))
	for key, value := range f.env {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	env := make([]string, 0, len(merged))
	for key, value := range merged {
		env = append(env, key+"="+value)
	}
	return env
}

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (f *fixture) readCapture(path string) string {
	f.t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("read capture %s: %v", path, err)
	}
	return string(contents)
}

// assertClean checks that the bootstrap removed its mktemp directory.
func (f *fixture) assertClean() {
	f.t.Helper()
	entries, err := os.ReadDir(f.tempDir)
	if err != nil {
		f.t.Fatalf("read %s: %v", f.tempDir, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "agent-links.") {
			f.t.Errorf("temporary directory was not cleaned up: %s", entry.Name())
		}
	}
}

func hostTarget() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return runtime.GOOS + "-" + arch
}

func releaseURL(target string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/v%s/agent-links-%s", repository, version, target)
}

func checksumURL(target string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/v%s/checksums-%s.txt", repository, version, target)
}

func TestPlatformDetection(t *testing.T) {
	requireHashTool(t)
	cases := []struct {
		osName  string
		machine string
		target  string
	}{
		{"Darwin", "x86_64", "darwin-x64"},
		{"Darwin", "amd64", "darwin-x64"},
		{"Darwin", "aarch64", "darwin-arm64"},
		{"Darwin", "arm64", "darwin-arm64"},
		{"Linux", "x86_64", "linux-x64"},
		{"Linux", "amd64", "linux-x64"},
		{"Linux", "aarch64", "linux-arm64"},
		{"Linux", "arm64", "linux-arm64"},
	}
	for _, tc := range cases {
		t.Run(tc.osName+" "+tc.machine, func(t *testing.T) {
			f := newFixture(t)
			f.fakeUname(tc.osName, tc.machine)
			f.withChecksum("agent-links-" + tc.target)

			got := f.run(nil, nil, "")

			if got.status != 0 {
				t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
			}
			urls := strings.Fields(f.readCapture(f.curlURLs))
			want := []string{releaseURL(tc.target), checksumURL(tc.target)}
			if !reflect.DeepEqual(urls, want) {
				t.Errorf("downloaded URLs = %v, want %v", urls, want)
			}
			if _, err := os.Stat(f.assetExe); err != nil {
				t.Errorf("asset was not executed: %v", err)
			}
			f.assertClean()
		})
	}
}

func TestRejectsUnsupportedOS(t *testing.T) {
	f := newFixture(t)
	f.fakeUname("FreeBSD", "x86_64")

	got := f.run(nil, nil, "")

	if got.status != 2 {
		t.Fatalf("status = %d, want 2; stderr:\n%s", got.status, got.stderr)
	}
	if !strings.Contains(got.stderr, "unsupported platform") {
		t.Errorf("stderr = %q, want an unsupported platform error", got.stderr)
	}
	if _, err := os.Stat(f.assetExe); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asset was executed despite the unsupported platform: %v", err)
	}
	f.assertNoCacheFiles()
	f.assertClean()
}

func TestRejectsUnsupportedArchitecture(t *testing.T) {
	f := newFixture(t)
	f.fakeUname("Linux", "riscv64")

	got := f.run(nil, nil, "")

	if got.status != 2 {
		t.Fatalf("status = %d, want 2; stderr:\n%s", got.status, got.stderr)
	}
	if !strings.Contains(got.stderr, "unsupported platform") {
		t.Errorf("stderr = %q, want an unsupported platform error", got.stderr)
	}
	if _, err := os.Stat(f.assetExe); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asset was executed despite the unsupported architecture: %v", err)
	}
	f.assertNoCacheFiles()
	f.assertClean()
}

func TestForwardsArgumentsVerbatim(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	args := []string{"--path", "my config/AGENTS.md", "./relative"}

	got := f.run(args, nil, "")

	if got.status != 0 {
		t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
	}
	forwarded := strings.Split(strings.TrimSuffix(f.readCapture(f.argsPath), "\n"), "\n")
	if !reflect.DeepEqual(forwarded, args) {
		t.Errorf("forwarded args = %q, want %q", forwarded, args)
	}
	f.assertClean()
}

func TestPreservesWorkingDirectory(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	workDir := filepath.Join(f.tempDir, "some work dir")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("SECRET=leaked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "bunfig.toml"), []byte("[install]\nregistry = 'https://example.com'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.withChecksum("agent-links-" + hostTarget())

	got := f.run(nil, nil, workDir)

	if got.status != 0 {
		t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
	}
	// macOS /var is a symlink to /private/var; compare real paths.
	captured, err := filepath.EvalSymlinks(strings.TrimSpace(f.readCapture(f.pwdPath)))
	if err != nil {
		t.Fatalf("resolve captured working directory: %v", err)
	}
	want, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	if captured != want {
		t.Errorf("asset working directory = %q, want %q", captured, want)
	}
	f.assertClean()
}

func TestWorksWithoutRuntimeOnPath(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	for _, name := range []string{"bun", "node", "go"} {
		if err := os.WriteFile(filepath.Join(f.binDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f.withChecksum("agent-links-" + hostTarget())

	got := f.run(nil, nil, "")

	if got.status != 0 {
		t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
	}
	if _, err := os.Stat(f.assetExe); err != nil {
		t.Errorf("asset was not executed: %v", err)
	}
	f.assertClean()
}

func TestChecksumMismatchNeverExecutesTheAsset(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	target := hostTarget()
	staged := strings.Repeat("0", 64) + "  agent-links-" + target + "\n"
	if err := os.WriteFile(filepath.Join(f.tempDir, "checksums-"+target+".txt"), []byte(staged), 0o644); err != nil {
		t.Fatal(err)
	}

	got := f.run(nil, nil, "")

	if got.status != 1 {
		t.Fatalf("status = %d, want 1", got.status)
	}
	if !strings.Contains(got.stderr, "checksum verification failed") {
		t.Errorf("stderr = %q, want a checksum failure", got.stderr)
	}
	if _, err := os.Stat(f.assetExe); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asset was executed despite the checksum mismatch: %v", err)
	}
	f.assertNoCacheFiles()
	f.assertClean()
}

func TestMissingChecksumEntryIsRejected(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.tempDir, "checksums-"+hostTarget()+".txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := f.run(nil, nil, "")

	if got.status != 1 {
		t.Fatalf("status = %d, want 1", got.status)
	}
	if !strings.Contains(got.stderr, "checksum") {
		t.Errorf("stderr = %q, want a checksum error", got.stderr)
	}
	if _, err := os.Stat(f.assetExe); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asset was executed without a checksum entry: %v", err)
	}
	f.assertNoCacheFiles()
	f.assertClean()
}

func TestDownloadFailureIsReported(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)

	got := f.run(nil, map[string]string{"CURL_FAIL": "1"}, "")

	if got.status != 1 {
		t.Fatalf("status = %d, want 1", got.status)
	}
	if !strings.Contains(got.stderr, "failed to download") {
		t.Errorf("stderr = %q, want a download failure", got.stderr)
	}
	if _, err := os.Stat(f.assetExe); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asset was executed despite the download failure: %v", err)
	}
	f.assertNoCacheFiles()
	f.assertClean()
}

func TestPartialDownloadIsNotExecuted(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)

	got := f.run(nil, map[string]string{"CURL_PARTIAL": "1"}, "")

	if got.status != 1 {
		t.Fatalf("status = %d, want 1", got.status)
	}
	if !strings.Contains(got.stderr, "failed to download") {
		t.Errorf("stderr = %q, want a download failure", got.stderr)
	}
	if _, err := os.Stat(f.assetExe); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("asset was executed despite the partial download: %v", err)
	}
	f.assertNoCacheFiles()
	f.assertClean()
}

func TestPropagatesExecutableFailureStatus(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())

	got := f.run(nil, map[string]string{"ASSET_EXIT": "7"}, "")

	if got.status != 7 {
		t.Fatalf("status = %d, want 7", got.status)
	}
	f.assertClean()
}

func TestCleansUpWhenTheExecutableCrashes(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	if err := os.WriteFile(f.assetSrc, []byte("#!/bin/sh\nexit 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.withChecksum("agent-links-" + hostTarget())

	got := f.run(nil, nil, "")

	if got.status != 99 {
		t.Fatalf("status = %d, want 99", got.status)
	}
	f.assertClean()
}

func TestReportsLaunchProblemForInvalidExecutable(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	if err := os.WriteFile(f.assetSrc, []byte("#!/does-not-exist/agent-links\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.withChecksum("agent-links-" + hostTarget())

	got := f.run(nil, nil, "")

	// Shells differ on the status for a missing shebang interpreter: bash on
	// macOS reports 126, while Linux shells commonly report 127.
	if got.status != 126 && got.status != 127 {
		t.Fatalf("status = %d, want 126 or 127", got.status)
	}
	if !strings.Contains(got.stderr, "could not launch") {
		t.Errorf("stderr = %q, want a launch error", got.stderr)
	}
	f.assertClean()
}

func TestCacheFirstRunDownloadsAndSecondRunDoesNot(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())

	first := f.run(nil, nil, "")
	if first.status != 0 {
		t.Fatalf("first run status = %d, stderr:\n%s", first.status, first.stderr)
	}
	if calls := f.downloadCalls(); calls != 2 {
		t.Errorf("first run downloads = %d, want 2", calls)
	}
	if _, err := os.Stat(f.cacheAsset()); err != nil {
		t.Fatalf("cached asset missing: %v", err)
	}
	f.assertModes()

	f.resetDownloads()
	// CURL_FAIL proves the second run also succeeds offline.
	second := f.run(nil, map[string]string{"CURL_FAIL": "1"}, "")
	if second.status != 0 {
		t.Fatalf("second run status = %d, stderr:\n%s", second.status, second.stderr)
	}
	if calls := f.downloadCalls(); calls != 0 {
		t.Errorf("second run downloads = %d, want 0", calls)
	}
	if _, err := os.Stat(f.assetExe); err != nil {
		t.Errorf("cached asset was not executed: %v", err)
	}
	f.assertModes()
	f.assertClean()
}

func TestCacheTamperedAssetIsRedownloaded(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	if first := f.run(nil, nil, ""); first.status != 0 {
		t.Fatalf("first run status = %d, stderr:\n%s", first.status, first.stderr)
	}

	if err := os.WriteFile(f.cacheAsset(), []byte("#!/bin/sh\nexit 42\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.resetDownloads()
	second := f.run(nil, nil, "")
	if second.status != 0 {
		t.Fatalf("second run status = %d, stderr:\n%s", second.status, second.stderr)
	}
	if calls := f.downloadCalls(); calls != 2 {
		t.Errorf("downloads after tampering = %d, want 2", calls)
	}
	got, err := os.ReadFile(f.cacheAsset())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(f.assetSrc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("cached asset was not restored after tampering")
	}
	f.assertModes()
	f.assertClean()
}

func TestCacheMissingOrTruncatedChecksumIsRedownloaded(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, f *fixture)
	}{
		{name: "missing", mutate: func(t *testing.T, f *fixture) {
			if err := os.Remove(f.cacheSums()); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "truncated", mutate: func(t *testing.T, f *fixture) {
			if err := os.WriteFile(f.cacheSums(), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireHashTool(t)
			f := newFixture(t)
			f.withChecksum("agent-links-" + hostTarget())
			if first := f.run(nil, nil, ""); first.status != 0 {
				t.Fatalf("first run status = %d, stderr:\n%s", first.status, first.stderr)
			}
			tc.mutate(t, f)
			f.resetDownloads()

			second := f.run(nil, nil, "")
			if second.status != 0 {
				t.Fatalf("second run status = %d, stderr:\n%s", second.status, second.stderr)
			}
			if calls := f.downloadCalls(); calls != 2 {
				t.Errorf("downloads after a %s checksum = %d, want 2", tc.name, calls)
			}
			if _, err := os.Stat(f.cacheSums()); err != nil {
				t.Errorf("checksum cache was not restored: %v", err)
			}
			f.assertModes()
			f.assertClean()
		})
	}
}

func TestCacheRejectsSymlinkedEntries(t *testing.T) {
	t.Run("asset symlink", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		if first := f.run(nil, nil, ""); first.status != 0 {
			t.Fatalf("first run status = %d, stderr:\n%s", first.status, first.stderr)
		}
		if err := os.Remove(f.cacheAsset()); err != nil {
			t.Fatal(err)
		}
		// The symlink target is a valid asset, so accepting the link would
		// pass the checksum; the cache must reject it anyway.
		if err := os.Symlink(f.assetSrc, f.cacheAsset()); err != nil {
			t.Fatal(err)
		}
		f.resetDownloads()

		second := f.run(nil, nil, "")
		if second.status != 0 {
			t.Fatalf("second run status = %d, stderr:\n%s", second.status, second.stderr)
		}
		if calls := f.downloadCalls(); calls != 2 {
			t.Errorf("downloads after a symlinked asset = %d, want 2", calls)
		}
		info, err := os.Lstat(f.cacheAsset())
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Errorf("cached asset mode = %v, want a regular file", info.Mode())
		}
		f.assertModes()
		f.assertClean()
	})

	t.Run("checksum symlink", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		if first := f.run(nil, nil, ""); first.status != 0 {
			t.Fatalf("first run status = %d, stderr:\n%s", first.status, first.stderr)
		}
		if err := os.Remove(f.cacheSums()); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(f.env["CURL_CHECKSUM_FILE"], f.cacheSums()); err != nil {
			t.Fatal(err)
		}
		f.resetDownloads()

		second := f.run(nil, nil, "")
		if second.status != 0 {
			t.Fatalf("second run status = %d, stderr:\n%s", second.status, second.stderr)
		}
		if calls := f.downloadCalls(); calls != 2 {
			t.Errorf("downloads after a symlinked checksum = %d, want 2", calls)
		}
		info, err := os.Lstat(f.cacheSums())
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Errorf("cached checksum mode = %v, want a regular file", info.Mode())
		}
		f.assertModes()
		f.assertClean()
	})
}

func TestCachePermissions(t *testing.T) {
	t.Run("created private", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		if got := f.run(nil, nil, ""); got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		f.assertModes()
	})

	t.Run("pre-existing world-writable leaf is corrected", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		if err := os.MkdirAll(f.cacheLeaf(), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(f.cacheLeaf(), 0o777); err != nil {
			t.Fatal(err)
		}
		if got := f.run(nil, nil, ""); got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		f.assertModes()
	})

	t.Run("leaf symlink falls back to mktemp", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		versionDir := filepath.Dir(f.cacheLeaf())
		if err := os.MkdirAll(versionDir, 0o700); err != nil {
			t.Fatal(err)
		}
		elsewhere := filepath.Join(f.tempDir, "elsewhere")
		if err := os.MkdirAll(elsewhere, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, f.cacheLeaf()); err != nil {
			t.Fatal(err)
		}

		got := f.run(nil, nil, "")
		if got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		entries, err := os.ReadDir(elsewhere)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("cache wrote through a leaf symlink: %v", entries)
		}
		f.assertClean()
	})
}

func TestCacheRootResolution(t *testing.T) {
	t.Run("absolute xdg root with spaces", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		xdg := filepath.Join(f.tempDir, "xdg cache")
		got := f.run(nil, map[string]string{"XDG_CACHE_HOME": xdg}, "")
		if got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		leaf := filepath.Join(xdg, "agent-links", "v"+version, hostTarget())
		for _, path := range []string{
			filepath.Join(leaf, "agent-links-"+hostTarget()),
			filepath.Join(leaf, "checksums-"+hostTarget()+".txt"),
		} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("expected cache at %s: %v", path, err)
			}
		}
		if _, err := os.Stat(filepath.Join(f.homeDir, ".cache", "agent-links")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("default cache was used despite an absolute XDG_CACHE_HOME: %v", err)
		}
	})

	t.Run("relative xdg falls back to home", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		got := f.run(nil, map[string]string{"XDG_CACHE_HOME": "relative-cache"}, "")
		if got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		if _, err := os.Stat(f.cacheAsset()); err != nil {
			t.Errorf("default home cache missing: %v", err)
		}
		if _, err := os.Stat(filepath.Join(f.tempDir, "relative-cache")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("relative XDG_CACHE_HOME was used: %v", err)
		}
	})
}

func TestCacheRootFallbacks(t *testing.T) {
	t.Run("group-writable cache root", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		root := filepath.Join(f.tempDir, "shared-cache")
		if err := os.MkdirAll(root, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o777); err != nil {
			t.Fatal(err)
		}
		got := f.run(nil, map[string]string{"XDG_CACHE_HOME": root}, "")
		if got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		if _, err := os.Stat(filepath.Join(root, "agent-links")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("an unsafe cache root was used: %v", err)
		}
		if _, err := os.Stat(f.assetExe); err != nil {
			t.Errorf("asset was not executed from the fallback: %v", err)
		}
		f.assertClean()
	})

	t.Run("cache root is a regular file", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		blocked := filepath.Join(f.tempDir, "cache-file")
		if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := f.run(nil, map[string]string{"XDG_CACHE_HOME": blocked}, "")
		if got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		if _, err := os.Stat(f.assetExe); err != nil {
			t.Errorf("asset was not executed from the fallback: %v", err)
		}
		f.assertClean()
	})

	t.Run("home unset", func(t *testing.T) {
		requireHashTool(t)
		f := newFixture(t)
		f.withChecksum("agent-links-" + hostTarget())
		delete(f.env, "HOME")
		delete(f.env, "XDG_CACHE_HOME")
		got := f.run(nil, nil, "")
		if got.status != 0 {
			t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
		}
		if _, err := os.Stat(f.assetExe); err != nil {
			t.Errorf("asset was not executed from the fallback: %v", err)
		}
		f.assertClean()
	})
}

// TestWarmCacheNeedsNoCurlOrMktemp removes both tools from PATH: a verified
// cache hit must run with only the shell, SHA-256, and core file utilities.
func TestWarmCacheNeedsNoCurlOrMktemp(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	if first := f.run(nil, nil, ""); first.status != 0 {
		t.Fatalf("first run status = %d, stderr:\n%s", first.status, first.stderr)
	}

	minimal := filepath.Join(f.tempDir, "minimal-bin")
	if err := os.MkdirAll(minimal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.binDir, "uname"), filepath.Join(minimal, "uname")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"id", "shasum", "sha256sum", "find", "awk", "chmod", "mkdir", "rm", "mv"} {
		if name == "sha256sum" && !lookPathIn(systemPath, name) {
			continue
		}
		if name == "shasum" && !lookPathIn(systemPath, name) {
			continue
		}
		source := executableIn(systemPath, name)
		if source == "" {
			t.Fatalf("%s is unavailable on %s", name, systemPath)
		}
		if err := os.Symlink(source, filepath.Join(minimal, name)); err != nil {
			t.Fatal(err)
		}
	}

	f.resetDownloads()
	second := f.run(nil, map[string]string{"PATH": minimal, "CURL_FAIL": "1"}, "")
	if second.status != 0 {
		t.Fatalf("warm-cache run status = %d, stderr:\n%s", second.status, second.stderr)
	}
	if calls := f.downloadCalls(); calls != 0 {
		t.Errorf("warm-cache downloads = %d, want 0", calls)
	}
	if _, err := os.Stat(f.assetExe); err != nil {
		t.Errorf("cached asset was not executed: %v", err)
	}
}

// TestCacheWorksUnderDash guards the portable ownership check: dash's test
// rejects -O, so the script uses find -user instead. CI's Linux runner has
// dash; macOS skips this test.
func TestCacheWorksUnderDash(t *testing.T) {
	requireHashTool(t)
	dash, err := exec.LookPath("dash")
	if err != nil {
		t.Skip("dash is unavailable")
	}
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	if first := f.run(nil, nil, ""); first.status != 0 {
		t.Fatalf("warm-up run status = %d, stderr:\n%s", first.status, first.stderr)
	}
	f.resetDownloads()

	cmd := exec.Command(dash, runSh)
	cmd.Dir = f.tempDir
	cmd.Env = f.envWith(map[string]string{"CURL_FAIL": "1"})
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("dash warm-cache run failed: %v\nstderr:\n%s", err, stderr.String())
	}
	if calls := f.downloadCalls(); calls != 0 {
		t.Errorf("dash warm-cache downloads = %d, want 0", calls)
	}
	if _, err := os.Stat(f.assetExe); err != nil {
		t.Errorf("cached asset was not executed under dash: %v", err)
	}
}

func TestCachePreservesCallerUmask(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	umaskCapture := filepath.Join(f.tempDir, "asset-umask")
	wrapper := filepath.Join(f.tempDir, "umask-wrapper")
	f.writeExecutable(wrapper, "#!/bin/sh\numask 022\nexec bash "+shellQuote(runSh)+" \"$@\"\n")

	cmd := exec.Command(wrapper)
	cmd.Dir = f.tempDir
	cmd.Env = f.envWith(map[string]string{"UMASK_CAPTURE": umaskCapture})
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("wrapper run failed: %v\nstderr:\n%s", err, stderr.String())
	}

	got := strings.TrimSpace(f.readCapture(umaskCapture))
	if got != "0022" && got != "022" {
		t.Errorf("child umask = %q, want 0022", got)
	}
	// The cache stays private even though the caller used umask 022.
	f.assertModes()
	f.assertClean()
}

func TestPipelinesRunShThroughAShell(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	script, err := os.ReadFile(runSh)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-s", "--")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Dir = f.tempDir
	cmd.Env = f.envWith(nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("piped run.sh failed: %v\nstderr:\n%s", err, stderr.String())
	}

	if _, err := os.Stat(f.assetExe); err != nil {
		t.Errorf("asset was not executed: %v", err)
	}
	f.assertClean()
}

func TestWithoutTTYInputIsNotRedirected(t *testing.T) {
	requireHashTool(t)
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	ttyCapture := filepath.Join(f.tempDir, "asset-stdin-tty")

	got := f.run(nil, map[string]string{"ASSET_STDIN_TTY": ttyCapture}, "")

	if got.status != 0 {
		t.Fatalf("status = %d, stderr:\n%s", got.status, got.stderr)
	}
	if contents := f.readCapture(ttyCapture); contents != "not-tty" {
		t.Errorf("asset stdin marker = %q, want \"not-tty\"", contents)
	}
	f.assertClean()
}

// TestRestoresInteractiveInputFromTTY keeps the runner's </dev/tty redirect:
// the binary, not the runner, now opens /dev/tty for Bubble Tea input, and the
// redirect still makes the later plain prompts see a terminal.
func TestRestoresInteractiveInputFromTTY(t *testing.T) {
	requireHashTool(t)
	form := pickScriptForm()
	if form == nil {
		t.Skip("script(1) is unavailable")
	}
	f := newFixture(t)
	f.withChecksum("agent-links-" + hostTarget())
	ttyCapture := filepath.Join(f.tempDir, "asset-stdin-tty")
	transcript := filepath.Join(f.tempDir, "transcript")
	command := "cat " + shellQuote(runSh) + " | bash"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "script", form(command, transcript)...)
	cmd.Dir = f.tempDir
	cmd.Env = f.envWith(map[string]string{"ASSET_STDIN_TTY": ttyCapture})
	// The pty keeps stdin attached to the terminal for the whole pipeline.
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("script(1) failed: %v\n%s", err, output)
	}

	if contents := f.readCapture(ttyCapture); contents != "tty" {
		t.Errorf("asset stdin marker = %q, want \"tty\"", contents)
	}
	f.assertClean()
}

// pickScriptForm returns the argument builder for the local script(1) dialect:
// BSD (macOS) takes the command as trailing arguments, util-linux takes -c.
func pickScriptForm() func(command, transcript string) []string {
	forms := []func(command, transcript string) []string{
		func(command, transcript string) []string {
			return []string{"-q", transcript, "sh", "-c", command}
		},
		func(command, transcript string) []string {
			return []string{"-qec", command, transcript}
		},
	}
	for _, form := range forms {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := exec.CommandContext(ctx, "script", form("true", os.DevNull)...).Run()
		cancel()
		if err == nil {
			return form
		}
	}
	return nil
}

// shellQuote wraps s in single quotes for POSIX shells.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
