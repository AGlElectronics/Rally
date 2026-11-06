package events

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func raw(m map[string]any) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

func f64(v float64) *float64 { return &v }

func TestNormalizeFormRejectsBadFields(t *testing.T) {
	f := Form{Fields: []FormField{
		{ID: "title", Type: FieldShortText, Label: "Title"},
		{ID: "ok", Type: FieldSingleChoice, Label: "Pick"},
		{ID: "ok", Type: FieldShortText, Label: "Dup"},
		{ID: "Bad-Id", Type: FieldShortText, Label: "x"},
		{ID: "weird", Type: "colour", Label: "x"},
	}}
	issues := normalizeForm(&f)
	for _, key := range []string{
		"form.fields[0].id", "form.fields[1].options", "form.fields[2].id",
		"form.fields[3].id", "form.fields[4].type",
	} {
		if _, ok := issues[key]; !ok {
			t.Errorf("expected issue %s, got %v", key, issues)
		}
	}
}

func TestCheckAnswersDraftVsFinal(t *testing.T) {
	f := Form{Fields: []FormField{
		{ID: "description", Type: FieldLongText, Label: "D", Required: true},
		{ID: "kind", Type: FieldSingleChoice, Label: "K", Options: []string{"Talk", "Show"}},
		{ID: "tags", Type: FieldMultiChoice, Label: "T", Options: []string{"a", "b"}, Max: f64(1)},
		{ID: "agree", Type: FieldCheckbox, Label: "A", Required: true},
		{ID: "site", Type: FieldURL, Label: "S"},
		{ID: "old", Type: FieldShortText, Label: "O", Archived: true, Required: true},
	}}

	draft := checkAnswers(f, raw(map[string]any{"kind": "Talk"}), false)
	if len(draft.issues) != 0 {
		t.Fatalf("draft should allow missing required answers: %v", draft.issues)
	}

	final := checkAnswers(f, raw(map[string]any{"kind": "Talk", "agree": false}), true)
	if final.issues["answers.description"] == "" || final.issues["answers.agree"] == "" {
		t.Fatalf("final should require description and ticked checkbox: %v", final.issues)
	}
	if _, ok := final.issues["answers.old"]; ok {
		t.Fatal("archived fields must not be required")
	}

	bad := checkAnswers(f, raw(map[string]any{"kind": "Dance", "tags": []string{"a", "b"}, "site": "ftp://x"}), false)
	for _, key := range []string{"answers.kind", "answers.tags", "answers.site"} {
		if bad.issues[key] == "" {
			t.Errorf("expected issue %s, got %v", key, bad.issues)
		}
	}
}

func TestCheckAnswersFiles(t *testing.T) {
	f := Form{Fields: []FormField{{ID: "rider", Type: FieldFile, Label: "Rider", MaxFiles: 1}}}
	id := uuid.New()
	ok := checkAnswers(f, raw(map[string]any{"rider": []string{id.String()}}), true)
	if len(ok.issues) != 0 || len(ok.files["rider"]) != 1 {
		t.Fatalf("one file should pass: %+v", ok)
	}
	tooMany := checkAnswers(f, raw(map[string]any{"rider": []string{id.String(), uuid.NewString()}}), true)
	if tooMany.issues["answers.rider"] == "" {
		t.Fatal("max_files not enforced")
	}
}

func TestAcceptsType(t *testing.T) {
	fd := FormField{Type: FieldFile, Accept: []string{"image/*", "application/pdf"}}
	for ct, want := range map[string]bool{
		"image/png": true, "IMAGE/JPEG": true, "application/pdf": true,
		"application/zip": false, "imagex/png": false,
	} {
		if got := fd.AcceptsType(ct); got != want {
			t.Errorf("%s: got %v want %v", ct, got, want)
		}
	}
}

func TestMergeAnswersKeepsArchived(t *testing.T) {
	f := Form{Fields: []FormField{
		{ID: "now", Type: FieldShortText, Label: "N"},
		{ID: "gone", Type: FieldShortText, Label: "G", Archived: true},
	}}
	out := mergeAnswers(f, map[string]any{"now": "old", "gone": "kept", "removed": "kept too"}, map[string]any{"now": "new"})
	if out["now"] != "new" || out["gone"] != "kept" || out["removed"] != "kept too" {
		t.Fatalf("unexpected merge: %v", out)
	}
}

func TestCleanFilename(t *testing.T) {
	for in, want := range map[string]string{
		`C:\fakepath\my rider.pdf`: "my rider.pdf",
		"../../etc/passwd":         "passwd",
		"weird<>name?.png":         "weird__name_.png",
	} {
		if got := cleanFilename(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
