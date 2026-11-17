// Feature-gated native smoke verification; absent from production binaries.
use std::{thread, time::Duration};
use tauri::{AppHandle, Manager};

pub fn start(app: &AppHandle) {
    if std::env::var_os("FARM_NATIVE_REPORT").is_none() {
        return;
    }
    if std::env::var_os("FARM_VERIFY_GROUP_CONTROL").is_some() {
        crate::verify_group_control::start(app);
        return;
    }
    if std::env::var_os("FARM_VERIFY_PLAYERS").is_some() {
        crate::verify_players::start(app);
        return;
    }
    if std::env::var_os("FARM_VERIFY_UPDATE_MENU").is_some() {
        start_update_menu(app);
        return;
    }
    let app = app.clone();
    thread::spawn(move || {
        thread::sleep(Duration::from_secs(3));
        let ui = app.clone();
        let _ = app.run_on_main_thread(move || {
            // Exercise the same handler used by native menu items.
            crate::menu::action(&ui, "home");
        });
        thread::sleep(Duration::from_secs(2));
        let ui = app.clone();
        let _ = app.run_on_main_thread(move || {
            if let Some(window) = ui.get_webview_window("main") {
                let _ = window.close();
            }
        });
        thread::sleep(Duration::from_millis(500));
        let ui = app.clone();
        let _ = app.run_on_main_thread(move || {
            if let Some(window) = ui.get_webview_window("main") {
                let hidden = match window.is_visible() { Ok(value)=>!value,Err(_)=>return };
                crate::menu::action(&ui,"show");
                let restored = match window.is_visible() { Ok(value)=>value,Err(_)=>return };
                let script=r#"
                    (async()=>{
 let updater;let updaterError='';
 try{updater=await window.__TAURI_INTERNALS__.invoke('desktop_check_update')}catch(error){updaterError=String(error)}
 fetch('/api/desktop/native-verification', {
                      method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+window.__FARM_DESKTOP_TOKEN__},
                      body:JSON.stringify({title:document.title,path:location.pathname,
                        tauri:window.isTauri===true, updater_invokable:updater?.runtime_mode==='desktop',updater_error:updaterError,
                        mounted:!!document.querySelector('#app')?.childElementCount,
                        home_path:location.pathname==='/',
                        close_hides:$HIDDEN,restore_visible:$RESTORED,
                        video_decoder:typeof VideoDecoder!=='undefined',audio_decoder:typeof AudioDecoder!=='undefined'})
                    });
 })();
                "#.replace("$HIDDEN",&hidden.to_string()).replace("$RESTORED",&restored.to_string());
                let _ = window.eval(script);
            }
        });
        let report=std::env::var("FARM_NATIVE_REPORT").unwrap();
        for _ in 0..400 {if std::path::Path::new(&report).is_file(){app.exit(0);return};thread::sleep(Duration::from_millis(100));}
        app.exit(1);
    });
}

fn start_update_menu(app: &AppHandle) {
    let app = app.clone();
    thread::spawn(move || {
        thread::sleep(Duration::from_secs(3));
        let ui = app.clone();
        let _ = app.run_on_main_thread(move || {
            crate::menu::action(&ui, "check-update");
        });
        thread::sleep(Duration::from_secs(2));
        let ui = app.clone();
        let _ = app.run_on_main_thread(move || {
            if let Some(window) = ui.get_webview_window("main") {
                let script=r#"
 fetch('/api/desktop/native-verification', {
                      method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+window.__FARM_DESKTOP_TOKEN__},
                      body:JSON.stringify({title:document.title,path:location.pathname,
                        tauri:window.isTauri===true,
                        mounted:!!document.querySelector('#app')?.childElementCount,
                        check_update_dialog:document.querySelector('[data-slot="dialog-content"]')?.textContent.includes('软件更新')===true})
                    });
                "#;
                if let Err(error) = window.eval(script) {
                    eprintln!("[Verify] 检查更新菜单验证脚本失败: {error}");
                }
            }
        });
        let report=std::env::var("FARM_NATIVE_REPORT").unwrap();
        for _ in 0..400 {if std::path::Path::new(&report).is_file(){app.exit(0);return};thread::sleep(Duration::from_millis(100));}
        app.exit(1);
    });
}
