package sources

import (
	"context"
	"testing"

	"miaoyinniangzi/internal/sourceruntime"
)

func TestUserImportAutomaticallyValidatesIncludingDuplicates(t *testing.T) {
	m, _, _, _ := fixture(t)
	ctx := context.Background()
	inspector := &fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"flac", "320k"}}}
	m.checker = inspector
	path := script(t, t.TempDir(), "source.js", example)
	first, err := m.ImportAndValidate(ctx, path)
	if err != nil || first.Source.CheckStatus != "ready" || first.Source.Active || first.ValidationError != "" || inspector.calls != 1 {
		t.Fatalf("import validation: %+v %v", first, err)
	}
	if err := m.Activate(ctx, first.Source.ID); err != nil {
		t.Fatal(err)
	}
	inspector.report = sourceruntime.Report{Error: "脚本未声明可用的 QQ 音乐地址解析能力", Qualities: []string{}}
	duplicate, err := m.ImportAndValidate(ctx, path)
	if err != nil || !duplicate.Duplicate || duplicate.Source.CheckStatus != "failed" || duplicate.Source.Active || inspector.calls != 3 {
		t.Fatalf("duplicate validation: %+v %v", duplicate, err)
	}
	entries, err := m.List(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed validation deleted or duplicated the copy")
	}
	if _, err := m.ImportAndValidate(ctx, script(t, t.TempDir(), "wrong.txt", "invalid")); err == nil || inspector.calls != 3 {
		t.Fatal("invalid import triggered validation")
	}
}

type cancelingInspector struct{ cancel context.CancelFunc }

func (i cancelingInspector) Inspect(context.Context, []byte) (sourceruntime.Report, error) {
	i.cancel()
	return sourceruntime.Report{Ready: true, Qualities: []string{"128k"}}, nil
}
func TestValidationInterruptedAfterImportPreservesCopyAndReportsSeparateError(t *testing.T) {
	m, _, _, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.checker = cancelingInspector{cancel}
	result, err := m.ImportAndValidate(ctx, script(t, t.TempDir(), "source.js", example))
	if err != nil || result.Source.ID == "" || result.ValidationError == "" {
		t.Fatalf("incorrect import result: %+v %v", result, err)
	}
	entries, err := m.List(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Active || entries[0].CheckStatus != "unchecked" {
		t.Fatal("interrupted validation lost copy or enabled it")
	}
}
