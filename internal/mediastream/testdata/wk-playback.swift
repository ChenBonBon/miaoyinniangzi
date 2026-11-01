import Cocoa
import WebKit

final class Probe: NSObject, WKScriptMessageHandler, WKURLSchemeHandler {
    var html = ""
    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        if (message.body as? String) == "ok" {
            print("WEBKIT_FLAC_SEEK_OK")
            exit(0)
        }
        print("WEBKIT_AUDIO_FAILED")
        exit(1)
    }
    func webView(_ webView: WKWebView, start task: WKURLSchemeTask) {
        let data = html.data(using: .utf8)!
        task.didReceive(URLResponse(url: task.request.url!, mimeType: "text/html", expectedContentLength: data.count, textEncodingName: "utf-8"))
        task.didReceive(data)
        task.didFinish()
    }
    func webView(_ webView: WKWebView, stop task: WKURLSchemeTask) {}
}
let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let probe = Probe()
let configuration = WKWebViewConfiguration()
configuration.mediaTypesRequiringUserActionForPlayback = []
configuration.setURLSchemeHandler(probe, forURLScheme: "wails")
configuration.userContentController.add(probe, name: "probe")
let url = String(data: try! JSONSerialization.data(withJSONObject: [CommandLine.arguments[1]]), encoding: .utf8)!
probe.html = """
<html><body><script>
const audio = new Audio(JSON.parse('\(url)')[0]);
audio.muted = true;
let moved = false;
audio.onerror = () => window.webkit.messageHandlers.probe.postMessage('error');
audio.onplaying = () => { if(!moved){moved=true;audio.currentTime=3;} };
audio.onseeked = () => {if(audio.currentTime>2.9 && audio.duration>5.9)window.webkit.messageHandlers.probe.postMessage('ok');};
audio.play().catch(()=>window.webkit.messageHandlers.probe.postMessage('play rejected'));
</script></body></html>
"""
let view = WKWebView(frame: NSRect(x: 0, y: 0, width: 400, height: 200), configuration: configuration)
let window = NSWindow(contentRect: view.frame, styleMask: [.borderless], backing: .buffered, defer: false)
window.contentView = view
view.load(URLRequest(url: URL(string: "wails://wails/index")!))
DispatchQueue.main.asyncAfter(deadline: .now() + 12) { print("WEBKIT_TIMEOUT"); exit(1) }
app.run()
