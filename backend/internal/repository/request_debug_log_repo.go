package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type requestDebugLogRepository struct {
	db *sql.DB
}

func NewRequestDebugLogRepository(db *sql.DB) service.RequestDebugLogRepository {
	return &requestDebugLogRepository{db: db}
}

func (r *requestDebugLogRepository) BatchInsert(ctx context.Context, logs []*service.RequestDebugLog) error {
	if len(logs) == 0 {
		return nil
	}

	const cols = 17
	placeholders := make([]string, 0, len(logs))
	args := make([]any, 0, len(logs)*cols)

	for i, l := range logs {
		base := i * cols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8,
			base+9, base+10, base+11, base+12, base+13, base+14, base+15, base+16,
			base+17,
		))

		var reqHeaders, reqBody, respHeaders, respBody, truncInfo *string
		if l.RequestHeaders != nil {
			s := string(l.RequestHeaders)
			reqHeaders = &s
		}
		if l.RequestBody != nil {
			s := string(l.RequestBody)
			reqBody = &s
		}
		if l.ResponseHeaders != nil {
			s := string(l.ResponseHeaders)
			respHeaders = &s
		}
		if l.ResponseBody != nil {
			s := string(l.ResponseBody)
			respBody = &s
		}
		if l.TruncationInfo != nil {
			s := string(l.TruncationInfo)
			truncInfo = &s
		}
		var reqText, respText *string
		if l.RequestText != "" {
			reqText = &l.RequestText
		}
		if l.ResponseText != "" {
			respText = &l.ResponseText
		}

		args = append(args,
			l.RequestID,
			l.UserID,
			l.APIKeyID,
			l.GroupID,
			l.Model,
			l.Stream,
			reqHeaders,
			reqBody,
			reqText,
			respHeaders,
			respBody,
			respText,
			l.Truncated,
			truncInfo,
			l.BodyBytes,
			l.CreatedAt,
			l.ExpiresAt,
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO request_debug_logs
			(request_id, user_id, api_key_id, group_id, model, stream,
			 request_headers, request_body, request_text,
			 response_headers, response_body, response_text,
			 truncated, truncation_info, body_bytes, created_at, expires_at)
		VALUES %s
	`, strings.Join(placeholders, ","))

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *requestDebugLogRepository) GetByRequestID(ctx context.Context, requestID string) (*service.RequestDebugLog, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, request_id, user_id, api_key_id, group_id, model, stream,
		       request_headers, request_body, request_text,
		       response_headers, response_body, response_text,
		       truncated, truncation_info, body_bytes, created_at, expires_at
		FROM request_debug_logs
		WHERE request_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, requestID)

	l := &service.RequestDebugLog{}
	var userID, apiKeyID, groupID sql.NullInt64
	var reqHeaders, reqBody, reqText, respHeaders, respBody, respText, truncInfo sql.NullString

	err := row.Scan(
		&l.ID,
		&l.RequestID,
		&userID,
		&apiKeyID,
		&groupID,
		&l.Model,
		&l.Stream,
		&reqHeaders,
		&reqBody,
		&reqText,
		&respHeaders,
		&respBody,
		&respText,
		&l.Truncated,
		&truncInfo,
		&l.BodyBytes,
		&l.CreatedAt,
		&l.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if userID.Valid {
		l.UserID = &userID.Int64
	}
	if apiKeyID.Valid {
		l.APIKeyID = &apiKeyID.Int64
	}
	if groupID.Valid {
		l.GroupID = &groupID.Int64
	}
	if reqHeaders.Valid {
		l.RequestHeaders = json.RawMessage(reqHeaders.String)
	}
	if reqBody.Valid {
		l.RequestBody = json.RawMessage(reqBody.String)
	}
	if reqText.Valid {
		l.RequestText = reqText.String
	}
	if respHeaders.Valid {
		l.ResponseHeaders = json.RawMessage(respHeaders.String)
	}
	if respBody.Valid {
		l.ResponseBody = json.RawMessage(respBody.String)
	}
	if respText.Valid {
		l.ResponseText = respText.String
	}
	if truncInfo.Valid {
		l.TruncationInfo = json.RawMessage(truncInfo.String)
	}

	return l, nil
}

func (r *requestDebugLogRepository) DeleteExpired(ctx context.Context, now time.Time, batchSize int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM request_debug_logs
		WHERE id IN (
			SELECT id FROM request_debug_logs
			WHERE expires_at < $1
			LIMIT $2
		)
	`, now, batchSize)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
