//! 桌面外壳自身诊断日志。
//!
//! Go 服务与 GUI 是同一进程内启动的两个程序：服务日志由服务进程写入
//! `desktop.log`，而 GUI 的 `eprintln!` 在 Windows 的 GUI 子系统下不可见。
//! 这里把桌面外壳的关键流程与错误单独写入 `desktop-gui.log`，便于排查
//! 只发生在某一平台（例如 Windows 小窗打不开）的问题。

use std::{
    fs::OpenOptions,
    io::Write,
    path::{Path, PathBuf},
    sync::OnceLock,
    time::{SystemTime, UNIX_EPOCH},
};

const MAX_LOG_BYTES: u64 = 2 * 1024 * 1024;
static LOG_PATH: OnceLock<PathBuf> = OnceLock::new();

/// 应用数据目录就绪后调用一次，之后 `log` 同时写入控制台与文件。
pub fn init(directory: &Path) {
    let _ = LOG_PATH.set(directory.join("desktop-gui.log"));
}

pub fn log(message: impl AsRef<str>) {
    let text = message.as_ref();
    eprintln!("{text}");
    let Some(path) = LOG_PATH.get() else { return };
    if let Ok(metadata) = path.metadata() {
        if metadata.len() > MAX_LOG_BYTES {
            let _ = std::fs::remove_file(path);
        }
    }
    if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) {
        let _ = write!(file, "{} {text}\n", timestamp());
    }
}

fn timestamp() -> String {
    let seconds = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|value| value.as_secs())
        .unwrap_or_default();
    format!("[{seconds}]")
}
