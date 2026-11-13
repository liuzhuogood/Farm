use crate::State;
use tauri::{
    menu::{Menu, MenuBuilder, PredefinedMenuItem, SubmenuBuilder},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    App, AppHandle, Manager,
};
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
        "home" | "dependencies" => {
            let path = match id {
                "home" => "/",
                _ => "/dependencies",
            };
            let state = app.state::<State>();
            if let Some(window) = app.get_webview_window("main") {
                let url = format!("{}{path}", state.url)
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

pub fn install(app: &mut App) -> tauri::Result<bool> {
    let application = SubmenuBuilder::with_id(app, "application", "Farm")
        .item(&PredefinedMenuItem::about(app, Some("关于 Farm"), None)?)
        .separator()
        .text("dependencies", "依赖管理")
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
    app.on_menu_event(|app, event| action(app, event.id.as_ref()));
    crate::menu_devices::start(app.handle(), view).map_err(|error| {
        eprintln!("[Desktop] 启动在线设备菜单刷新失败: {error}");
        tauri::Error::Anyhow(error.into())
    })?;
    eprintln!("[Desktop] 原生菜单已安装，在线设备每 5 秒刷新");
    let tray_menu = MenuBuilder::new(app)
        .text("show", "打开主窗口")
        .text("restore-main", "恢复主窗口")
        .text("restore-players", "恢复所有小窗")
        .text("home", "设备列表")
        .text("dependencies", "依赖管理")
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
