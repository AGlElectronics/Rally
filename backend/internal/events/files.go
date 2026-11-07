package events

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/files"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

const (
	uploadURLExpiry   = 15 * time.Minute
	downloadURLExpiry = 5 * time.Minute
	// Uploads not attached to a submission by then are deleted.
	orphanUploadAge = 24 * time.Hour
	maxFilenameLen  = 200
)

// FileService hands out presigned upload/download URLs. Files go straight
// between the browser and object storage.
type FileService struct {
	pool  *pgxpool.Pool
	store *files.Client // nil when object storage is disabled
	now   func() time.Time
}

// NewFileService returns a service; store may be nil (uploads then answer 503).
func NewFileService(pool *pgxpool.Pool, store *files.Client) *FileService {
	return &FileService{pool: pool, store: store, now: time.Now}
}

func (s *FileService) requireStore() error {
	if s.store == nil {
		return httperror.New(httperror.CodeServiceUnavailable, "file uploads are not available (object storage is disabled)")
	}
	return nil
}

// UploadInput asks for an upload slot for one file of a form field.
type UploadInput struct {
	CfPID       uuid.UUID `json:"cfp_id"`
	FieldID     string    `json:"field_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
}

// UploadTicket is what the browser needs to POST the file to storage.
type UploadTicket struct {
	FileID    uuid.UUID         `json:"file_id"`
	URL       string            `json:"url"`
	Fields    map[string]string `json:"fields"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// StartUpload checks the file against the field's rules and returns a
// presigned POST. The file row stays pending until CompleteUpload.
func (s *FileService) StartUpload(ctx context.Context, in UploadInput) (*UploadTicket, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	cfp, err := loadCfP(ctx, s.pool, in.CfPID, false)
	if err != nil {
		return nil, err
	}
	if cfp.Status != CfPOpen && !isStaffForEdition(u, cfp.OrganizationID, cfp.EditionID) {
		return nil, httperror.NotFound("call for papers")
	}
	if !cfp.AcceptsSubmissions(s.now()) {
		return nil, httperror.Conflict("call for papers is not accepting submissions")
	}
	fd, ok := cfp.Form.Field(in.FieldID)
	if !ok || fd.Type != FieldFile {
		return nil, httperror.Validation("invalid upload", map[string]string{"field_id": "is not a file field of this form"})
	}
	name := cleanFilename(in.Filename)
	ct := strings.ToLower(strings.TrimSpace(in.ContentType))
	issues := map[string]string{}
	if name == "" {
		issues["filename"] = "is required"
	}
	if ct == "" || strings.ContainsAny(ct, " ;,") {
		issues["content_type"] = "is required"
	} else if !fd.AcceptsType(ct) {
		issues["content_type"] = "this file type isn't allowed here (" + strings.Join(fd.Accept, ", ") + ")"
	}
	if in.SizeBytes <= 0 {
		issues["size_bytes"] = "file is empty"
	} else if in.SizeBytes > fd.MaxBytes() {
		issues["size_bytes"] = fmt.Sprintf("file is larger than %g MB", float64(fd.MaxBytes())/1024/1024)
	}
	if err := validationIfAny("invalid upload", issues); err != nil {
		return nil, err
	}

	id := uuid.New()
	key := path.Join("submissions", cfp.ID.String(), id.String(), name)
	if _, err := s.pool.Exec(ctx, `INSERT INTO submission_files
		(id, organization_id, cfp_id, uploader_user_id, field_id, object_key, filename, content_type, size_bytes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, cfp.OrganizationID, cfp.ID, u.ID, fd.ID, key, name, ct, in.SizeBytes); err != nil {
		return nil, httperror.Internal(err)
	}
	url, fields, err := s.store.PresignedPost(ctx, key, ct, fd.MaxBytes(), uploadURLExpiry)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	return &UploadTicket{FileID: id, URL: url, Fields: fields, ExpiresAt: s.now().Add(uploadURLExpiry)}, nil
}

// CompleteUpload confirms the object arrived and matches what was announced.
func (s *FileService) CompleteUpload(ctx context.Context, id uuid.UUID) (*FileInfo, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	var f FileInfo
	var key string
	var uploader uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT id, field_id, filename, content_type, size_bytes, object_key, uploader_user_id
		FROM submission_files WHERE id=$1`, id).
		Scan(&f.ID, &f.FieldID, &f.Filename, &f.ContentType, &f.SizeBytes, &key, &uploader)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && uploader != u.ID) {
		return nil, httperror.NotFound("file")
	}
	if err != nil {
		return nil, httperror.Internal(err)
	}
	info, err := s.store.Stat(ctx, key)
	if err != nil {
		return nil, httperror.Conflict("the file hasn't arrived in storage yet; upload it first")
	}
	// The policy capped the size; record the real one.
	if _, err := s.pool.Exec(ctx, `UPDATE submission_files SET status='uploaded', size_bytes=$2 WHERE id=$1`,
		id, info.Size); err != nil {
		return nil, httperror.Internal(err)
	}
	f.SizeBytes = info.Size
	return &f, nil
}

// DownloadURL returns a short-lived link to a file the caller may see: their
// own upload, or a file on a submission visible to them.
func (s *FileService) DownloadURL(ctx context.Context, id uuid.UUID) (string, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return "", httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	if err := s.requireStore(); err != nil {
		return "", err
	}
	var key, name string
	var uploader uuid.UUID
	var subID *uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT object_key, filename, uploader_user_id, submission_id
		FROM submission_files WHERE id=$1 AND status='uploaded'`, id).Scan(&key, &name, &uploader, &subID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", httperror.NotFound("file")
	}
	if err != nil {
		return "", httperror.Internal(err)
	}
	allowed := uploader == u.ID
	if !allowed && subID != nil {
		sub, err := loadSubmission(ctx, s.pool, *subID, false)
		if err != nil {
			return "", err
		}
		allowed = canView(u, sub)
	}
	if !allowed {
		return "", httperror.NotFound("file")
	}
	url, err := s.store.PresignedDownload(ctx, key, name, downloadURLExpiry)
	if err != nil {
		return "", httperror.Internal(err)
	}
	return url, nil
}

// attachFiles links uploaded files to a new submission.
func attachFiles(ctx context.Context, tx pgx.Tx, subID, cfpID, userID uuid.UUID, byField map[string][]uuid.UUID) error {
	for fieldID, ids := range byField {
		if len(ids) == 0 {
			continue
		}
		tag, err := tx.Exec(ctx, `UPDATE submission_files SET submission_id=$1
			WHERE id = ANY($2) AND uploader_user_id=$3 AND cfp_id=$4 AND field_id=$5
			  AND status='uploaded' AND (submission_id IS NULL OR submission_id=$1)`,
			subID, uniqueIDs(ids), userID, cfpID, fieldID)
		if err != nil {
			return httperror.Internal(err)
		}
		if int(tag.RowsAffected()) != len(uniqueIDs(ids)) {
			return httperror.Validation("invalid submission", map[string]string{
				"answers." + fieldID: "a file is missing or still uploading; upload it again",
			})
		}
	}
	return nil
}

// syncFiles makes the attached files of each active file field match the
// answers: newly referenced files are attached, dropped ones detached (the
// sweeper deletes them later).
func syncFiles(ctx context.Context, tx pgx.Tx, f Form, subID, cfpID, userID uuid.UUID, byField map[string][]uuid.UUID) error {
	for _, fd := range f.Fields {
		if fd.Archived || fd.Type != FieldFile {
			continue
		}
		keep := uniqueIDs(byField[fd.ID])
		if _, err := tx.Exec(ctx, `UPDATE submission_files SET submission_id=NULL
			WHERE submission_id=$1 AND field_id=$2 AND NOT (id = ANY($3))`, subID, fd.ID, keep); err != nil {
			return httperror.Internal(err)
		}
	}
	return attachFiles(ctx, tx, subID, cfpID, userID, byField)
}

// Run deletes orphaned uploads periodically until ctx ends.
func (s *FileService) Run(ctx context.Context, log zerolog.Logger) {
	if s.store == nil {
		return
	}
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		if n, err := s.sweep(ctx); err != nil {
			log.Warn().Err(err).Msg("upload sweep failed")
		} else if n > 0 {
			log.Info().Int("files", n).Msg("removed unattached uploads")
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *FileService) sweep(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, object_key FROM submission_files
		WHERE submission_id IS NULL AND created_at < $1 LIMIT 500`, s.now().Add(-orphanUploadAge))
	if err != nil {
		return 0, err
	}
	type orphan struct {
		id  uuid.UUID
		key string
	}
	var list []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.key); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, o)
	}
	rows.Close()
	n := 0
	for _, o := range list {
		if err := s.store.Remove(ctx, o.key); err != nil {
			continue
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM submission_files WHERE id=$1 AND submission_id IS NULL`, o.id); err == nil {
			n++
		}
	}
	return n, nil
}

// cleanFilename keeps a safe, readable base name.
func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "." || name == "/" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), strings.ContainsRune("._- ()", r):
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.TrimSpace(b.String())
	if r := []rune(out); len(r) > maxFilenameLen {
		out = string(r[len(r)-maxFilenameLen:])
	}
	return out
}
