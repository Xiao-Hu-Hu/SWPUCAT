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
	godotenv.Load()

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
	viper.SetDefault("database.password", "csa_password")
	viper.SetDefault("database.dbname", "csa_db")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_open_conns", 100)
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("redis.addr", "localhost:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("jwt.access_expiry", 15*time.Minute)
	viper.SetDefault("jwt.refresh_expiry", 7*24*time.Hour)
	viper.SetDefault("jwt.access_secret", "dev-access-secret-change-in-production")
	viper.SetDefault("jwt.refresh_secret", "dev-refresh-secret-change-in-production")
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

	// Override with environment variables if set
	if host := os.Getenv("CSA_DATABASE_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if port := os.Getenv("CSA_DATABASE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Database.Port = p
		}
	}
	if user := os.Getenv("CSA_DATABASE_USER"); user != "" {
		cfg.Database.User = user
	}
	if password := os.Getenv("CSA_DATABASE_PASSWORD"); password != "" {
		cfg.Database.Password = password
	}
	if dbname := os.Getenv("CSA_DATABASE_DBNAME"); dbname != "" {
		cfg.Database.DBName = dbname
	}
	if secret := os.Getenv("CSA_JWT_ACCESS_SECRET"); secret != "" {
		cfg.JWT.AccessSecret = secret
	}
	if secret := os.Getenv("CSA_JWT_REFRESH_SECRET"); secret != "" {
		cfg.JWT.RefreshSecret = secret
	}
	if host := os.Getenv("CSA_EMAIL_SMTP_HOST"); host != "" {
		cfg.Email.SMTPHost = host
	}
	if port := os.Getenv("CSA_EMAIL_SMTP_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Email.SMTPPort = p
		}
	}
	if username := os.Getenv("CSA_EMAIL_USERNAME"); username != "" {
		cfg.Email.Username = username
	}
	if password := os.Getenv("CSA_EMAIL_PASSWORD"); password != "" {
		cfg.Email.Password = password
	}
	if from := os.Getenv("CSA_EMAIL_FROM"); from != "" {
		cfg.Email.From = from
	}

	return &cfg, nil
}
