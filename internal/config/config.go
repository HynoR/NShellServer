package config

type Config struct {
	Addr     string
	CertFile string
	KeyFile  string
	DBPath   string
	LogLevel string
	Dev      bool
	SelfHost bool
	Public   bool
}

func DefaultConfig(getenv func(string) string) *Config {
	return &Config{
		Addr:     envOrDefault(getenv, "NSHELL_ADDR", ":8443"),
		DBPath:   envOrDefault(getenv, "NSHELL_DB", "./nshell.db"),
		LogLevel: envOrDefault(getenv, "NSHELL_LOG_LEVEL", "info"),
	}
}

func ApplyTLSEnv(c *Config, getenv func(string) string) {
	if c.Dev || c.SelfHost {
		return
	}
	if c.CertFile == "" {
		c.CertFile = envOrDefault(getenv, "NSHELL_CERT", "")
	}
	if c.KeyFile == "" {
		c.KeyFile = envOrDefault(getenv, "NSHELL_KEY", "")
	}
}

func envOrDefault(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}
