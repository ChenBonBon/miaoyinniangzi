package sources

import (
	"context"
	"errors"
	"testing"

	"miaoyinniangzi/internal/sourceruntime"
)

type fakeInspector struct {
	report sourceruntime.Report
	calls  int
	err    error
}

func (i *fakeInspector) Inspect(context.Context, []byte) (sourceruntime.Report, error) {
	i.calls++
	return i.report, i.err
}
func TestCheckActivateDisableAndRecheck(t *testing.T) {
	m, s, _, _ := fixture(t)
	ctx := context.Background()
	inspector := &fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"flac", "320k"}}}
	m.checker = inspector
	imported, err := m.Import(ctx, script(t, t.TempDir(), "a.js", example))
	if err != nil {
		t.Fatal(err)
	}
	id := imported.Source.ID
	checked, err := m.Check(ctx, id)
	if err != nil || checked.CheckStatus != "ready" || checked.Active {
		t.Fatalf("check: %+v %v", checked, err)
	}
	if err := m.Activate(ctx, id); err != nil {
		t.Fatal(err)
	}
	if inspector.calls != 2 {
		t.Fatal("activation did not recheck")
	}
	active, err := s.Source(ctx, id)
	if err != nil || !active.Active {
		t.Fatal("not active")
	}
	if err := m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	active, err = s.Source(ctx, id)
	if err != nil || active.Active {
		t.Fatal("disable failed")
	}
	if err := m.Activate(ctx, id); err != nil {
		t.Fatal(err)
	}
	inspector.report = sourceruntime.Report{Error: "脚本未声明可用的 QQ 音乐地址解析能力", Qualities: []string{}}
	checked, err = m.Check(ctx, id)
	if err != nil || checked.CheckStatus != "failed" || checked.Active {
		t.Fatalf("recheck: %+v %v", checked, err)
	}
	if err := m.Activate(ctx, id); err == nil {
		t.Fatal("activated failed source")
	}
}
func TestCheckKeyLossAndProcessErrorNeverExposeScript(t *testing.T) {
	m, _, keys, _ := fixture(t)
	ctx := context.Background()
	imported, err := m.Import(ctx, script(t, t.TempDir(), "a.js", example))
	if err != nil {
		t.Fatal(err)
	}
	inspector := &fakeInspector{err: errors.New("SECRET interpreter exception")}
	m.checker = inspector
	checked, err := m.Check(ctx, imported.Source.ID)
	if err != nil || checked.CheckStatus != "failed" || checked.CheckError != "音源隔离校验失败或超出资源限制，请重试" {
		t.Fatalf("unsafe failure: %+v %v", checked, err)
	}
	keys.err = errors.New("keychain unavailable")
	if err := m.Activate(ctx, imported.Source.ID); err == nil {
		t.Fatal("activated without key")
	}
	if inspector.calls != 1 {
		t.Fatal("keychain failure reached worker")
	}
}
func TestDeletingActiveSourceDoesNotSwitchToAnother(t *testing.T) {
	m, s, _, _ := fixture(t)
	ctx := context.Background()
	m.checker = &fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"128k"}}}
	a, err := m.Import(ctx, script(t, t.TempDir(), "a.js", example))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Import(ctx, script(t, t.TempDir(), "b.js", example+"\n// second")); err != nil {
		t.Fatal(err)
	}
	if err := m.Activate(ctx, a.Source.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, a.Source.ID); err != nil {
		t.Fatal(err)
	}
	entries, err := s.Sources(ctx)
	if err != nil || len(entries) != 1 || entries[0].Active {
		t.Fatal("unexpected automatic fallback")
	}
}
