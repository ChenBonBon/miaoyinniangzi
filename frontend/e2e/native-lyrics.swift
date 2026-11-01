// Opt-in system WKWebView UI regression. Serve the production frontend on
// localhost first. All music/lyrics below are synthetic bridge fixtures; no
// native audio, user database, source script or network service is accessed.
import Cocoa
import WebKit

final class Probe: NSObject, WKScriptMessageHandler {
    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        let value = message.body as? String ?? "INVALID"
        print(value)
        exit(value == "SYSTEM_WEBKIT_LYRICS_OK" ? 0 : 1)
    }
}
let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let probe = Probe()
let config = WKWebViewConfiguration()
config.userContentController.add(probe, name: "probe")
let script = #"""
const song = {id:'Song0',source:'tx',title:'UI regression',artist:'Test',album:'Test',coverUrl:'',duration:120};
const native = {position:0,duration:120,playing:false,ended:false,seekable:true};
let prepares = 0;
window.go={main:{App:{
 GetPlayerSession:async()=>({playback:{queue:[song],index:0,position:0,mode:'sequential'},volume:0}),
 SavePlayback:async()=>{},
 GetSettings:async()=>({downloadDirectory:'/Downloads',downloadQuality:'highest',playbackQuality:'highest',volume:0}),
 SaveSettings:async()=>{},
 PrepareSong:async(s,id)=>{prepares++;return {songId:s.id,engine:'libmpv',nativeSession:id,mediaUrl:'',quality:'flac',codec:'flac',duration:120,seekable:true,downgraded:false}},
 NativePlayback:async(id,action,value)=>{if(action==='play')native.playing=true;if(action==='pause')native.playing=false;if(action==='seek')native.position=value;return {...native}},
 CancelPlayback:async()=>true,
 GetLyrics:async(id)=>({songId:id,lines:Array.from({length:100},(_,i)=>({time:i,text:'Test line '+i})),text:''}),
 CancelLyrics:async()=>true,
 GetSearchHistory:async()=>[],SuggestSongs:async()=>[],CancelSongSuggestions:async()=>true
}}};
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
async function wait(check){for(let i=0;i<100;i++){const v=check();if(v)return v;await sleep(50)}throw Error('UI_WAIT_TIMEOUT')}
window.addEventListener('load',async()=>{
 try{
  (await wait(()=>document.querySelector('button[aria-label="打开播放详情"]'))).click();
  const pane=await wait(()=>document.querySelector('[aria-label="歌词"]'));
  const line=await wait(()=>pane.querySelector('[data-lyric-index="20"]'));
  line.click();await sleep(150);
  if(prepares!==0||Number(document.querySelector('[aria-label="播放进度"]').getAttribute('aria-valuenow'))!==20)throw Error('RESTORED_SEEK_FAILED');
  document.querySelector('footer button[aria-label="播放"]').click();
  await wait(()=>native.playing);native.position=40;await sleep(800);
  const firstActive=pane.querySelector('[aria-current="true"]');
  if(!firstActive)throw Error('AUTO_ACTIVE_MISSING');
  const firstPaneBox=pane.getBoundingClientRect(),firstLineBox=firstActive.getBoundingClientRect();
  if(Math.abs(firstLineBox.top+firstLineBox.height/2-firstPaneBox.top-firstPaneBox.height/2)>12)throw Error('AUTO_FOLLOW_FAILED');
  document.querySelector('footer button[aria-label="暂停"]').click();
  await wait(()=>!native.playing);
  pane.dispatchEvent(new WheelEvent('wheel',{bubbles:true,deltaY:900}));pane.scrollTop=900;
  await sleep(600);
  if(Math.abs(pane.scrollTop-900)>5)throw Error('PAUSED_SCROLL_RESET');
  document.querySelector('footer button[aria-label="播放"]').click();
  await wait(()=>native.playing);await sleep(500);
  if(Math.abs(pane.scrollTop-900)>5)throw Error('PLAYING_SCROLL_RESET');
  native.position=80;await sleep(300);
  const follow=Array.from(document.querySelectorAll('button')).find(e=>e.textContent.includes('回到当前歌词'));
  if(!follow)throw Error('FOLLOW_CONTROL_MISSING');follow.click();await sleep(900);
  const active=pane.querySelector('[aria-current="true"]');
  if(!active)throw Error('ACTIVE_LINE_MISSING');
  const p=pane.getBoundingClientRect(),a=active.getBoundingClientRect();
  if(Math.abs(a.top+a.height/2-p.top-p.height/2)>12)throw Error('FOLLOW_NOT_CENTERED');
  window.webkit.messageHandlers.probe.postMessage('SYSTEM_WEBKIT_LYRICS_OK');
 }catch(e){window.webkit.messageHandlers.probe.postMessage(String(e.message))}
});
"""#
config.userContentController.addUserScript(WKUserScript(source: script, injectionTime: .atDocumentStart, forMainFrameOnly: true))
let view = WKWebView(frame: NSRect(x: 0, y: 0, width: 1180, height: 800), configuration: config)
let window = NSWindow(contentRect: view.frame, styleMask: [.titled], backing: .buffered, defer: false)
window.title = "Lyrics UI regression"
window.contentView = view
window.orderFront(nil)
view.load(URLRequest(url: URL(string: "http://127.0.0.1:14873")!))
DispatchQueue.main.asyncAfter(deadline: .now() + 20) {print("SYSTEM_WEBKIT_TIMEOUT");exit(1)}
app.run()
