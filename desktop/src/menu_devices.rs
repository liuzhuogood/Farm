//! Online device selection for the native View menu.
use crate::State;
use serde::Deserialize;
use std::{
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Mutex,
    },
    time::Duration,
};
use tauri::{
    menu::{MenuItem, Submenu, SubmenuBuilder},
    AppHandle, Manager,
};

const DEVICE_PREFIX: &str = "device:";

#[derive(Deserialize)]
struct Device {
    serial: String,
    state: String,
    alias: Option<String>,
    brand: Option<String>,
    model: Option<String>,
}

#[derive(Debug, PartialEq, Eq)]
struct Entry {
    serial: String,
    title: String,
}

fn device_id(serial: &str) -> String {
    // Tauri 2.12.3 re-exports muda::MenuId (String, infallible FromStr).
    // There is no character whitelist: keep the exact serial, including colons.
    format!("{DEVICE_PREFIX}{serial}")
}

pub(crate) fn device_serial(id: &str) -> Option<&str> {
    id.strip_prefix(DEVICE_PREFIX)
        .filter(|serial| !serial.is_empty())
}

fn device_title(device: &Device) -> String {
    let name = [&device.alias, &device.brand, &device.model]
        .into_iter()
        .filter_map(|name| name.as_deref())
        .map(str::trim)
        .find(|name| !name.is_empty());
    match name {
        Some(name) if name != device.serial => format!("{name} · {}", device.serial),
        _ => device.serial.clone(),
    }
}

fn online_entries(devices: Vec<Device>) -> Result<Vec<Entry>, String> {
    let mut entries = Vec::new();
    for device in devices
        .into_iter()
        .filter(|device| device.state == "device")
    {
        if device.serial.is_empty() {
            return Err("在线设备缺少 serial".into());
        }
        entries.push(Entry {
            title: device_title(&device),
            serial: device.serial,
        });
    }
    entries.sort_by(|a, b| a.serial.cmp(&b.serial));
    if entries
        .windows(2)
        .any(|pair| pair[0].serial == pair[1].serial)
    {
        return Err("在线设备 serial 重复".into());
    }
    Ok(entries)
}

fn build_devices(app: &AppHandle, entries: &[Entry]) -> tauri::Result<Submenu<tauri::Wry>> {
    let mut builder = SubmenuBuilder::with_id(app, "view-devices", "设备列表")
        .text("home", "打开设备列表")
        .separator();
    if entries.is_empty() {
        builder = builder.item(&MenuItem::with_id(
            app,
            "devices-empty",
            "无在线设备",
            false,
            None::<&str>,
        )?);
    } else {
        for entry in entries {
            // Escape the native mnemonic marker in user-provided display names.
            builder = builder.text(device_id(&entry.serial), entry.title.replace('&', "&&"));
        }
    }
    builder.build()
}

pub(crate) fn build_view(app: &AppHandle) -> tauri::Result<Submenu<tauri::Wry>> {
    SubmenuBuilder::with_id(app, "view", "视图")
        .item(&build_devices(app, &[])?)
        .separator()
        .text("show", "打开主窗口")
        .text("restore-players", "恢复所有小窗")
        .build()
}

fn replace_devices(
    app: &AppHandle,
    view: &Submenu<tauri::Wry>,
    entries: &[Entry],
) -> tauri::Result<()> {
    let devices = build_devices(app, entries)?;
    // Tauri 2.12.3 Submenu::insert/remove_at forward to muda's live native
    // submenu on the main thread. Replace only this child, keeping the macOS
    // application menu installed. No set_items API exists in this version.
    // Build first, insert before removing: construction/insert failure keeps
    // the last successful list. remove_at (rather than remove by shared ID)
    // removes the old child without discarding the replacement's bookkeeping.
    view.insert(&devices, 0)?;
    if let Err(error) = view.remove_at(1) {
        if let Err(rollback) = view.remove_at(0) {
            eprintln!("[Desktop] 回滚设备菜单失败: {rollback}");
        }
        return Err(error);
    }
    Ok(())
}

struct Refresh {
    active: Arc<AtomicBool>,
    task: Mutex<Option<tauri::async_runtime::JoinHandle<()>>>,
}

fn polling_client() -> Result<reqwest::Client, reqwest::Error> {
    // The updater enables reqwest/rustls-no-provider. Match its ring provider
    // initialization even for this HTTP-only client: reqwest builds TLS eagerly.
    // A concurrent installer may win; either way a default provider is ready.
    if rustls::crypto::CryptoProvider::get_default().is_none() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    reqwest::Client::builder()
        .timeout(Duration::from_secs(3))
        .no_proxy()
        .redirect(reqwest::redirect::Policy::none())
        .build()
}

pub(crate) fn start(app: &AppHandle, view: Submenu<tauri::Wry>) -> Result<(), reqwest::Error> {
    let client = polling_client()?;
    let url = format!("{}/api/devices", app.state::<State>().url);
    let active = Arc::new(AtomicBool::new(true));
    let running = active.clone();
    let handle = app.clone();
    let task = tauri::async_runtime::spawn(async move {
        let mut previous = Vec::new();
        let mut last_error = None;
        while running.load(Ordering::Acquire) {
            let result = async {
                // Ordinary REST endpoints do not require the desktop token.
                let devices = client
                    .get(&url)
                    .send()
                    .await
                    .map_err(|error| error.to_string())?
                    .error_for_status()
                    .map_err(|error| error.to_string())?
                    .json::<Vec<Device>>()
                    .await
                    .map_err(|error| error.to_string())?;
                let entries = online_entries(devices)?;
                if entries != previous {
                    let ui = handle.clone();
                    let view = view.clone();
                    let valid = running.clone();
                    let (send, receive) = tokio::sync::oneshot::channel();
                    // At most one request and one UI update exist at a time.
                    // Await application before polling again; no stale queue.
                    handle
                        .run_on_main_thread(move || {
                            let result = if valid.load(Ordering::Acquire) {
                                replace_devices(&ui, &view, &entries)
                                    .map(|_| entries)
                                    .map_err(|error| error.to_string())
                            } else {
                                Err("设备菜单刷新已停止".into())
                            };
                            let _ = send.send(result);
                        })
                        .map_err(|error| error.to_string())?;
                    previous = receive.await.map_err(|error| error.to_string())??;
                    eprintln!(
                        "[Desktop] 在线设备菜单已刷新: {} 台 {:?}",
                        previous.len(),
                        previous
                            .iter()
                            .map(|entry| entry.serial.as_str())
                            .collect::<Vec<_>>()
                    );
                }
                Ok::<_, String>(())
            }
            .await;
            match result {
                Ok(()) => {
                    if last_error.take().is_some() {
                        eprintln!("[Desktop] 在线设备菜单刷新已恢复");
                    }
                }
                Err(error) => {
                    // Log changing failures, not the same error every five seconds.
                    if last_error.as_ref() != Some(&error) {
                        eprintln!("[Desktop] 刷新在线设备菜单失败，保留上次结果: {error}");
                        last_error = Some(error);
                    }
                }
            }
            tokio::time::sleep(Duration::from_secs(5)).await;
        }
    });
    app.manage(Refresh {
        active,
        task: Mutex::new(Some(task)),
    });
    Ok(())
}

pub(crate) fn stop(app: &AppHandle) {
    if let Some(refresh) = app.try_state::<Refresh>() {
        refresh.active.store(false, Ordering::Release);
        if let Ok(mut task) = refresh.task.lock() {
            if let Some(task) = task.take() {
                task.abort();
                eprintln!("[Desktop] 在线设备菜单刷新已停止");
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn polling_client_builds_with_shared_updater_tls_features() {
        assert!(polling_client().is_ok());
    }

    #[test]
    fn device_ids_preserve_special_characters_without_collisions() {
        for serial in ["emulator-5554", "192.168.1.8:5555", "adb:设备/usb_1%&?"] {
            let id = device_id(serial);
            assert_eq!(device_serial(&id), Some(serial));
            assert_eq!(tauri::menu::MenuId::new(&id).as_ref(), id);
        }
        assert_ne!(device_id("a:b"), device_id("a_b"));
        for id in ["device:", "devices:abc", "home", "main-opacity-80"] {
            assert_eq!(device_serial(id), None);
        }
    }

    #[test]
    fn titles_follow_alias_brand_model_serial_priority() {
        let mut device: Device = serde_json::from_str(
            r#"{
            "serial":"emulator-5554","state":"device",
            "alias":"我的手机","brand":"Google","model":"Pixel 7"
        }"#,
        )
        .unwrap();
        assert_eq!(device_title(&device), "我的手机 · emulator-5554");
        device.alias = Some("  ".into());
        assert_eq!(device_title(&device), "Google · emulator-5554");
        device.brand = None;
        assert_eq!(device_title(&device), "Pixel 7 · emulator-5554");
        device.model = None;
        assert_eq!(device_title(&device), "emulator-5554");
        device.alias = Some(device.serial.clone());
        assert_eq!(device_title(&device), "emulator-5554");
    }

    #[test]
    fn only_online_devices_are_shown_and_order_is_stable() {
        let devices = serde_json::from_str(
            r#"[
            {"serial":"z","state":"device","product":"ignored"},
            {"serial":"offline","state":"offline"},
            {"serial":"unauthorized","state":"unauthorized"},
            {"serial":"a","state":"device"}
        ]"#,
        )
        .unwrap();
        let entries = online_entries(devices).unwrap();
        assert_eq!(
            entries
                .iter()
                .map(|entry| entry.serial.as_str())
                .collect::<Vec<_>>(),
            ["a", "z"]
        );
        assert!(online_entries(Vec::new()).unwrap().is_empty());
    }

    #[test]
    fn malformed_data_cannot_replace_last_successful_menu() {
        for json in [
            r#"[{"serial":"","state":"device"}]"#,
            r#"[{"serial":"a","state":"device"},{"serial":"a","state":"device"}]"#,
        ] {
            assert!(online_entries(serde_json::from_str(json).unwrap()).is_err());
        }
        assert!(serde_json::from_str::<Vec<Device>>(r#"[{"serial":"a"}]"#).is_err());
        assert!(serde_json::from_str::<Vec<Device>>(r#"{"devices":[]}"#).is_err());
    }
}
