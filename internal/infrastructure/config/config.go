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
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Log       LogConfig       `mapstructure:"log"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Email     EmailConfig     `mapstructure:"email"`
	Agent     AgentConfig     `mapstructure:"agent"`
	Embedding EmbeddingConfig `mapstructure:"embedding"`
	Retrieval RetrievalConfig `mapstructure:"retrieval"`
}

type EmbeddingConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Provider   string        `mapstructure:"provider"`
	Model      string        `mapstructure:"model"`
	APIKey     string        `mapstructure:"api_key"`
	BaseURL    string        `mapstructure:"base_url"`
	Dimensions int           `mapstructure:"dimensions"`
	BatchSize  int           `mapstructure:"batch_size"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

func (e EmbeddingConfig) Validate() error {
	if !e.Enabled {
		return nil
	}
	if e.Provider != "openai" && e.Provider != "openai_compatible" {
		return fmt.Errorf("embedding.provider must be openai or openai_compatible")
	}
	if strings.TrimSpace(e.Model) == "" {
		return fmt.Errorf("embedding.model is required when embedding is enabled")
	}
	if strings.TrimSpace(e.APIKey) == "" {
		return fmt.Errorf("embedding.api_key is required when embedding is enabled")
	}
	if strings.TrimSpace(e.BaseURL) == "" {
		return fmt.Errorf("embedding.base_url is required when embedding is enabled")
	}
	if e.Dimensions < 8 || e.Dimensions > 8192 {
		return fmt.Errorf("embedding.dimensions must be between 8 and 8192")
	}
	if e.BatchSize < 1 || e.BatchSize > 128 {
		return fmt.Errorf("embedding.batch_size must be between 1 and 128")
	}
	if e.Timeout <= 0 {
		return fmt.Errorf("embedding.timeout must be positive")
	}
	return nil
}

type RetrievalConfig struct {
	Mode              string        `mapstructure:"mode"`
	CandidateLimit    int           `mapstructure:"candidate_limit"`
	FinalLimit        int           `mapstructure:"final_limit"`
	MinVectorScore    float64       `mapstructure:"min_vector_score"`
	ChunkRunes        int           `mapstructure:"chunk_runes"`
	ChunkOverlapRunes int           `mapstructure:"chunk_overlap_runes"`
	MaxExtractedRunes int           `mapstructure:"max_extracted_runes"`
	WorkerInterval    time.Duration `mapstructure:"worker_interval"`
}

func (r RetrievalConfig) Validate(embeddingEnabled bool) error {
	if r.Mode != "keyword" && r.Mode != "vector" && r.Mode != "hybrid" {
		return fmt.Errorf("retrieval.mode must be keyword, vector, or hybrid")
	}
	if (r.Mode == "vector" || r.Mode == "hybrid") && !embeddingEnabled {
		return fmt.Errorf("retrieval.mode %s requires embedding.enabled=true", r.Mode)
	}
	if r.CandidateLimit < 1 || r.CandidateLimit > 100 {
		return fmt.Errorf("retrieval.candidate_limit must be between 1 and 100")
	}
	if r.FinalLimit < 1 || r.FinalLimit > r.CandidateLimit {
		return fmt.Errorf("retrieval.final_limit must be between 1 and candidate_limit")
	}
	if r.MinVectorScore < -1 || r.MinVectorScore > 1 {
		return fmt.Errorf("retrieval.min_vector_score must be between -1 and 1")
	}
	if r.ChunkRunes < 128 || r.ChunkRunes > 4000 {
		return fmt.Errorf("retrieval.chunk_runes must be between 128 and 4000")
	}
	if r.ChunkOverlapRunes < 0 || r.ChunkOverlapRunes >= r.ChunkRunes {
		return fmt.Errorf("retrieval.chunk_overlap_runes must be non-negative and less than chunk_runes")
	}
	if r.MaxExtractedRunes < r.ChunkRunes || r.MaxExtractedRunes > 5000000 {
		return fmt.Errorf("retrieval.max_extracted_runes must be between chunk_runes and 5000000")
	}
	if r.WorkerInterval <= 0 || r.WorkerInterval > time.Hour {
		return fmt.Errorf("retrieval.worker_interval must be positive and at most one hour")
	}
	return nil
}

type AgentConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	WriteEnabled      bool          `mapstructure:"write_enabled"`
	Port              int           `mapstructure:"port"`
	BaseURL           string        `mapstructure:"base_url"`
	ServerURL         string        `mapstructure:"server_url"`
	InternalSecret    string        `mapstructure:"internal_secret"`
	LLMProvider       string        `mapstructure:"llm_provider"`
	LLMModel          string        `mapstructure:"llm_model"`
	LLMAPIKey         string        `mapstructure:"llm_api_key"`
	LLMBaseURL        string        `mapstructure:"llm_base_url"`
	LLMTimeout        time.Duration `mapstructure:"llm_timeout"`
	MaxIterations     int           `mapstructure:"max_iterations"`
	MaxToolCalls      int           `mapstructure:"max_tool_calls"`
	WorkerConcurrency int           `mapstructure:"worker_concurrency"`
}

func (a *AgentConfig) Validate() error {
	if a.Port < 1 || a.Port > 65535 {
		return fmt.Errorf("agent.port must be between 1 and 65535")
	}
	if a.MaxIterations < 1 || a.MaxIterations > 32 {
		return fmt.Errorf("agent.max_iterations must be between 1 and 32")
	}
	if a.MaxToolCalls < 0 || a.MaxToolCalls > 64 {
		return fmt.Errorf("agent.max_tool_calls must be between 0 and 64")
	}
	if a.WorkerConcurrency < 1 || a.WorkerConcurrency > 16 {
		return fmt.Errorf("agent.worker_concurrency must be between 1 and 16")
	}
	if a.WriteEnabled && !a.Enabled {
		return fmt.Errorf("agent.write_enabled requires agent.enabled=true")
	}
	if a.Enabled {
		if strings.TrimSpace(a.LLMProvider) == "" {
			return fmt.Errorf("agent.llm_provider is required when agent is enabled")
		}
		if strings.TrimSpace(a.LLMModel) == "" {
			return fmt.Errorf("agent.llm_model is required when agent is enabled")
		}
		if strings.TrimSpace(a.LLMAPIKey) == "" {
			return fmt.Errorf("agent.llm_api_key is required when agent is enabled")
		}
		if a.LLMTimeout <= 0 {
			return fmt.Errorf("agent.llm_timeout must be positive")
		}
		if len(a.InternalSecret) < 32 {
			return fmt.Errorf("agent.internal_secret must be at least 32 characters when agent is enabled")
		}
	}
	return nil
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
	if err := c.Embedding.Validate(); err != nil {
		return err
	}
	if err := c.Retrieval.Validate(c.Embedding.Enabled); err != nil {
		return err
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
	viper.SetDefault("agent.enabled", false)
	viper.SetDefault("agent.write_enabled", false)
	viper.SetDefault("agent.port", 8090)
	viper.SetDefault("agent.base_url", "http://localhost:8090")
	viper.SetDefault("agent.server_url", "http://localhost:8080")
	viper.SetDefault("agent.internal_secret", "")
	viper.SetDefault("agent.llm_provider", "openai_compatible")
	viper.SetDefault("agent.llm_model", "")
	viper.SetDefault("agent.llm_api_key", "")
	viper.SetDefault("agent.llm_base_url", "")
	viper.SetDefault("agent.llm_timeout", 60*time.Second)
	viper.SetDefault("agent.max_iterations", 4)
	viper.SetDefault("agent.max_tool_calls", 8)
	viper.SetDefault("agent.worker_concurrency", 1)
	viper.SetDefault("embedding.enabled", false)
	viper.SetDefault("embedding.provider", "openai_compatible")
	viper.SetDefault("embedding.model", "")
	viper.SetDefault("embedding.api_key", "")
	viper.SetDefault("embedding.base_url", "")
	viper.SetDefault("embedding.dimensions", 1024)
	viper.SetDefault("embedding.batch_size", 16)
	viper.SetDefault("embedding.timeout", 30*time.Second)
	viper.SetDefault("retrieval.mode", "keyword")
	viper.SetDefault("retrieval.candidate_limit", 20)
	viper.SetDefault("retrieval.final_limit", 8)
	viper.SetDefault("retrieval.min_vector_score", 0.3)
	viper.SetDefault("retrieval.chunk_runes", 600)
	viper.SetDefault("retrieval.chunk_overlap_runes", 80)
	viper.SetDefault("retrieval.max_extracted_runes", 500000)
	viper.SetDefault("retrieval.worker_interval", 5*time.Second)

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
	applyBoolEnv("CSA_AGENT_ENABLED", &cfg.Agent.Enabled)
	applyBoolEnv("CSA_AGENT_WRITE_ENABLED", &cfg.Agent.WriteEnabled)
	applyIntEnv("CSA_AGENT_PORT", &cfg.Agent.Port)
	applyStringEnv("CSA_AGENT_BASE_URL", &cfg.Agent.BaseURL)
	applyStringEnv("CSA_SERVER_INTERNAL_URL", &cfg.Agent.ServerURL)
	applyStringEnv("CSA_AGENT_INTERNAL_SECRET", &cfg.Agent.InternalSecret)
	applyStringEnv("CSA_LLM_PROVIDER", &cfg.Agent.LLMProvider)
	applyStringEnv("CSA_LLM_MODEL", &cfg.Agent.LLMModel)
	applyStringEnv("CSA_LLM_API_KEY", &cfg.Agent.LLMAPIKey)
	applyStringEnv("CSA_LLM_BASE_URL", &cfg.Agent.LLMBaseURL)
	applyDurationEnv("CSA_LLM_TIMEOUT", &cfg.Agent.LLMTimeout)
	applyIntEnv("CSA_AGENT_MAX_ITERATIONS", &cfg.Agent.MaxIterations)
	applyIntEnv("CSA_AGENT_MAX_TOOL_CALLS", &cfg.Agent.MaxToolCalls)
	applyIntEnv("CSA_AGENT_WORKER_CONCURRENCY", &cfg.Agent.WorkerConcurrency)
	applyBoolEnv("CSA_EMBEDDING_ENABLED", &cfg.Embedding.Enabled)
	applyStringEnv("CSA_EMBEDDING_PROVIDER", &cfg.Embedding.Provider)
	applyStringEnv("CSA_EMBEDDING_MODEL", &cfg.Embedding.Model)
	applyStringEnv("CSA_EMBEDDING_API_KEY", &cfg.Embedding.APIKey)
	applyStringEnv("CSA_EMBEDDING_BASE_URL", &cfg.Embedding.BaseURL)
	applyIntEnv("CSA_EMBEDDING_DIMENSIONS", &cfg.Embedding.Dimensions)
	applyIntEnv("CSA_EMBEDDING_BATCH_SIZE", &cfg.Embedding.BatchSize)
	applyDurationEnv("CSA_EMBEDDING_TIMEOUT", &cfg.Embedding.Timeout)
	applyStringEnv("CSA_RETRIEVAL_MODE", &cfg.Retrieval.Mode)
	applyIntEnv("CSA_RETRIEVAL_CANDIDATE_LIMIT", &cfg.Retrieval.CandidateLimit)
	applyIntEnv("CSA_RETRIEVAL_FINAL_LIMIT", &cfg.Retrieval.FinalLimit)
	applyFloat64Env("CSA_RETRIEVAL_MIN_VECTOR_SCORE", &cfg.Retrieval.MinVectorScore)
	applyIntEnv("CSA_RETRIEVAL_CHUNK_RUNES", &cfg.Retrieval.ChunkRunes)
	applyIntEnv("CSA_RETRIEVAL_CHUNK_OVERLAP_RUNES", &cfg.Retrieval.ChunkOverlapRunes)
	applyIntEnv("CSA_RETRIEVAL_MAX_EXTRACTED_RUNES", &cfg.Retrieval.MaxExtractedRunes)
	applyDurationEnv("CSA_RETRIEVAL_WORKER_INTERVAL", &cfg.Retrieval.WorkerInterval)
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

func applyFloat64Env(key string, target *float64) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			*target = parsed
		}
	}
}

func applyBoolEnv(key string, target *bool) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
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
