package internal

import (
	"os"
	"testing"
)

func TestConfigDir(t *testing.T) {
	key := gopsConfigDirEnvKey
	oldDir := os.Getenv(key)
	defer os.Setenv(key, oldDir)

	newDir := "foo-bar"
	os.Setenv(key, newDir)
	configDir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	if g, w := configDir, newDir; g != w {
		t.Errorf("ConfigDir: got=%v want=%v", g, w)
	}
}
