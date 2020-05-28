package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir(t *testing.T) {
	configDir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	if g, w := filepath.Base(configDir), "gops"; g != w {
		t.Errorf("ConfigDir: got base directory %q, want %q", g, w)
	}

	key := gopsConfigDirEnvKey
	oldDir := os.Getenv(key)
	defer os.Setenv(key, oldDir)

	newDir := "foo-bar"
	os.Setenv(key, newDir)
	configDir, err = ConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	if g, w := configDir, newDir; g != w {
		t.Errorf("ConfigDir: got=%v want=%v", g, w)
	}
}
