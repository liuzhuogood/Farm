use crate::State;
use tauri::{
    menu::MenuBuilder,
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    App, AppHandle, Emitter, Manager,
};
#[cfg(target_os = "macos")]
use tauri::menu::{Menu, PredefinedMenuItem, SubmenuBuilder};
use tauri_plugin_opener::OpenerExt;

pub fn show(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        if let Err(error) = window
            .show()
            .and_then(|_| window.unminimize())
            .and_then(|_| window.set_focus())
        {
            eprintln!("[Desktop] 显示窗口失败: {error}");
        }
    }
}

pub fn action(app: &AppHandle, id: &str) {
    if let Some(serial) = crate::menu_devices::device_serial(id) {
        eprintln!("[Desktop] 菜单打开设备小窗: serial={serial}");
        if let Err(error) =
            crate::links::open_device_window_internal(app, serial.to_string(), None, None, None)
        {
            eprintln!("[Desktop] 菜单打开设备小窗失败: serial={serial}: {error}");
        }
        return;
    }
    match id {
        "show" => show(app),
        "check-update" => check_update(app),
        "adb-restart" => restart_adb(app),
        "restore-players" => crate::player_windows::restore_all(app),
        "restore-main" => {
            let _ = crate::main_window::restore(app);
        }
        "main-toggle-pin" => crate::main_window::toggle_always_on_top(app),
        "main-click-through" => crate::main_window::toggle_click_through(app),
        "main-borderless" => {
            let enabled = !crate::main_window::get_state(app).borderless;
            if let Err(error) = crate::main_window::set_borderless_internal(app, enabled) {
                eprintln!("[Desktop] 切换主窗口无边框失败: {error}");
            }
        }
        "main-opacity-100" => crate::main_window::set_opacity(app, 100),
        "main-opacity-80" => crate::main_window::set_opacity(app, 80),
        "main-opacity-60" => crate::main_window::set_opacity(app, 60),
        "main-opacity-40" => crate::main_window::set_opacity(app, 40),
        "main-opacity-10" => crate::main_window::set_opacity(app, 10),
        "main-opacity-5" => crate::main_window::set_opacity(app, 5),
        "main-opacity-2" => crate::main_window::set_opacity(app, 2),
        "main-opacity-1" => crate::main_window::set_opacity(app, 1),
        "hide" => {
            if let Some(window) = app.get_webview_window("main") {
                if let Err(error) = window.hide() {
                    eprintln!("[Desktop] 隐藏窗口失败: {error}");
                }
            }
        }
        "quit" => app.exit(0),
        "browser" => {
            let state = app.state::<State>();
            if let Err(error) = app.opener().open_url(&state.url, None::<&str>) {
                eprintln!("[Desktop] 打开浏览器失败: {error}");
            }
        }
        "home" => {
            let state = app.state::<State>();
            if let Some(window) = app.get_webview_window("main") {
                let url = format!("{}/", state.url)
                    .parse()
                    .expect("validated local URL");
                if let Err(error) = window.navigate(url) {
                    eprintln!("[Desktop] 页面导航失败: {error}");
                }
                show(app);
            }
        }
        _ => {}
    }
}

fn check_update(app: &AppHandle) {
    show(app);
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        if let Err(error) = app.emit_to("main", "check-update", ()) {
            eprintln!("[Desktop] 打开检查更新失败: {error}");
        }
    });
}

// 重启本机 ADB 服务（adb kill-server 后 adb start-server），结果只记日志。
fn restart_adb(app: &AppHandle) {
    let url = format!("{}/api/adb/restart", app.state::<State>().url);
    let client = match crate::menu_devices::polling_client() {
        Ok(client) => client,
        Err(error) => {
            eprintln!("[Desktop] 重启 ADB 服务失败: {error}");
            return;
        }
    };
    eprintln!("[Desktop] 请求重启 ADB 服务: {url}");
    tauri::async_runtime::spawn(async move {
        // 后端串行执行 kill-server 与 start-server，留足启动时间。
        let result = async {
            let response = client
                .post(&url)
                .timeout(std::time::Duration::from_secs(30))
                .send()
                .await
                .map_err(|error| error.to_string())?;
            let status = response.status();
            if status.is_success() {
                return Ok(());
            }
            let body = response.text().await.unwrap_or_default();
            Err(format!("HTTP {status}: {}", body.trim()))
        }
        .await;
        match result {
            Ok(()) => eprintln!("[Desktop] ADB 服务已重启"),
            Err(error) => eprintln!("[Desktop] 重启 ADB 服务失败: {error}"),
        }
    });
}

#[cfg(target_os = "macos")]
fn install_app_menu(app: &mut App) -> tauri::Result<()> {
    let application = SubmenuBuilder::with_id(app, "application", "Farm")
        .item(&PredefinedMenuItem::about(app, Some("关于 Farm"), None)?)
        .text("check-update", "检查更新…")
        .separator()
        .text("browser", "在浏览器中打开")
        .separator()
        .text("quit", "退出 Farm")
        .build()?;
    let edit = SubmenuBuilder::with_id(app, "edit", "编辑")
        .item(&PredefinedMenuItem::undo(app, Some("撤销"))?)
        .item(&PredefinedMenuItem::redo(app, Some("重做"))?)
        .separator()
        .item(&PredefinedMenuItem::cut(app, Some("剪切"))?)
        .item(&PredefinedMenuItem::copy(app, Some("复制"))?)
        .item(&PredefinedMenuItem::paste(app, Some("粘贴"))?)
        .item(&PredefinedMenuItem::select_all(app, Some("全选"))?)
        .build()?;
    let view = crate::menu_devices::build_view(app.handle())?;
    let effects = SubmenuBuilder::with_id(app, "main-effects", "窗口效果")
        .text("main-toggle-pin", "主窗口置顶")
        .text("main-borderless", "主窗口无边框")
        .text("main-click-through", "主窗口鼠标穿透")
        .build()?;
    let opacity = SubmenuBuilder::with_id(app, "main-opacity", "透明度")
        .text("main-opacity-100", "100%")
        .text("main-opacity-80", "80%")
        .text("main-opacity-60", "60%")
        .text("main-opacity-40", "40%")
        .text("main-opacity-10", "10%")
        .text("main-opacity-5", "5%")
        .text("main-opacity-2", "2%")
        .text("main-opacity-1", "1%")
        .build()?;
    let window_menu = SubmenuBuilder::with_id(app, "window", "窗口")
        .item(&effects)
        .item(&opacity)
        .separator()
        .text("restore-main", "恢复主窗口")
        .text("restore-players", "恢复所有小窗")
        .build()?;
    let menu = Menu::with_items(app, &[&application, &edit, &view, &window_menu])?;
    app.set_menu(menu)?;
    crate::menu_devices::start(app.handle(), view).map_err(|error| {
        eprintln!("[Desktop] 启动在线设备菜单刷新失败: {error}");
        tauri::Error::Anyhow(error.into())
    })?;
    eprintln!("[Desktop] 原生菜单已安装，在线设备每 5 秒刷新");
    Ok(())
}

#[cfg(not(target_os = "macos"))]
fn install_app_menu(_app: &mut App) -> tauri::Result<()> {
    eprintln!("[Desktop] 当前平台不安装原生菜单栏，仅保留托盘菜单");
    Ok(())
}

pub fn install(app: &mut App) -> tauri::Result<bool> {
    install_app_menu(app)?;
    // 菜单事件与托盘共用同一套动作，Windows/Linux 没有应用菜单但托盘菜单仍要生效。
    app.on_menu_event(|app, event| action(app, event.id.as_ref()));
    let tray_menu = MenuBuilder::new(app)
        .text("check-update", "检查更新…")
        .separator()
        .text("show", "打开主窗口")
        .text("restore-main", "恢复主窗口")
        .text("restore-players", "恢复所有小窗")
        .text("home", "设备列表")
        .text("adb-restart", "重启 ADB 服务")
        .separator()
        .text("browser", "在浏览器中打开")
        .separator()
        .text("quit", "退出 Farm")
        .build()?;
    let mut tray = TrayIconBuilder::with_id("main-tray")
        .tooltip("Farm")
        .menu(&tray_menu)
        .show_menu_on_left_click(true)
        .on_tray_icon_event(|tray, event| {
            if matches!(
                event,
                TrayIconEvent::Click {
                    button: MouseButton::Left,
                    button_state: MouseButtonState::Up,
                    ..
                }
            ) {
                show(tray.app_handle());
            }
        });
    #[cfg(target_os = "macos")]
    {
        let icon = tauri::image::Image::from_bytes(include_bytes!("../icons/tray-white.png"))?;
        // Keep the requested white pixels; template rendering recolors them by appearance.
        tray = tray.icon(icon).icon_as_template(false);
        eprintln!("[Desktop] macOS 托盘使用纯白透明图标");
    }
    #[cfg(not(target_os = "macos"))]
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    match tray.build(app) {
        Ok(_) => Ok(tray_host_available()),
        Err(error) => {
            eprintln!("[Desktop] 系统托盘不可用，关闭窗口将退出: {error}");
            Ok(false)
        }
    }
}

// Linux tray creation can succeed even when the desktop has no tray host.
// Only hide on close when the standard watcher explicitly reports a host.
fn tray_host_available() -> bool {
    #[cfg(not(target_os = "linux"))]
    {
        true
    }
    #[cfg(target_os = "linux")]
    {
        use std::{
            process::{Command, Stdio},
            thread,
            time::{Duration, Instant},
        };
        let mut child = match Command::new("gdbus")
            .args([
                "call",
                "--session",
                "--dest",
                "org.kde.StatusNotifierWatcher",
                "--object-path",
                "/StatusNotifierWatcher",
                "--method",
                "org.freedesktop.DBus.Properties.Get",
                "org.kde.StatusNotifierWatcher",
                "IsStatusNotifierHostRegistered",
            ])
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
        {
            Ok(child) => child,
            Err(_) => return false,
        };
        let deadline = Instant::now() + Duration::from_millis(800);
        loop {
            match child.try_wait() {
                Ok(Some(status)) => {
                    return status.success()
                        && child
                            .wait_with_output()
                            .map(|output| output.stdout == b"(<true>,)\n")
                            .unwrap_or(false)
                }
                Err(_) => break,
                Ok(None) => {}
            }
            if Instant::now() >= deadline {
                break;
            }
            thread::sleep(Duration::from_millis(20));
        }
        let _ = child.kill();
        let _ = child.wait();
        false
    }
}
