package main

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"

	"miaoyinniangzi/internal/sourcenetwork"
	"miaoyinniangzi/internal/sourceruntime"
)

const sandboxSmokeScript = `
 lx.on(lx.EVENT_NAMES.request,()=>new Promise((resolve,reject)=>{
  lx.request('https://sandbox-probe.invalid/resolve',{method:'GET'},(err,response)=>{
   if(err)reject(Error('probe'));else resolve(response.body.url);
  });
 }));
 lx.send(lx.EVENT_NAMES.inited,{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['flac']}}});`

type smokeProxy struct{ calls int }

func (p *smokeProxy) Do(_ context.Context, request sourcenetwork.Request) (sourcenetwork.Response, error) {
	if request.URL != "https://sandbox-probe.invalid/resolve" || request.Method != "GET" || p.calls != 0 {
		return sourcenetwork.Response{}, sourcenetwork.ErrDenied
	}
	p.calls++
	return sourcenetwork.Response{Status: 200, Body: []byte(`{"url":"https://media.sandbox-probe.invalid/audio.flac"}`)}, nil
}
func (*smokeProxy) ValidateMedia(_ context.Context, url string) error {
	if url != "https://media.sandbox-probe.invalid/audio.flac" {
		return sourcenetwork.ErrDenied
	}
	return nil
}
func sandboxSmoke(executable string) error {
	if runtime.GOOS != "windows" {
		return errors.New("source-worker smoke must run on Windows")
	}
	helper := filepath.Join(filepath.Dir(executable), "miaoyin-source-worker.exe")
	return runSandboxSmoke(helper)
}
func runSandboxSmoke(helper string) error {
	proxy := &smokeProxy{}
	inspector := &sourceruntime.ProcessInspector{Executable: helper, Network: proxy}
	report, err := inspector.Inspect(context.Background(), []byte(sandboxSmokeScript))
	if err != nil || !report.Ready {
		return errors.New("packaged source-worker offline inspection failed")
	}
	result, err := inspector.Resolve(context.Background(), []byte(sandboxSmokeScript), sourceruntime.ResolveInput{SongMID: "SandboxProbe1"}, "flac")
	if err != nil || result.Error != "" || result.Quality != "flac" || proxy.calls != 1 {
		return errors.New("packaged source-worker broker protocol failed")
	}
	return nil
}
