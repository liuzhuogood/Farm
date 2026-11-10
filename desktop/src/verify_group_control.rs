// Feature-gated native WebView verification; production builds omit this module.
use std::{thread, time::Duration};
use tauri::{AppHandle, Manager};

pub fn start(app: &AppHandle) {
    let app = app.clone();
    thread::spawn(move || {
        thread::sleep(Duration::from_secs(3));
        let Some(main) = app.get_webview_window("main") else {
            app.exit(1);
            return;
        };
        let config = serde_json::json!({
            "source": std::env::var("FARM_VERIFY_GROUP_SOURCE").ok(),
            "target": std::env::var("FARM_VERIFY_GROUP_TARGET").ok(),
        });
        let script = format!(
            "window.__nativeGroupControlConfig = {config};\n{}",
            include_str!("verify_group_control.js")
        );
        if let Err(error) = main.eval(script) {
            eprintln!("[Desktop Verify GroupControl] {error}");
            app.exit(1);
            return;
        }
        let report = std::env::var("FARM_NATIVE_REPORT").unwrap();
        for _ in 0..600 {
            for (label, window) in app.webview_windows() {
                if label.starts_with("player-") {
                    let _ = window.eval(include_str!("verify_group_control_worker.js"));
                }
            }
            let finished = std::fs::read(&report)
                .ok()
                .and_then(|bytes| serde_json::from_slice::<serde_json::Value>(&bytes).ok())
                .and_then(|value| value["group_control"]["ok"].as_bool());
            if let Some(ok) = finished {
                app.exit(if ok { 0 } else { 1 });
                return;
            }
            thread::sleep(Duration::from_millis(100));
        }
        eprintln!("[Desktop Verify GroupControl] native verification timed out");
        app.exit(1);
    });
}
