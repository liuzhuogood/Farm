use crate::State;
use tauri::{AppHandle, Emitter, Manager};
use tauri_plugin_opener::OpenerExt;

#[tauri::command]
pub fn open_browser_link(app: AppHandle, url: String) -> Result<(), String> {
    let destination = tauri::Url::parse(&url).map_err(|error| error.to_string())?;
    let origin = tauri::Url::parse(&app.state::<State>().url)
        .map_err(|error| error.to_string())?;
    if destination.scheme() != "https" && destination.origin() != origin.origin() {
        return Err("不支持的浏览器链接".into());
    }
    app.opener().open_url(destination.as_str(), None::<&str>).map_err(|error| {
        eprintln!("[Desktop] 打开浏览器链接失败: {error}");
        error.to_string()
    })
}

#[tauri::command]
pub fn open_device_window(
    app: AppHandle,
    serial: String,
    title: Option<String>,
    width: Option<f64>,
    height: Option<f64>,
) -> Result<(), String> {
    open_device_window_internal(&app, serial, title, width, height)
}

pub(crate) fn open_device_window_internal(
    app: &AppHandle,
    serial: String,
    title: Option<String>,
    width: Option<f64>,
    height: Option<f64>,
) -> Result<(), String> {
    let state = app.state::<State>();
    let safe_serial = serial.replace(|c: char| !c.is_alphanumeric(), "_");
    let label = format!("player-{}", safe_serial);
    crate::diag::log(format!(
        "[Desktop] 请求打开设备小窗: serial={serial} label={label} size={:?}x{:?}",
        width, height
    ));

    if let Some(existing) = app.get_webview_window(&label) {
        crate::player_windows::restore(app, &label)?;
        let _ = existing.show();
        let _ = existing.unminimize();
        let _ = existing.set_focus();
        crate::diag::log(format!("[Desktop] 已恢复设备小窗: serial={serial} label={label}"));
        return Ok(());
    }

    let mut dest = tauri::Url::parse(&format!("{}/player", state.url))
        .map_err(|e| e.to_string())?;
    dest.query_pairs_mut()
        .append_pair("serial", &serial)
        .append_pair("embedded", "1");

    let win_title = title.unwrap_or_else(|| format!("投屏 - {}", serial));
    let w = width.unwrap_or(380.0);
    let h = height.unwrap_or(836.0);
    let borderless = true;

    let token_script = if let Ok(guard) = state.backend.lock() {
        if let Some(b) = guard.as_ref() {
            let t = serde_json::to_string(&b.ready.token).unwrap_or_default();
            format!("Object.defineProperty(window,'__FARM_DESKTOP_TOKEN__',{{value:{}}});", t)
        } else {
            String::new()
        }
    } else {
        String::new()
    };

    let builder = tauri::WebviewWindowBuilder::new(app, &label, tauri::WebviewUrl::External(dest.clone()))
        .title(win_title)
        .inner_size(w, h)
        .decorations(!borderless)
        .on_page_load(|window, payload| {
            let event = match payload.event() {
                tauri::webview::PageLoadEvent::Started => "开始加载",
                tauri::webview::PageLoadEvent::Finished => "加载完成",
            };
            crate::diag::log(format!(
                "[Desktop] 设备小窗页面{event}: label={} url={}",
                window.label(),
                payload.url()
            ));
            if matches!(payload.event(), tauri::webview::PageLoadEvent::Started) {
                if let Err(error) = crate::player_windows::restore_after_navigation(window.app_handle(), window.label()) {
                    crate::diag::log(format!("[Desktop] 页面加载后恢复小窗失败: {error}"));
                }
            }
        })
        .initialization_script(&token_script)
        .initialization_script(CLICK_SCRIPT);

    // 透明无边框窗口只在 macOS 上保留；Windows 的透明窗口依赖 DWM 模糊区域，
    // 部分环境下 WebView 内容不绘制，表现为“点了打开小窗但什么也看不到”。
    #[cfg(target_os = "macos")]
    let builder = builder.transparent(true);
    #[cfg(not(target_os = "macos"))]
    let builder = builder
        .transparent(false)
        .background_color(tauri::window::Color(0, 0, 0, 255));

    // Track before loading so the first frontend snapshot already has this window.
    if let Err(error) = crate::player_windows::track(app, &label, &serial, borderless) {
        crate::diag::log(format!("[Desktop] 登记设备小窗失败: label={label}: {error}"));
        return Err(error);
    }
    let window = match builder.build() {
        Ok(window) => window,
        Err(error) => {
            crate::player_windows::forget(app, &label);
            crate::diag::log(format!(
                "[Desktop] 创建设备小窗失败: serial={serial} label={label}: {error}"
            ));
            return Err(error.to_string());
        }
    };
    // 从 IPC 回调创建的窗口在 Windows 上可能停在主窗口之后，显式显示并聚焦。
    if let Err(error) = window.show().and_then(|_| window.set_focus()) {
        crate::diag::log(format!("[Desktop] 显示设备小窗失败: label={label}: {error}"));
    }
    crate::diag::log(format!(
        "[Desktop] 已创建设备小窗: serial={serial} label={label} url={dest} visible={}",
        window.is_visible().unwrap_or(false)
    ));
    Ok(())
}

#[tauri::command]
pub fn set_window_always_on_top(window: tauri::WebviewWindow, always_on_top: bool) -> Result<(), String> {
    window.set_always_on_top(always_on_top).map_err(|e| e.to_string())
}

#[tauri::command]
pub fn is_window_always_on_top(window: tauri::WebviewWindow) -> Result<bool, String> {
    window.is_always_on_top().map_err(|e| e.to_string())
}

#[tauri::command]
pub fn open_main_device_detail(app: AppHandle, serial: String) -> Result<(), String> {
    if let Some(main_window) = app.get_webview_window("main") {
        let state = app.state::<State>();
        let target_url = format!("{}/player?serial={}", state.url, serial);
        let _ = main_window.show();
        let _ = main_window.unminimize();
        let _ = main_window.set_focus();
        let _ = main_window.emit("navigate-to-player", &serial);
        let script = format!(
            "if (window.__NAVIGATE_TO_PLAYER__) {{ window.__NAVIGATE_TO_PLAYER__({}); }} else {{ window.location.href = {}; }}",
            serde_json::to_string(&serial).unwrap_or_default(),
            serde_json::to_string(&target_url).unwrap_or_default()
        );
        let _ = main_window.eval(&script);
        Ok(())
    } else {
        Err("主窗口不存在".into())
    }
}

pub const CLICK_SCRIPT: &str = include_str!("links.js");

#[tauri::command]
pub fn fit_player_window(window: tauri::WebviewWindow, video_width: f64, video_height: f64, extra_height: Option<f64>) -> Result<(), String> {
    if !window.label().starts_with("player-") { return Err("仅设备小窗支持画面适配".into()); }
    if !video_width.is_finite() || !video_height.is_finite() || video_width < 1.0 || video_height < 1.0 { return Err("视频尺寸无效".into()); }
    let extra_height = extra_height.unwrap_or(0.0).clamp(0.0, 200.0);
    let scale = window.scale_factor().map_err(|e| e.to_string())?;
    let current = window.inner_size().map_err(|e| e.to_string())?.to_logical::<f64>(scale);
    let ratio = video_width / video_height;
    let mut width = current.width;
    if let Some(monitor) = window.current_monitor().map_err(|e| e.to_string())? {
        let screen = monitor.size().to_logical::<f64>(monitor.scale_factor());
        width = width.min(screen.width - 40.0).min((screen.height - 100.0 - extra_height) * ratio);
    }
    let height = (width / ratio).round() + extra_height;
    if (current.width - width).abs() >= 1.0 || (current.height - height).abs() >= 1.0 {
        window.set_size(tauri::LogicalSize::new(width, height)).map_err(|e| e.to_string())?;
        eprintln!("[Desktop] 小窗 {} 适配视频 {}x{} -> {:.0}x{:.0}", window.label(), video_width, video_height, width, height);
    }
    Ok(())
}

/// One approval window for the entire application. Existing windows are not
/// focused again by polling; only a new request restores a dismissed window.
#[tauri::command]
pub fn open_approval_window(app: AppHandle, approval_id: String) -> Result<(), String> {
    if !approval_id.starts_with("ap_") || !approval_id[3..].chars().all(|c| c.is_ascii_hexdigit()) || approval_id.len() > 64 || approval_id.len() < 4 {
        return Err("审核请求 ID 无效".into());
    }
    let label = format!("safety-approval-{approval_id}");
    if let Some(window) = app.get_webview_window(&label) {
        window.set_always_on_top(true).map_err(|e| e.to_string())?;
        window.show().map_err(|e| e.to_string())?;
        window.unminimize().map_err(|e| e.to_string())?;
        return Ok(());
    }
    let state = app.state::<State>();
    let mut dest = tauri::Url::parse(&format!("{}/approval", state.url)).map_err(|e| e.to_string())?;
    dest.query_pairs_mut().append_pair("approval", &approval_id);
    let offset = (app.webview_windows().keys().filter(|key| key.starts_with("safety-approval-")).count() % 6) as i32 * 32;
    let token_script = {
        let guard = state.backend.lock().map_err(|e| e.to_string())?;
        match guard.as_ref() {
            Some(backend) => format!("Object.defineProperty(window,'__FARM_DESKTOP_TOKEN__',{{value:{}}});", serde_json::to_string(&backend.ready.token).map_err(|e| e.to_string())?),
            None => String::new(),
        }
    };
    let window = tauri::WebviewWindowBuilder::new(&app, &label, tauri::WebviewUrl::External(dest))
        .title("安全审核")
        .inner_size(420.0, 200.0)
        .min_inner_size(340.0, 200.0)
        .always_on_top(true)
        .center()
        .initialization_script(&token_script)
        .build()
        .map_err(|error| {
            eprintln!("[Desktop] 创建安全审核小窗失败: {error}");
            error.to_string()
        })?;
    if offset > 0 {
        let position = window.outer_position().map_err(|e| e.to_string())?;
        window.set_position(tauri::PhysicalPosition::new(position.x + offset, position.y + offset)).map_err(|e| e.to_string())?;
    }
    eprintln!("[Desktop] 安全审核小窗已创建 always_on_top={}", window.is_always_on_top().map_err(|e| e.to_string())?);
    Ok(())
}
