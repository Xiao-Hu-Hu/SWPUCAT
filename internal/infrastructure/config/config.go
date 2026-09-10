package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Log      LogConfig      `mapstructure:"log"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Email    EmailConfig    `mapstructure:"email"`
}

// ValidateForServer checks the values that are required by the HTTP service.
// Secrets intentionally have no insecure code defaults; they must be supplied
// by the process environment (or a local .env file).
func (c *Config) ValidateForServer() error {
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if len(c.JWT.AccessSecret) < 32 || len(c.JWT.RefreshSecret) < 32 {
		return fmt.Errorf("jwt access_secret and refresh_secret must each be at least 32 characters")
	}
	if c.JWT.AccessSecret == c.JWT.RefreshSecret {
		return fmt.Errorf("jwt access_secret and refresh_secret must be different")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.Database.MaxOpenConns < 1 || c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("database connection pool limits are invalid")
	}
	return nil
}

// ValidateForMigration is deliberately narrower than server validation: a
// migration process does not need JWT or SMTP configuration.
func (c *Config) ValidateForMigration() error {
	return c.Database.Validate()
}

type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

func (d *DatabaseConfig) Validate() error {
	if strings.TrimSpace(d.Host) == "" {
		return fmt.Errorf("database.host is required")
	}
	if d.Port < 1 || d.Port > 65535 {
		return fmt.Errorf("database.port must be between 1 and 65535")
	}
	if strings.TrimSpace(d.User) == "" {
		return fmt.Errorf("database.user is required")
	}
	if d.Password == "" {
		return fmt.Errorf("database.password is required and must be supplied through the environment")
	}
	if strings.TrimSpace(d.DBName) == "" {
		return fmt.Errorf("database.dbname is required")
	}
	if strings.TrimSpace(d.SSLMode) == "" {
		return fmt.Errorf("database.sslmode is required")
	}
	return nil
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	AccessSecret  string        `mapstructure:"access_secret"`
	RefreshSecret string        `mapstructure:"refresh_secret"`
	AccessExpiry  time.Duration `mapstructure:"access_expiry"`
	RefreshExpiry time.Duration `mapstructure:"refresh_expiry"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type StorageConfig struct {
	UploadDir   string `mapstructure:"upload_dir"`
	MaxFileSize int64  `mapstructure:"max_file_size"`
}

type EmailConfig struct {
	SMTPHost string `mapstructure:"smtp_host"`
	SMTPPort int    `mapstructure:"smtp_port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
}

func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CSA")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("server.read_timeout", 10*time.Second)
	viper.SetDefault("server.write_timeout", 10*time.Second)
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "csa")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "csa_db")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_open_conns", 15)
	viper.SetDefault("database.max_idle_conns", 3)
	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("jwt.access_expiry", 15*time.Minute)
	viper.SetDefault("jwt.refresh_expiry", 7*24*time.Hour)
	viper.SetDefault("jwt.access_secret", "")
	viper.SetDefault("jwt.refresh_secret", "")
	viper.SetDefault("log.level", "debug")
	viper.SetDefault("log.format", "console")
	viper.SetDefault("storage.upload_dir", "./uploads")
	viper.SetDefault("storage.max_file_size", 100*1024*1024) // 100MB
	viper.SetDefault("email.smtp_host", "")
	viper.SetDefault("email.smtp_port", 587)
	viper.SetDefault("email.username", "")
	viper.SetDefault("email.password", "")
	viper.SetDefault("email.from", "")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Viper's AutomaticEnv does not reliably populate nested structs during
	// Unmarshal, so all supported process overrides are bound explicitly.
	applyStringEnv("CSA_DATABASE_HOST", &cfg.Database.Host)
	applyIntEnv("CSA_DATABASE_PORT", &cfg.Database.Port)
	applyStringEnv("CSA_DATABASE_USER", &cfg.Database.User)
	applyStringEnv("CSA_DATABASE_PASSWORD", &cfg.Database.Password)
	applyStringEnv("CSA_DATABASE_DBNAME", &cfg.Database.DBName)
	applyStringEnv("CSA_DATABASE_SSLMODE", &cfg.Database.SSLMode)
	applyIntEnv("CSA_DATABASE_MAX_OPEN_CONNS", &cfg.Database.MaxOpenConns)
	applyIntEnv("CSA_DATABASE_MAX_IDLE_CONNS", &cfg.Database.MaxIdleConns)
	applyStringEnv("CSA_JWT_ACCESS_SECRET", &cfg.JWT.AccessSecret)
	applyStringEnv("CSA_JWT_REFRESH_SECRET", &cfg.JWT.RefreshSecret)
	applyStringEnv("CSA_EMAIL_SMTP_HOST", &cfg.Email.SMTPHost)
	applyIntEnv("CSA_EMAIL_SMTP_PORT", &cfg.Email.SMTPPort)
	applyStringEnv("CSA_EMAIL_USERNAME", &cfg.Email.Username)
	applyStringEnv("CSA_EMAIL_PASSWORD", &cfg.Email.Password)
	applyStringEnv("CSA_EMAIL_FROM", &cfg.Email.From)
	applyStringEnv("CSA_STORAGE_UPLOAD_DIR", &cfg.Storage.UploadDir)
	applyInt64Env("CSA_STORAGE_MAX_FILE_SIZE", &cfg.Storage.MaxFileSize)
	applyIntEnv("CSA_SERVER_PORT", &cfg.Server.Port)
	applyStringEnv("CSA_SERVER_MODE", &cfg.Server.Mode)
	applyDurationEnv("CSA_SERVER_READ_TIMEOUT", &cfg.Server.ReadTimeout)
	applyDurationEnv("CSA_SERVER_WRITE_TIMEOUT", &cfg.Server.WriteTimeout)
	applyStringEnv("CSA_LOG_LEVEL", &cfg.Log.Level)
	applyStringEnv("CSA_LOG_FORMAT", &cfg.Log.Format)

	return &cfg, nil
}

func applyStringEnv(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

func applyIntEnv(key string, target *int) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}

func applyInt64Env(key string, target *int64) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			*target = parsed
		}
	}
}

func applyDurationEnv(key string, target *time.Duration) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			*target = parsed
		}
	}
}
