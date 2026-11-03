package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/brand"
	"github.com/AGlElectronics/rally/backend/internal/config"
	rallyhttp "github.com/AGlElectronics/rally/backend/internal/http"
)

// Happy path: open CfP → draft submission → submit → accept (different user).
// Requires RALLY_TEST_DATABASE_URL with migrations applied (incl. CfP).
func TestCfPSubmissionAcceptFlow(t *testing.T) {
	dsn := os.Getenv("RALLY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("RALLY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	cfg := &config.Config{
		AppEnv:             "development",
		AppBaseURL:         "http://localhost:8100",
		HTTPAddr:           ":0",
		AuthMode:           config.AuthModeDev,
		DevDefaultUser:     "dev-admin",
		CORSAllowedOrigins: []string{"*"},
	}
	access := auth.DefaultAccessConfig()
	router := rallyhttp.NewRouter(rallyhttp.Deps{
		Config: cfg,
		Pool:   pool,
		Logger: zerolog.Nop(),
		Brand:  brand.New(cfg),
		Access: access,
	}, "test")

	do := func(user, method, path string, body any) *httptest.ResponseRecorder {
		var buf *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			buf = bytes.NewReader(b)
		} else {
			buf = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, buf)
		req.Header.Set("X-Dev-User", user)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	editionID := "00000000-0000-0000-0000-000000000030"

	cfpRec := do("dev-admin", http.MethodPost, "/api/v1/editions/"+editionID+"/cfps", map[string]any{
		"title":       "Demo CfP",
		"description": "Talks welcome",
		"status":      "open",
	})
	if cfpRec.Code != http.StatusCreated {
		t.Fatalf("create cfp %d %s", cfpRec.Code, cfpRec.Body.String())
	}
	var cfp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(cfpRec.Body.Bytes(), &cfp)
	if cfp.ID == "" || cfp.Status != "open" {
		t.Fatalf("bad cfp: %s", cfpRec.Body.String())
	}

	subRec := do("alice", http.MethodPost, "/api/v1/cfps/"+cfp.ID+"/submissions", map[string]any{
		"title":    "Why conventions need inventory",
		"abstract": "A practical talk about warehouse ops at events.",
		"speakers": []map[string]string{{"name": "Alice", "email": "alice@example.test"}},
	})
	if subRec.Code != http.StatusCreated {
		t.Fatalf("create submission %d %s", subRec.Code, subRec.Body.String())
	}
	var sub struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	_ = json.Unmarshal(subRec.Body.Bytes(), &sub)
	if sub.State != "draft" {
		t.Fatalf("want draft, got %s", subRec.Body.String())
	}

	submitRec := do("alice", http.MethodPost, "/api/v1/submissions/"+sub.ID+"/submit", nil)
	if submitRec.Code != http.StatusOK {
		t.Fatalf("submit %d %s", submitRec.Code, submitRec.Body.String())
	}
	_ = json.Unmarshal(submitRec.Body.Bytes(), &sub)
	if sub.State != "submitted" {
		t.Fatalf("want submitted, got %s", submitRec.Body.String())
	}

	acceptRec := do("dev-admin", http.MethodPost, "/api/v1/submissions/"+sub.ID+"/accept", map[string]any{
		"note": "Great fit",
	})
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept %d %s", acceptRec.Code, acceptRec.Body.String())
	}
	_ = json.Unmarshal(acceptRec.Body.Bytes(), &sub)
	if sub.State != "accepted" {
		t.Fatalf("want accepted, got %s", acceptRec.Body.String())
	}
}
