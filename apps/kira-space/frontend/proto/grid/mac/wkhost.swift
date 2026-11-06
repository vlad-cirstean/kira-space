// wkhost.swift: swiftc -O -o /tmp/wkhost wkhost.swift
// usage: /tmp/wkhost <url> <hold-seconds> <driver.js> 2> run.log
// Appendix A harness of `git show 7d8a72ab4a5e:docs/WEBVIEW-SCROLL-MEMORY.md` plus an in-process 4 Hz footprint poll.
// stderr: WEBPID, LOADED, TITLE <mark>, FOOTPRINT <seconds> <mark> <MB>.
import Cocoa
import Darwin
import WebKit

let args = CommandLine.arguments
let urlString = args.count > 1 ? args[1] : "about:blank"
let holdSeconds = args.count > 2 ? Double(args[2]) ?? 90.0 : 90.0
let jsPath = args.count > 3 ? args[3] : ""

func log(_ s: String) { FileHandle.standardError.write((s + "\n").data(using: .utf8)!) }

/// ri_phys_footprint of one process, in MB. Never vmmap: it suspends the target.
func footprintMB(_ pid: Int32) -> Double? {
    var info = rusage_info_v2()
    let status = withUnsafeMutablePointer(to: &info) { pointer in
        pointer.withMemoryRebound(to: rusage_info_t?.self, capacity: 1) {
            proc_pid_rusage(pid, RUSAGE_INFO_V2, $0)
        }
    }
    return status == 0 ? Double(info.ri_phys_footprint) / 1_048_576 : nil
}

class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var webPid: Int32 = 0
    var mark = "start"
    let started = Date()

    func applicationDidFinishLaunching(_ n: Notification) {
        let rect = NSRect(x: 0, y: 0, width: 1440, height: 960)
        window = NSWindow(contentRect: rect,
                          styleMask: [.titled, .closable, .resizable],
                          backing: .buffered, defer: false)
        web = WKWebView(frame: rect, configuration: WKWebViewConfiguration())
        web.navigationDelegate = self
        window.contentView = web
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        web.load(URLRequest(url: URL(string: urlString)!))
    }

    func webView(_ w: WKWebView, didFinish nav: WKNavigation!) {
        if let v = w.value(forKey: "_webProcessIdentifier") as? Int32 {
            webPid = v
            log("WEBPID \(v)")
        }
        log("LOADED")
        Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { _ in
            guard self.webPid != 0, let mb = footprintMB(self.webPid) else { return }
            let t = Date().timeIntervalSince(self.started)
            log(String(format: "FOOTPRINT %.2f %@ %.1f", t, self.mark, mb))
        }
        guard !jsPath.isEmpty, let js = try? String(contentsOfFile: jsPath, encoding: .utf8) else { return }
        DispatchQueue.main.asyncAfter(deadline: .now() + 4.0) {
            w.evaluateJavaScript(js) { _, err in if let e = err { log("JSERR \(e)") } }
        }
        // The driver publishes phase markers through document.title.
        Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { _ in
            w.evaluateJavaScript("document.title") { r, _ in
                if let t = r as? String, t.hasPrefix("MARK-"), t != self.mark {
                    self.mark = t
                    log("TITLE \(t)")
                }
            }
        }
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.setActivationPolicy(.regular)
app.delegate = delegate
DispatchQueue.main.asyncAfter(deadline: .now() + holdSeconds) { NSApp.terminate(nil) }
app.run()
