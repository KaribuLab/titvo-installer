package internal

import (
	"os"
	"testing"
)

func TestMergeEnvOverridesExistingKeys(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/tmp/home"}
	overrides := map[string]string{
		"PATH":       "/custom/bin",
		"TG_TF_PATH": "/custom/bin/terraform",
	}

	env := mergeEnv(base, overrides)

	got := map[string]string{}
	for _, kv := range env {
		key, value, ok := cutEnv(kv)
		if !ok {
			t.Fatalf("invalid env entry: %q", kv)
		}
		got[key] = value
	}

	if got["PATH"] != "/custom/bin" {
		t.Fatalf("PATH = %q, want /custom/bin", got["PATH"])
	}
	if got["HOME"] != "/tmp/home" {
		t.Fatalf("HOME = %q, want /tmp/home", got["HOME"])
	}
	if got["TG_TF_PATH"] != "/custom/bin/terraform" {
		t.Fatalf("TG_TF_PATH = %q, want /custom/bin/terraform", got["TG_TF_PATH"])
	}
}

func cutEnv(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return "", "", false
}

func TestMergeEnvMatchesProcessEnviron(t *testing.T) {
	env := mergeEnv(os.Environ(), map[string]string{"TITVO_TEST_VAR": "ok"})

	found := false
	for _, kv := range env {
		key, value, ok := cutEnv(kv)
		if ok && key == "TITVO_TEST_VAR" && value == "ok" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected override var in merged env")
	}
}
