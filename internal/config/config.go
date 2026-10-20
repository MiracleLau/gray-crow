package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // 内嵌时区库，SCAN_TIMEZONE 不依赖宿主 tzdata

	"github.com/goccy/go-yaml"
)

type Config struct {
	Port            string
	DBDriver        string
	DBDSN           string
	MasterKey       string
	MasterKeyOld    []string
	SessionTTLHours int
	CookieSecure    bool
	AdminUser       string
	AdminPassword   string
	ScanEnabled     bool
	ScanMinInterval int
	ScanTimezone    string
	APIRatePerMin   int
	GitHubAPIBase   string
	LogLevel        string
	StaticDir       string
}

func Load() (*Config, error) {
	loadDotEnv(".env")
	seen = map[string]bool{}
	used, err := loadConfigFile()
	if err != nil {
		return nil, err
	}
	c := &Config{
		Port:            env("PORT", "8080"),
		DBDriver:        env("DB_DRIVER", "sqlite"),
		DBDSN:           env("DB_DSN", "data/graycrow.db"),
		MasterKey:       env("MASTER_KEY", ""),
		MasterKeyOld:    splitEnv("MASTER_KEY_OLD"),
		SessionTTLHours: envInt("SESSION_TTL_HOURS", 168),
		CookieSecure:    envBool("COOKIE_SECURE", false),
		AdminUser:       env("ADMIN_USER", "admin"),
		AdminPassword:   env("ADMIN_PASSWORD", "admin123"),
		ScanEnabled:     envBool("SCAN_ENABLED", true),
		ScanMinInterval: envInt("SCAN_MIN_INTERVAL_SEC", 60),
		ScanTimezone:    env("SCAN_TIMEZONE", "Asia/Shanghai"),
		APIRatePerMin:   envInt("API_RATE_PER_MIN", 8),
		GitHubAPIBase:   env("GITHUB_API_BASE", ""),
		LogLevel:        env("LOG_LEVEL", "info"),
		StaticDir:       env("STATIC_DIR", ""),
	}
	if strings.TrimSpace(c.MasterKey) == "" {
		return nil, fmt.Errorf("MASTER_KEY 未设置：请生成一个 >=32 字符的随机串（用于加密存储的密钥字段）")
	}
	if len(c.MasterKey) < 32 {
		return nil, fmt.Errorf("MASTER_KEY 长度不足 32 字符")
	}
	switch c.DBDriver {
	case "sqlite", "mysql", "postgres":
	default:
		return nil, fmt.Errorf("不支持的 DB_DRIVER: %s", c.DBDriver)
	}
	if unknown := unknownKeys(used); len(unknown) > 0 {
		log.Printf("配置文件里有无法识别的配置项（已忽略，配置项名须与同名环境变量一致）: %s", strings.Join(unknown, ", "))
	}
	if _, err := time.LoadLocation(c.ScanTimezone); err != nil {
		return nil, fmt.Errorf("SCAN_TIMEZONE 无效（%s）: %w", c.ScanTimezone, err)
	}
	return c, nil
}

// loadConfigFile 读取 YAML 配置文件并写入环境变量，已存在的环境变量（含 .env）优先。
// 配置项名与同名环境变量一一对应（大小写不敏感），因此不必为配置文件的每一项单独写字段。
func loadConfigFile() (map[string]string, error) {
	path := env("CONFIG_FILE", "config.yaml")
	explicit := strings.TrimSpace(os.Getenv("CONFIG_FILE")) != ""
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if explicit {
			return nil, fmt.Errorf("配置文件 %s 不存在（CONFIG_FILE 已显式指定）", path)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	used := make(map[string]string, len(parsed))
	for key, value := range parsed {
		key = strings.ToUpper(strings.TrimSpace(key))
		if key == "" || value == nil {
			continue
		}
		switch v := value.(type) {
		case map[string]any:
			return nil, fmt.Errorf("配置文件 %s 的 %s 是嵌套结构；配置项须平铺，名称与同名环境变量一致（如 db_dsn）", path, key)
		case []any:
			items := make([]string, 0, len(v))
			for _, item := range v {
				items = append(items, fmt.Sprint(item))
			}
			value = strings.Join(items, ",")
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			continue
		}
		used[key] = text
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, text)
		}
	}
	if len(used) > 0 {
		log.Printf("已加载配置文件 %s（%d 项；环境变量与 .env 优先）", path, len(used))
	}
	return used, nil
}

// unknownKeys 找出配置文件里没被任何配置读取消费的项，通常意味着拼写错误。
func unknownKeys(used map[string]string) []string {
	out := []string{}
	for key := range used {
		if !seen[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// loadDotEnv 逐行解析 KEY=VALUE，已存在的环境变量优先（不覆盖）。
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" || value == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

// splitEnv 逗号分隔（密钥轮换时可能同时保留多把旧密钥）。
func splitEnv(key string) []string {
	seen[key] = true
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// seen 记录本次 Load 读过的配置项，用于发现配置文件里的拼写错误（避免静默忽略）。
var seen map[string]bool

func env(key, def string) string {
	seen[key] = true
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(env(key, "")); err == nil {
		return v
	}
	return def
}
