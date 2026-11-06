package events

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Field types an events lead can put on a submission form.
const (
	FieldShortText    = "short_text"
	FieldLongText     = "long_text"
	FieldNumber       = "number"
	FieldDuration     = "duration" // minutes
	FieldSingleChoice = "single_choice"
	FieldMultiChoice  = "multi_choice"
	FieldCheckbox     = "checkbox"
	FieldEmail        = "email"
	FieldURL          = "url"
	FieldDateTime     = "datetime"
	FieldFile         = "file"
)

const (
	maxFormFields      = 50
	maxFieldLabelLen   = 200
	maxFieldHelpLen    = 1000
	maxFieldOptions    = 50
	maxOptionLen       = 200
	defaultShortMax    = 200
	defaultLongMax     = 10000
	defaultMaxFiles    = 1
	maxFilesPerField   = 10
	defaultMaxSizeMB   = 10
	maxSizeMBLimit     = 100
	maxAcceptPatterns  = 20
	maxStatusLabelLen  = 60
	maxEventTypesInCfP = 100
)

var fieldIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// reservedFieldIDs are the system fields stored in their own columns because
// scheduling depends on them.
var reservedFieldIDs = map[string]struct{}{
	"title": {}, "event_type": {}, "duration": {}, "people": {},
}

// FormField is one question on a submission form.
type FormField struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Help     string   `json:"help,omitempty"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
	// Min/Max: length for text fields, value for number/duration.
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Accept    []string `json:"accept,omitempty"` // file: "image/*", "application/pdf", ...
	MaxFiles  int      `json:"max_files,omitempty"`
	MaxSizeMB float64  `json:"max_size_mb,omitempty"`
	// Archived fields are hidden from the form but old answers stay readable.
	Archived bool `json:"archived,omitempty"`
}

// Form is the configurable part of a call for papers' submission form.
type Form struct {
	Fields []FormField `json:"fields"`
}

// DefaultForm is what a new call for papers starts with.
func DefaultForm() Form {
	return Form{Fields: []FormField{{
		ID:       "description",
		Type:     FieldLongText,
		Label:    "Description",
		Help:     "What is it about, and who is it for?",
		Required: true,
	}}}
}

// Field returns the active (non-archived) field with id.
func (f Form) Field(id string) (FormField, bool) {
	for _, fd := range f.Fields {
		if fd.ID == id && !fd.Archived {
			return fd, true
		}
	}
	return FormField{}, false
}

func (fd FormField) maxFiles() int {
	if fd.MaxFiles <= 0 {
		return defaultMaxFiles
	}
	return fd.MaxFiles
}

// MaxBytes is the per-file size limit of a file field.
func (fd FormField) MaxBytes() int64 {
	mb := fd.MaxSizeMB
	if mb <= 0 {
		mb = defaultMaxSizeMB
	}
	return int64(mb * 1024 * 1024)
}

// AcceptsType reports whether contentType matches the field's accept list
// (empty = anything).
func (fd FormField) AcceptsType(contentType string) bool {
	if len(fd.Accept) == 0 {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	for _, a := range fd.Accept {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == ct || (strings.HasSuffix(a, "/*") && strings.HasPrefix(ct, strings.TrimSuffix(a, "*"))) {
			return true
		}
	}
	return false
}

// normalizeForm trims and validates a form definition.
func normalizeForm(f *Form) map[string]string {
	issues := map[string]string{}
	if f.Fields == nil {
		f.Fields = []FormField{}
	}
	if len(f.Fields) > maxFormFields {
		issues["form.fields"] = fmt.Sprintf("at most %d fields", maxFormFields)
		return issues
	}
	seen := map[string]struct{}{}
	for i := range f.Fields {
		fd := &f.Fields[i]
		key := fmt.Sprintf("form.fields[%d]", i)
		fd.ID = strings.TrimSpace(fd.ID)
		fd.Label = strings.TrimSpace(fd.Label)
		fd.Help = strings.TrimSpace(fd.Help)
		switch {
		case !fieldIDPattern.MatchString(fd.ID):
			issues[key+".id"] = "must start with a letter and use a-z, 0-9, _ (max 40)"
		case isReserved(fd.ID):
			issues[key+".id"] = "is reserved for a built-in field"
		default:
			if _, dup := seen[fd.ID]; dup {
				issues[key+".id"] = "is used twice"
			}
			seen[fd.ID] = struct{}{}
		}
		if fd.Label == "" {
			issues[key+".label"] = "is required"
		} else if len([]rune(fd.Label)) > maxFieldLabelLen {
			issues[key+".label"] = "is too long"
		}
		if len([]rune(fd.Help)) > maxFieldHelpLen {
			issues[key+".help"] = "is too long"
		}
		if fd.Min != nil && fd.Max != nil && *fd.Min > *fd.Max {
			issues[key+".max"] = "must be at least min"
		}
		switch fd.Type {
		case FieldSingleChoice, FieldMultiChoice:
			opts := make([]string, 0, len(fd.Options))
			uniq := map[string]struct{}{}
			for _, o := range fd.Options {
				o = strings.TrimSpace(o)
				if o == "" {
					continue
				}
				if _, dup := uniq[o]; dup || len([]rune(o)) > maxOptionLen {
					issues[key+".options"] = "options must be unique and at most 200 characters"
					continue
				}
				uniq[o] = struct{}{}
				opts = append(opts, o)
			}
			if len(opts) == 0 || len(opts) > maxFieldOptions {
				issues[key+".options"] = fmt.Sprintf("needs 1 to %d options", maxFieldOptions)
			}
			fd.Options = opts
		case FieldFile:
			fd.Options = nil
			if fd.MaxFiles < 0 || fd.MaxFiles > maxFilesPerField {
				issues[key+".max_files"] = fmt.Sprintf("must be between 1 and %d", maxFilesPerField)
			}
			if fd.MaxSizeMB < 0 || fd.MaxSizeMB > maxSizeMBLimit {
				issues[key+".max_size_mb"] = fmt.Sprintf("must be at most %d", maxSizeMBLimit)
			}
			if len(fd.Accept) > maxAcceptPatterns {
				issues[key+".accept"] = "too many file types"
			}
		case FieldShortText, FieldLongText, FieldNumber, FieldDuration,
			FieldCheckbox, FieldEmail, FieldURL, FieldDateTime:
			fd.Options = nil
		default:
			issues[key+".type"] = "unknown field type"
		}
		if fd.Type != FieldFile {
			fd.Accept, fd.MaxFiles, fd.MaxSizeMB = nil, 0, 0
		}
	}
	return issues
}

func isReserved(id string) bool {
	_, ok := reservedFieldIDs[id]
	return ok
}

// answerCheck is the outcome of validating submitted answers against a form.
type answerCheck struct {
	answers map[string]any         // cleaned answers for active fields
	files   map[string][]uuid.UUID // file field id -> referenced file ids
	issues  map[string]string
}

// checkAnswers validates raw answers against the form. With final=false
// (saving a draft) required fields may be empty; with final=true they may not.
func checkAnswers(f Form, raw map[string]json.RawMessage, final bool) answerCheck {
	out := answerCheck{answers: map[string]any{}, files: map[string][]uuid.UUID{}, issues: map[string]string{}}
	for _, fd := range f.Fields {
		if fd.Archived {
			continue
		}
		key := "answers." + fd.ID
		val, present := raw[fd.ID]
		if present && (string(val) == "null" || len(val) == 0) {
			present = false
		}
		v, empty, msg := parseAnswer(fd, val, present)
		if msg != "" {
			out.issues[key] = msg
			continue
		}
		if empty {
			if final && fd.Required {
				out.issues[key] = "is required"
			}
			continue
		}
		if ids, ok := v.([]uuid.UUID); ok {
			out.files[fd.ID] = ids
			strs := make([]string, len(ids))
			for i, id := range ids {
				strs[i] = id.String()
			}
			out.answers[fd.ID] = strs
			continue
		}
		out.answers[fd.ID] = v
	}
	return out
}

// parseAnswer returns the cleaned value, whether it counts as empty, and an
// error message.
func parseAnswer(fd FormField, val json.RawMessage, present bool) (any, bool, string) {
	if !present {
		return nil, true, ""
	}
	switch fd.Type {
	case FieldShortText, FieldLongText, FieldEmail, FieldURL, FieldDateTime, FieldSingleChoice:
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, false, "must be text"
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, true, ""
		}
		return checkText(fd, s)
	case FieldNumber, FieldDuration:
		var n float64
		if err := json.Unmarshal(val, &n); err != nil {
			return nil, false, "must be a number"
		}
		if fd.Type == FieldDuration && (n <= 0 || n != float64(int(n))) {
			return nil, false, "must be whole minutes"
		}
		if fd.Min != nil && n < *fd.Min {
			return nil, false, fmt.Sprintf("must be at least %g", *fd.Min)
		}
		if fd.Max != nil && n > *fd.Max {
			return nil, false, fmt.Sprintf("must be at most %g", *fd.Max)
		}
		return n, false, ""
	case FieldCheckbox:
		var b bool
		if err := json.Unmarshal(val, &b); err != nil {
			return nil, false, "must be true or false"
		}
		// An unticked required checkbox counts as missing.
		return b, !b, ""
	case FieldMultiChoice:
		var list []string
		if err := json.Unmarshal(val, &list); err != nil {
			return nil, false, "must be a list of options"
		}
		picked := make([]string, 0, len(list))
		seen := map[string]struct{}{}
		for _, s := range list {
			s = strings.TrimSpace(s)
			if !contains(fd.Options, s) {
				return nil, false, fmt.Sprintf("%q is not an option", s)
			}
			if _, dup := seen[s]; !dup {
				seen[s] = struct{}{}
				picked = append(picked, s)
			}
		}
		if len(picked) == 0 {
			return nil, true, ""
		}
		if fd.Min != nil && float64(len(picked)) < *fd.Min {
			return nil, false, fmt.Sprintf("pick at least %g", *fd.Min)
		}
		if fd.Max != nil && float64(len(picked)) > *fd.Max {
			return nil, false, fmt.Sprintf("pick at most %g", *fd.Max)
		}
		return picked, false, ""
	case FieldFile:
		var list []string
		if err := json.Unmarshal(val, &list); err != nil {
			return nil, false, "must be a list of uploaded files"
		}
		ids := make([]uuid.UUID, 0, len(list))
		for _, s := range list {
			id, err := uuid.Parse(s)
			if err != nil {
				return nil, false, "contains an invalid file id"
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return nil, true, ""
		}
		if len(ids) > fd.maxFiles() {
			return nil, false, fmt.Sprintf("at most %d file(s)", fd.maxFiles())
		}
		return ids, false, ""
	}
	return nil, false, "unknown field type"
}

func checkText(fd FormField, s string) (any, bool, string) {
	n := float64(len([]rune(s)))
	limit := float64(defaultShortMax)
	if fd.Type == FieldLongText {
		limit = defaultLongMax
	}
	if fd.Max != nil && *fd.Max < limit && (fd.Type == FieldShortText || fd.Type == FieldLongText) {
		limit = *fd.Max
	}
	if n > limit {
		return nil, false, fmt.Sprintf("must be at most %g characters", limit)
	}
	switch fd.Type {
	case FieldShortText, FieldLongText:
		if fd.Min != nil && n < *fd.Min {
			return nil, false, fmt.Sprintf("must be at least %g characters", *fd.Min)
		}
	case FieldEmail:
		if a, err := mail.ParseAddress(s); err != nil || a.Address != s {
			return nil, false, "must be an email address"
		}
	case FieldURL:
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, false, "must be a link starting with http:// or https://"
		}
	case FieldDateTime:
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return nil, false, "must be a date and time"
		}
	case FieldSingleChoice:
		if !contains(fd.Options, s) {
			return nil, false, "is not one of the options"
		}
	}
	return s, false, ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// normalizeStatusLabels keeps only known states with short, non-empty labels.
func normalizeStatusLabels(in map[string]string) (map[string]string, map[string]string) {
	out := map[string]string{}
	issues := map[string]string{}
	for k, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch k {
		case "draft", "submitted", "accepted", "rejected", "withdrawn":
		default:
			issues["status_labels."+k] = "unknown state"
			continue
		}
		if len([]rune(v)) > maxStatusLabelLen {
			issues["status_labels."+k] = "is too long"
			continue
		}
		out[k] = v
	}
	return out, issues
}

func validationIfAny(msg string, issues map[string]string) error {
	if len(issues) == 0 {
		return nil
	}
	return httperror.Validation(msg, issues)
}
