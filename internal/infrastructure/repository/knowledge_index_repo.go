package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"SWPUCAT/internal/application/knowledgeindex"
	"SWPUCAT/internal/retrieval"

	"gorm.io/gorm"
)

type KnowledgeIndexRepo struct{ db *gorm.DB }

func NewKnowledgeIndexRepo(db *gorm.DB) *KnowledgeIndexRepo {
	return &KnowledgeIndexRepo{db: db}
}

func (r *KnowledgeIndexRepo) ListApprovedSources(ctx context.Context) ([]knowledgeindex.Source, error) {
	var result []knowledgeindex.Source
	err := r.db.WithContext(ctx).Raw(`
		SELECT ki.id, ki.type, ki.name, COALESCE(ki.description, '') AS description,
		       COALESCE(ki.url, '') AS url, COALESCE(ki.file_key, '') AS file_key,
		       COALESCE(kc.name, '') AS category_name
		FROM knowledge_items ki
		LEFT JOIN knowledge_categories kc ON kc.id = ki.category_id
		WHERE ki.status = 'approved' AND ki.deleted_at IS NULL
		ORDER BY ki.id ASC`).Scan(&result).Error
	return result, err
}

func (r *KnowledgeIndexRepo) EnsureJob(ctx context.Context, itemID int64, fingerprint string) error {
	if itemID <= 0 || strings.TrimSpace(fingerprint) == "" {
		return errors.New("knowledge index item and fingerprint are required")
	}
	return ensureKnowledgeIndexJob(r.db.WithContext(ctx), itemID, fingerprint)
}

func ensureKnowledgeIndexJob(db *gorm.DB, itemID int64, fingerprint string) error {
	return db.Exec(`
		INSERT INTO knowledge_index_jobs (
			knowledge_item_id, status, source_fingerprint, attempts,
			available_at, locked_at, last_error, created_at, updated_at
		) VALUES (?, 'queued', ?, 0, CURRENT_TIMESTAMP, NULL, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (knowledge_item_id) DO UPDATE SET
			source_fingerprint = EXCLUDED.source_fingerprint,
			attempts = CASE
				WHEN knowledge_index_jobs.source_fingerprint IS DISTINCT FROM EXCLUDED.source_fingerprint
				  OR (knowledge_index_jobs.status = 'failed' AND knowledge_index_jobs.available_at <= CURRENT_TIMESTAMP) THEN 0
				ELSE knowledge_index_jobs.attempts
			END,
			status = CASE
				WHEN knowledge_index_jobs.source_fingerprint IS DISTINCT FROM EXCLUDED.source_fingerprint
				  OR (knowledge_index_jobs.status = 'failed' AND knowledge_index_jobs.available_at <= CURRENT_TIMESTAMP) THEN 'queued'
				ELSE knowledge_index_jobs.status
			END,
			available_at = CASE
				WHEN knowledge_index_jobs.source_fingerprint IS DISTINCT FROM EXCLUDED.source_fingerprint
				  OR (knowledge_index_jobs.status = 'failed' AND knowledge_index_jobs.available_at <= CURRENT_TIMESTAMP) THEN CURRENT_TIMESTAMP
				ELSE knowledge_index_jobs.available_at
			END,
			locked_at = CASE
				WHEN knowledge_index_jobs.source_fingerprint IS DISTINCT FROM EXCLUDED.source_fingerprint
				  OR (knowledge_index_jobs.status = 'failed' AND knowledge_index_jobs.available_at <= CURRENT_TIMESTAMP) THEN NULL
				ELSE knowledge_index_jobs.locked_at
			END,
			last_error = CASE
				WHEN knowledge_index_jobs.source_fingerprint IS DISTINCT FROM EXCLUDED.source_fingerprint
				  OR (knowledge_index_jobs.status = 'failed' AND knowledge_index_jobs.available_at <= CURRENT_TIMESTAMP) THEN ''
				ELSE knowledge_index_jobs.last_error
			END,
			updated_at = CURRENT_TIMESTAMP
		WHERE knowledge_index_jobs.source_fingerprint IS DISTINCT FROM EXCLUDED.source_fingerprint
		   OR (knowledge_index_jobs.status = 'failed' AND knowledge_index_jobs.available_at <= CURRENT_TIMESTAMP)`, itemID, fingerprint).Error
}

type claimedKnowledgeJob struct {
	KnowledgeItemID   int64  `gorm:"column:knowledge_item_id"`
	SourceFingerprint string `gorm:"column:source_fingerprint"`
	Attempts          int    `gorm:"column:attempts"`
	Type              string `gorm:"column:type"`
	Name              string `gorm:"column:name"`
	Description       string `gorm:"column:description"`
	URL               string `gorm:"column:url"`
	FileKey           string `gorm:"column:file_key"`
	CategoryName      string `gorm:"column:category_name"`
}

func (r *KnowledgeIndexRepo) ClaimJob(ctx context.Context, now, staleBefore time.Time, maxAttempts int) (*knowledgeindex.Job, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var row claimedKnowledgeJob
	err := r.db.WithContext(ctx).Raw(`
		WITH candidate AS (
			SELECT j.knowledge_item_id
			FROM knowledge_index_jobs j
			JOIN knowledge_items ki ON ki.id = j.knowledge_item_id
			WHERE j.attempts < ?
			  AND ki.status = 'approved'
			  AND ki.deleted_at IS NULL
			  AND (
				(j.status = 'queued' AND j.available_at <= ?)
				OR (j.status = 'running' AND j.locked_at < ?)
			  )
			ORDER BY j.available_at ASC, j.knowledge_item_id ASC
			FOR UPDATE OF j SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE knowledge_index_jobs j
			SET status = 'running', attempts = j.attempts + 1,
			    locked_at = ?, updated_at = ?
			FROM candidate c
			WHERE j.knowledge_item_id = c.knowledge_item_id
			RETURNING j.knowledge_item_id, j.source_fingerprint, j.attempts
		)
		SELECT claimed.knowledge_item_id, claimed.source_fingerprint, claimed.attempts,
		       ki.type, ki.name, COALESCE(ki.description, '') AS description,
		       COALESCE(ki.url, '') AS url, COALESCE(ki.file_key, '') AS file_key,
		       COALESCE(kc.name, '') AS category_name
		FROM claimed
		JOIN knowledge_items ki ON ki.id = claimed.knowledge_item_id
		LEFT JOIN knowledge_categories kc ON kc.id = ki.category_id`,
		maxAttempts, now, staleBefore, now, now).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.KnowledgeItemID == 0 {
		return nil, knowledgeindex.ErrNoPendingJob
	}
	return &knowledgeindex.Job{
		Source: knowledgeindex.Source{
			ID: row.KnowledgeItemID, Type: row.Type, Name: row.Name,
			Description: row.Description, URL: row.URL, FileKey: row.FileKey,
			CategoryName: row.CategoryName,
		},
		ClaimedFingerprint: row.SourceFingerprint,
		Attempts:           row.Attempts,
	}, nil
}

func (r *KnowledgeIndexRepo) CompleteJob(ctx context.Context, job *knowledgeindex.Job, finalFingerprint string, chunks []knowledgeindex.Chunk) error {
	if job == nil || job.Source.ID <= 0 || strings.TrimSpace(finalFingerprint) == "" || len(chunks) == 0 {
		return errors.New("invalid completed knowledge index job")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state struct {
			Status      string `gorm:"column:status"`
			Fingerprint string `gorm:"column:source_fingerprint"`
		}
		if err := tx.Raw(`SELECT status, source_fingerprint FROM knowledge_index_jobs WHERE knowledge_item_id = ? FOR UPDATE`, job.Source.ID).Scan(&state).Error; err != nil {
			return err
		}
		if state.Status != "running" || state.Fingerprint != job.ClaimedFingerprint {
			return knowledgeindex.ErrStaleJob
		}
		var visible int64
		if err := tx.Raw(`SELECT COUNT(*) FROM knowledge_items WHERE id = ? AND status = 'approved' AND deleted_at IS NULL`, job.Source.ID).Scan(&visible).Error; err != nil {
			return err
		}
		if visible != 1 {
			return knowledgeindex.ErrStaleJob
		}
		if err := tx.Exec(`DELETE FROM knowledge_chunks WHERE knowledge_item_id = ?`, job.Source.ID).Error; err != nil {
			return err
		}
		for _, chunk := range chunks {
			if err := insertKnowledgeChunk(tx, job.Source.ID, chunk); err != nil {
				return err
			}
		}
		result := tx.Exec(`
			UPDATE knowledge_index_jobs
			SET status = 'completed', source_fingerprint = ?, locked_at = NULL,
			    last_error = '', updated_at = CURRENT_TIMESTAMP
			WHERE knowledge_item_id = ? AND status = 'running' AND source_fingerprint = ?`,
			finalFingerprint, job.Source.ID, job.ClaimedFingerprint)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return knowledgeindex.ErrStaleJob
		}
		return nil
	})
}

// CompletePartialJob replaces the searchable chunks while retaining a queued
// or failed job state. This lets descriptions remain searchable when parsing
// or embedding a file fails, and gives the worker a bounded retry schedule.
func (r *KnowledgeIndexRepo) CompletePartialJob(ctx context.Context, job *knowledgeindex.Job, sourceFingerprint string, chunks []knowledgeindex.Chunk, message string, retryAt time.Time, maxAttempts int) error {
	if job == nil || job.Source.ID <= 0 || strings.TrimSpace(sourceFingerprint) == "" || len(chunks) == 0 {
		return errors.New("invalid partial knowledge index job")
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if retryAt.IsZero() {
		retryAt = time.Now().Add(time.Hour)
	}
	message = truncateDatabaseText(message, 1000)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state struct {
			Status      string `gorm:"column:status"`
			Fingerprint string `gorm:"column:source_fingerprint"`
			Attempts    int    `gorm:"column:attempts"`
		}
		if err := tx.Raw(`SELECT status, source_fingerprint, attempts FROM knowledge_index_jobs WHERE knowledge_item_id = ? FOR UPDATE`, job.Source.ID).Scan(&state).Error; err != nil {
			return err
		}
		if state.Status != "running" || state.Fingerprint != job.ClaimedFingerprint {
			return knowledgeindex.ErrStaleJob
		}
		var visible int64
		if err := tx.Raw(`SELECT COUNT(*) FROM knowledge_items WHERE id = ? AND status = 'approved' AND deleted_at IS NULL`, job.Source.ID).Scan(&visible).Error; err != nil {
			return err
		}
		if visible != 1 {
			return knowledgeindex.ErrStaleJob
		}
		if err := tx.Exec(`DELETE FROM knowledge_chunks WHERE knowledge_item_id = ?`, job.Source.ID).Error; err != nil {
			return err
		}
		for _, chunk := range chunks {
			if err := insertKnowledgeChunk(tx, job.Source.ID, chunk); err != nil {
				return err
			}
		}
		status := "queued"
		if state.Attempts >= maxAttempts {
			status = "failed"
		}
		result := tx.Exec(`
			UPDATE knowledge_index_jobs
			SET status = ?, source_fingerprint = ?, available_at = ?, locked_at = NULL,
			    last_error = ?, updated_at = CURRENT_TIMESTAMP
			WHERE knowledge_item_id = ? AND status = 'running' AND source_fingerprint = ?`,
			status, sourceFingerprint, retryAt, message, job.Source.ID, job.ClaimedFingerprint)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return knowledgeindex.ErrStaleJob
		}
		return nil
	})
}

func insertKnowledgeChunk(tx *gorm.DB, itemID int64, chunk knowledgeindex.Chunk) error {
	if strings.TrimSpace(chunk.Content) == "" || chunk.Index < 0 || strings.TrimSpace(chunk.SourceKind) == "" || strings.TrimSpace(chunk.ContentHash) == "" {
		return errors.New("invalid knowledge chunk")
	}
	metadata := strings.TrimSpace(chunk.MetadataJSON)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return errors.New("invalid knowledge chunk metadata")
	}
	if len(chunk.Embedding) == 0 {
		return tx.Exec(`
			INSERT INTO knowledge_chunks (
				knowledge_item_id, source_kind, chunk_index, content, content_hash,
				embedding, embedding_model, embedding_dimensions, metadata_json,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, NULL, '', 0, CAST(? AS jsonb), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			itemID, chunk.SourceKind, chunk.Index, chunk.Content, chunk.ContentHash, metadata).Error
	}
	if err := retrieval.ValidateVector(chunk.Embedding, chunk.EmbeddingDimensions); err != nil {
		return err
	}
	return tx.Exec(`
		INSERT INTO knowledge_chunks (
			knowledge_item_id, source_kind, chunk_index, content, content_hash,
			embedding, embedding_model, embedding_dimensions, metadata_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, CAST(? AS vector), ?, ?, CAST(? AS jsonb), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		itemID, chunk.SourceKind, chunk.Index, chunk.Content, chunk.ContentHash,
		vectorLiteral(chunk.Embedding), chunk.EmbeddingModel, chunk.EmbeddingDimensions, metadata).Error
}

func (r *KnowledgeIndexRepo) FailJob(ctx context.Context, job *knowledgeindex.Job, message string, retryAt time.Time, maxAttempts int) error {
	if job == nil || job.Source.ID <= 0 {
		return errors.New("invalid failed knowledge index job")
	}
	message = truncateDatabaseText(message, 1000)
	result := r.db.WithContext(ctx).Exec(`
		UPDATE knowledge_index_jobs
		SET status = CASE WHEN attempts >= ? THEN 'failed' ELSE 'queued' END,
		    available_at = ?, locked_at = NULL, last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE knowledge_item_id = ? AND status = 'running' AND source_fingerprint = ?`,
		maxAttempts, retryAt, message, job.Source.ID, job.ClaimedFingerprint)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return knowledgeindex.ErrStaleJob
	}
	return nil
}

func (r *KnowledgeIndexRepo) KeywordSearch(ctx context.Context, query string, limit int) ([]knowledgeindex.Candidate, error) {
	if limit < 1 {
		return []knowledgeindex.Candidate{}, nil
	}
	pattern := "%" + escapeLike(query) + "%"
	var result []knowledgeindex.Candidate
	err := r.db.WithContext(ctx).Raw(`
		WITH params AS (
			SELECT LOWER(?)::text AS query, ?::text AS pattern
		), scored AS (
			SELECT kc.id AS chunk_id, ki.id AS item_id, ki.type, ki.name,
			       COALESCE(ki.description, '') AS description,
			       COALESCE(ki.url, '') AS url,
			       COALESCE(cat.name, '') AS category_name,
			       kc.source_kind, kc.content AS excerpt,
			       GREATEST(
				   similarity(LOWER(COALESCE(ki.name, '')), p.query),
				   word_similarity(p.query, LOWER(COALESCE(ki.name, ''))),
				   word_similarity(p.query, LOWER(COALESCE(ki.description, ''))),
				   word_similarity(p.query, LOWER(COALESCE(cat.name, ''))),
				   word_similarity(p.query, LOWER(kc.content)),
				   CASE WHEN ki.name ILIKE p.pattern ESCAPE E'\\' THEN 1.0 ELSE 0.0 END,
				   CASE WHEN COALESCE(ki.description, '') ILIKE p.pattern ESCAPE E'\\' THEN 1.0 ELSE 0.0 END,
				   CASE WHEN kc.content ILIKE p.pattern ESCAPE E'\\' THEN 1.0 ELSE 0.0 END
			       ) AS score
			FROM knowledge_chunks kc
			JOIN knowledge_items ki ON ki.id = kc.knowledge_item_id
			LEFT JOIN knowledge_categories cat ON cat.id = ki.category_id
			CROSS JOIN params p
			WHERE ki.status = 'approved' AND ki.deleted_at IS NULL
		), ranked AS (
			SELECT scored.*, ROW_NUMBER() OVER (
				PARTITION BY item_id ORDER BY score DESC, chunk_id ASC
			) AS item_rank
			FROM scored
		)
		SELECT chunk_id, item_id, type, name, description, url, category_name,
		       source_kind, excerpt, score
		FROM ranked
		WHERE item_rank = 1 AND score >= 0.04
		ORDER BY score DESC, item_id ASC
		LIMIT ?`, strings.TrimSpace(query), pattern, limit).Scan(&result).Error
	return result, err
}

func (r *KnowledgeIndexRepo) VectorSearch(ctx context.Context, vector []float32, model string, dimensions, limit int, minScore float64) ([]knowledgeindex.Candidate, error) {
	if limit < 1 {
		return []knowledgeindex.Candidate{}, nil
	}
	if err := retrieval.ValidateVector(vector, dimensions); err != nil {
		return nil, err
	}
	var result []knowledgeindex.Candidate
	err := r.db.WithContext(ctx).Raw(`
		WITH query_vector AS (
			SELECT CAST(? AS vector) AS embedding
		), scored AS (
			SELECT kc.id AS chunk_id, ki.id AS item_id, ki.type, ki.name,
			       COALESCE(ki.description, '') AS description,
			       COALESCE(ki.url, '') AS url,
			       COALESCE(cat.name, '') AS category_name,
			       kc.source_kind, kc.content AS excerpt,
			       1 - (kc.embedding <=> q.embedding) AS score
			FROM knowledge_chunks kc
			JOIN knowledge_items ki ON ki.id = kc.knowledge_item_id
			LEFT JOIN knowledge_categories cat ON cat.id = ki.category_id
			CROSS JOIN query_vector q
			WHERE ki.status = 'approved' AND ki.deleted_at IS NULL
			  AND kc.embedding IS NOT NULL
			  AND kc.embedding_model = ?
			  AND kc.embedding_dimensions = ?
		), ranked AS (
			SELECT scored.*, ROW_NUMBER() OVER (
				PARTITION BY item_id ORDER BY score DESC, chunk_id ASC
			) AS item_rank
			FROM scored
			WHERE score >= ?
		)
		SELECT chunk_id, item_id, type, name, description, url, category_name,
		       source_kind, excerpt, score
		FROM ranked
		WHERE item_rank = 1
		ORDER BY score DESC, item_id ASC
		LIMIT ?`, vectorLiteral(vector), strings.TrimSpace(model), dimensions, minScore, limit).Scan(&result).Error
	return result, err
}

func vectorLiteral(vector []float32) string {
	var b strings.Builder
	b.Grow(len(vector) * 12)
	b.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(strings.TrimSpace(value))
}

func truncateDatabaseText(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

// queueKnowledgeIndexJob is used inside knowledge-item write transactions.
// An empty fingerprint deliberately forces the next reconciliation to replace
// it with the current model/config fingerprint before or during processing.
func queueKnowledgeIndexJob(tx *gorm.DB, itemID int64) error {
	return tx.Exec(`
		INSERT INTO knowledge_index_jobs (
			knowledge_item_id, status, source_fingerprint, attempts,
			available_at, locked_at, last_error, created_at, updated_at
		) VALUES (?, 'queued', '', 0, CURRENT_TIMESTAMP, NULL, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (knowledge_item_id) DO UPDATE SET
			status = 'queued', source_fingerprint = '', attempts = 0,
			available_at = CURRENT_TIMESTAMP, locked_at = NULL, last_error = '',
			updated_at = CURRENT_TIMESTAMP`, itemID).Error
}
