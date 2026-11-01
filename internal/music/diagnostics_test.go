package music

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readDiagnostic(t *testing.T, dir string) diagnostic {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "qq-metadata.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var event diagnostic
	if err = json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	return event
}
func TestSearchDiagnosticsCodesAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		result     string
	}{
		{"filter", fixtureResponse("[]", `{"curpage":1,"sum":0,"ret":0,"is_filter":-2}`), 200, "restricted"},
		{"outer", `{"code":1001,"message":"BODY-SECRET"}`, 200, "restricted"},
		{"request", `{"code":0,"req_1":{"code":2001}}`, 200, "restricted"},
		{"data", `{"code":0,"req_1":{"code":0,"data":{"code":3001}}}`, 200, "restricted"},
		{"forbidden", "BODY-SECRET", 403, "restricted"},
		{"rate", "BODY-SECRET", 429, "rate_limit"},
		{"invalid", "BODY-SECRET", 200, "invalid_response"},
		{"success", goodResponse(), 200, "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			client := newQQClient(server.Client().Transport, server.URL).WithDiagnostics(NewDiagnostics(dir))
			client.Search(context.Background(), "QUERY-SECRET", 1)
			event := readDiagnostic(t, dir)
			if event.Operation != "search" || event.HTTPStatus != tc.status || event.Result != tc.result || event.Time == "" || event.DurationMS < 0 {
				t.Fatalf("bad diagnostic %+v", event)
			}
			switch tc.name {
			case "filter":
				if event.Filter == nil || *event.Filter != -2 || event.MetaRet == nil || *event.MetaRet != 0 {
					t.Fatal("missing filter diagnostics")
				}
			case "outer":
				if event.Code == nil || *event.Code != 1001 {
					t.Fatal("missing outer code")
				}
			case "request":
				if event.RequestCode == nil || *event.RequestCode != 2001 {
					t.Fatal("missing request code")
				}
			case "data":
				if event.DataCode == nil || *event.DataCode != 3001 {
					t.Fatal("missing data code")
				}
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "qq-metadata.jsonl"))
			for _, secret := range []string{"QUERY-SECRET", "BODY-SECRET", server.URL, "晴天", "周杰伦", "叶惠美"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("leaked %q", secret)
				}
			}
		})
	}
}
func TestDiagnosticsDistinguishAbsentCodesFromZero(t *testing.T) {
	event := diagnostic{}
	_, err := decodeSearchDiagnostic([]byte(fixtureResponse("[]", `{"curpage":1,"sum":0}`)), "歌曲", 1, &event)
	if err != nil || event.Code == nil || *event.Code != 0 || event.MetaRet != nil || event.Filter != nil {
		t.Fatalf("absent metadata codes were fabricated: %+v %v", event, err)
	}
	event = diagnostic{}
	_, err = decodeSuggestionsDiagnostic([]byte(`{"code":0,"data":{}}`), &event)
	if err != nil || event.Code == nil || *event.Code != 0 || event.Subcode != nil {
		t.Fatal("absent subcode was fabricated")
	}
}

func TestSuggestionDiagnosticsAndBrokenDestination(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"subcode":42,"message":"SECRET","data":{}}`)
	}))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL).WithDiagnostics(NewDiagnostics(dir))
	client.suggestionEndpoint = server.URL
	client.Suggest(context.Background(), "QUERY-SECRET")
	event := readDiagnostic(t, dir)
	if event.Operation != "suggestions" || event.Result != "restricted" || event.Subcode == nil || *event.Subcode != 42 {
		t.Fatalf("%+v", event)
	}
	broken := filepath.Join(t.TempDir(), "file")
	os.WriteFile(broken, []byte("not a directory"), 0600)
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, goodResponse()) }))
	defer good.Close()
	client = newQQClient(good.Client().Transport, good.URL).WithDiagnostics(NewDiagnostics(broken))
	if _, err := client.Search(context.Background(), "歌曲", 1); err != nil {
		t.Fatalf("diagnostic failure broke search: %v", err)
	}
}
func TestDiagnosticsRotationPermissionsAndConcurrentWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	logger := NewDiagnostics(dir)
	var wait sync.WaitGroup
	for i := 0; i < 4; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 250; j++ {
				logger.record(diagnostic{Operation: "search"}, time.Now(), ErrRestricted)
			}
		}()
	}
	wait.Wait()
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("rotation: %v %v", files, err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal("directory is not private")
	}
	for _, entry := range files {
		info, _ := entry.Info()
		if info.Size() > diagnosticLimit || info.Mode().Perm() != 0600 {
			t.Fatal("invalid log bounds or permissions")
		}
		data, _ := os.ReadFile(filepath.Join(dir, entry.Name()))
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if !json.Valid([]byte(line)) {
				t.Fatal("interleaved or invalid JSON")
			}
		}
	}
}
func TestDiagnosticsRefuseSymlinkAndNeverLogRawErrors(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("preserved"), 0600)
	if err := os.Symlink(outside, filepath.Join(dir, "qq-metadata.jsonl")); err != nil {
		t.Fatal(err)
	}
	NewDiagnostics(dir).record(diagnostic{Operation: "search"}, time.Now(), fmt.Errorf("SECRET-URL"))
	data, _ := os.ReadFile(outside)
	if string(data) != "preserved" {
		t.Fatal("followed log symlink")
	}
	os.Remove(filepath.Join(dir, "qq-metadata.jsonl"))
	NewDiagnostics(dir).record(diagnostic{Operation: "search"}, time.Now(), fmt.Errorf("SECRET-URL"))
	data, _ = os.ReadFile(filepath.Join(dir, "qq-metadata.jsonl"))
	if strings.Contains(string(data), "SECRET") {
		t.Fatal("raw error leaked")
	}
}
