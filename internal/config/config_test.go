package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKey = "0123456789012345678901234567890123"

// clear 真正删除环境变量（t.Setenv 只会设成空串，而空串会被当作「已设置」跳过配置文件）。
func clear(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, ok := os.LookupEnv(k)
		os.Unsetenv(k)
		if ok {
			t.Cleanup(func() { os.Setenv(k, old) })
		}
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigFileApplies(t *testing.T) {
	clear(t, "CONFIG_FILE", "PORT", "DB_DSN", "DB_DRIVER", "MASTER_KEY", "MASTER_KEY_OLD", "SCAN_ENABLED")
	t.Setenv("CONFIG_FILE", writeConfig(t, `
# 配置项名 = 同名环境变量，大小写不敏感
port: 9999
db_driver: mysql
db_dsn: file-dsn
master_key: "0123456789012345678901234567890123"
master_key_old:
  - aaa
  - bbb
scan_enabled: false
`))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9999" || cfg.DBDriver != "mysql" || cfg.DBDSN != "file-dsn" {
		t.Fatalf("配置文件未生效: %+v", cfg)
	}
	if cfg.ScanEnabled {
		t.Fatal("布尔值 false 应覆盖默认 true")
	}
	if len(cfg.MasterKeyOld) != 2 || cfg.MasterKeyOld[0] != "aaa" || cfg.MasterKeyOld[1] != "bbb" {
		t.Fatalf("列表项应合并为逗号分隔: %v", cfg.MasterKeyOld)
	}
}

func TestEnvBeatsConfigFile(t *testing.T) {
	clear(t, "CONFIG_FILE", "PORT", "MASTER_KEY")
	t.Setenv("CONFIG_FILE", writeConfig(t, "port: 9999\nmaster_key: \""+testKey+"\"\n"))
	t.Setenv("PORT", "7777")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "7777" {
		t.Fatalf("环境变量应优先于配置文件，实际 %s", cfg.Port)
	}
}

func TestExplicitConfigFileMustExist(t *testing.T) {
	clear(t, "CONFIG_FILE")
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "nope.yaml"))
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("显式指定的配置文件缺失应当报错，实际 %v", err)
	}
}

func TestNestedConfigRejected(t *testing.T) {
	clear(t, "CONFIG_FILE", "MASTER_KEY")
	t.Setenv("CONFIG_FILE", writeConfig(t, "database:\n  dsn: x\n"))
	t.Setenv("MASTER_KEY", testKey)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "嵌套") {
		t.Fatalf("嵌套结构应当报错而不是被静默忽略，实际 %v", err)
	}
}

func TestUnknownKeysDetected(t *testing.T) {
	clear(t, "CONFIG_FILE", "PORT", "MASTER_KEY")
	t.Setenv("CONFIG_FILE", writeConfig(t, "port: 1234\nmaster_key: \""+testKey+"\"\n"))
	t.Setenv("MASTER_KEY", testKey)
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	got := unknownKeys(map[string]string{"PORT": "1234", "DB_DSN_TYPO": "x"})
	if len(got) != 1 || got[0] != "DB_DSN_TYPO" {
		t.Fatalf("应只报出拼错的项，实际 %v", got)
	}
}

func TestMissingMasterKey(t *testing.T) {
	clear(t, "CONFIG_FILE", "MASTER_KEY")
	t.Setenv("CONFIG_FILE", writeConfig(t, "port: 1234\n"))
	t.Setenv("MASTER_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("MASTER_KEY 缺失应当报错")
	}
}
