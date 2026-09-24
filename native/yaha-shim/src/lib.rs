//! 代理原版客户端的 HTTP 传输，把请求改写到本地实现。
//!
//! 客户端的所有服务端调用都经过 `Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll`
//! （上游 Rust 写的 cdylib）。我们把这个文件换成自己，把原件改名成 `yaha_orig.dll`
//! 留在旁边：46 个导出里 45 个由 proxy.def 直接转发回去，只有
//! `yaha_request_set_uri` 被我们接管——在这里把 URI 改写到本地，再交给原件。
//!
//! 这样做的好处是 TLS、HTTP/2、连接池、流式收发全部还是上游实现，我们只改一个
//! 字符串；同时不需要知道客户端原本连的是哪个域名，第一次运行的日志会告诉我们。

#![allow(non_snake_case)]
// 非 Windows 上这个 crate 只为跑纯函数的单元测试而编译，Windows 专用的那些项
// 在这里没有调用者。
#![cfg_attr(not(windows), allow(dead_code))]

use std::fs::OpenOptions;
use std::io::Write;
use std::path::PathBuf;
use std::sync::OnceLock;

// 与 Windows 有关的部分全部关在 cfg 后面：改写逻辑本身是纯函数，可以在别的平台上
// 直接跑测试，不必为了验证一个字符串变换去搭一套 Windows 环境。
#[cfg(windows)]
use core::ffi::{c_char, c_void};
#[cfg(windows)]
use std::os::windows::ffi::OsStringExt;

/// 与上游 `interop::StringBuffer` 同布局。它是导出函数的参数类型，所以必须是 pub
/// 的——否则 `yaha_request_set_uri` 的签名会引用一个私有类型。
#[repr(C)]
pub struct StringBuffer {
    ptr: *const u8,
    length: i32,
}

/// 改名后的原版 DLL。它必须和本代理在同一目录（游戏目录的搜索路径里）。
const ORIGINAL_DLL: &str = "yaha_orig.dll";
const DEFAULT_LOCAL: &str = "http://127.0.0.1:50172";
const CONFIG_FILE: &str = "yaha-shim.conf";

#[cfg(windows)]
unsafe extern "system" {
    fn LoadLibraryW(name: *const u16) -> *mut c_void;
    fn GetProcAddress(module: *mut c_void, name: *const c_char) -> *mut c_void;
    fn GetModuleHandleExW(flags: u32, address: *const c_void, module: *mut *mut c_void) -> i32;
    fn GetModuleFileNameW(module: *mut c_void, buffer: *mut u16, size: u32) -> u32;
}

#[cfg(windows)]
const GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT: u32 = 0x2;
#[cfg(windows)]
const GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS: u32 = 0x4;

struct Config {
    /// 改写目标，允许带路径前缀，例如 `http://127.0.0.1:50172/portal`。
    local: String,
    /// 不改写的主机名（不含端口），用于放行必须走真实网络的调用。
    passthrough: Vec<String>,
    /// 要在进程内存里找的那 52 字节常量（十六进制）。
    ///
    /// 客户端谈判用的"对端公钥"写死在这个常量的前 32 字节里，而这份内容来自
    /// 内嵌在 GameAssembly.dll 里的加密元数据，磁盘上搜不到明文——只有运行时内存
    /// 里才有。所以只能在这里改。
    pattern: Option<Vec<u8>>,
    /// 改写进去的 32 字节（十六进制，按客户端读取的字节序排好）。
    peerkey: Option<Vec<u8>>,
    /// 只为了做对照实验：置为 true 时完全不碰内存。
    no_patch: bool,
    /// 是否在按元数据偏移改写之外，再扫全内存改掉所有 32 字节副本。
    ///
    /// 默认关：那是一次"广撒网"，会碰到只是碰巧相同的无关数据，实测能把客户端改崩。
    /// 留着这个开关是为了做对照实验（也为了在新版本上先摸清副本分布）。
    copy_scan: bool,
    /// 原件 DLL 的完整路径。
    ///
    /// 默认是在自己旁边找 `yaha_orig.dll`，但挂载命名空间那套做法只覆盖了插件本名，
    /// 没有同时暴露出改名后的原件——所以 launcher 会显式告诉我们去哪里加载。
    original: Option<String>,
    log: Option<PathBuf>,
}

impl Config {
    fn load(directory: Option<PathBuf>) -> Config {
        let mut config = Config {
            local: DEFAULT_LOCAL.to_string(),
            passthrough: Vec::new(),
            pattern: None,
            peerkey: None,
            no_patch: false,
            copy_scan: false,
            original: None,
            // 默认不写日志：模块目录就是游戏的插件目录，往那里写文件等于改动
            // Steam 的安装目录。只有明确配置了 log 才落盘。
            log: None,
        };
        // 配置来源按优先级：环境变量（launcher 现场生成，不碰游戏目录）→
        // 模块旁边的 yaha-shim.conf（手工安装时的用法）。
        let path = std::env::var_os("YAHA_SHIM_CONFIG")
            .map(PathBuf::from)
            .or_else(|| directory.map(|dir| dir.join(CONFIG_FILE)));
        let Some(path) = path else {
            return config;
        };
        let Ok(text) = std::fs::read_to_string(&path) else {
            return config;
        };
        let base = path.parent().map(|parent| parent.to_path_buf());
        for line in text.lines() {
            let line = line.trim();
            if line.is_empty() || line.starts_with('#') {
                continue;
            }
            let Some((key, value)) = line.split_once('=') else {
                continue;
            };
            let value = value.trim();
            match key.trim().to_ascii_lowercase().as_str() {
                "local" if !value.is_empty() => config.local = value.to_string(),
                "passthrough" if !value.is_empty() => {
                    config.passthrough.push(value.to_ascii_lowercase())
                }
                "original" if !value.is_empty() => config.original = Some(value.to_string()),
                "pattern" if !value.is_empty() => config.pattern = decode_hex(value),
                "peerkey" if !value.is_empty() => config.peerkey = decode_hex(value),
                "no-patch" => config.no_patch = value.eq_ignore_ascii_case("true") || value == "1",
                "copy-scan" => config.copy_scan = value.eq_ignore_ascii_case("true") || value == "1",
                "log" if !value.is_empty() => {
                    let candidate = PathBuf::from(value);
                    config.log = Some(if candidate.is_absolute() {
                        candidate
                    } else if let Some(base) = &base {
                        base.join(candidate)
                    } else {
                        candidate
                    });
                }
                _ => {}
            }
        }
        config.record(&format!("config: local={} 放行 {} 个主机", config.local, config.passthrough.len()));
        config
    }

    /// 记录一条诊断信息。观测永远不能让传输停下来，所以这里吞掉所有写入错误。
    fn record(&self, message: &str) {
        let Some(path) = &self.log else { return };
        let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) else {
            return;
        };
        let seconds = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|value| value.as_secs())
            .unwrap_or(0);
        let _ = writeln!(file, "{seconds} {message}");
    }
}

#[cfg(windows)]
fn config() -> &'static Config {
    static CONFIG: OnceLock<Config> = OnceLock::new();
    CONFIG.get_or_init(|| Config::load(module_directory()))
}

/// 本 DLL 所在目录，用于找配置和写日志。拿不到时返回 None，功能退化成默认值。
#[cfg(windows)]
fn own_directory() -> Option<PathBuf> {
    let mut module: *mut c_void = std::ptr::null_mut();
    // 用本模块里的一个函数地址反查模块句柄；不用 DllMain 里的 hInstance，因为
    // Rust 的 cdylib 没有稳定的入口。
    let ok = unsafe {
        GetModuleHandleExW(
            GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS | GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
            own_directory as *const c_void,
            &mut module,
        )
    };
    if ok == 0 || module.is_null() {
        return None;
    }
    let mut buffer = vec![0u16; 32_768];
    let length = unsafe { GetModuleFileNameW(module, buffer.as_mut_ptr(), buffer.len() as u32) };
    if length == 0 {
        return None;
    }
    buffer.truncate(length as usize);
    let path = PathBuf::from(std::ffi::OsString::from_wide(&buffer));
    path.parent().map(|parent| parent.to_path_buf())
}

#[cfg(windows)]
fn module_directory() -> Option<PathBuf> {
    own_directory()
}

#[cfg(windows)]
type SetUri = unsafe extern "C" fn(*const c_void, *const c_void, *const StringBuffer) -> bool;

/// 原件的 `yaha_request_set_uri`。加载失败就返回 None，调用方按"不支持"处理。
#[cfg(windows)]
fn original() -> Option<SetUri> {
    static ORIGINAL: OnceLock<Option<SetUri>> = OnceLock::new();
    *ORIGINAL.get_or_init(|| {
        let pointer = original_proc(c"yaha_request_set_uri")?;
        Some(unsafe { std::mem::transmute::<*mut c_void, SetUri>(pointer) })
    })
}

/// 加载原件模块并取一个导出。加载失败只记一次日志。
#[cfg(windows)]
fn original_proc(name: &core::ffi::CStr) -> Option<*mut c_void> {
    // 存成 usize：裸指针不满足 Sync，而模块句柄本来就是个不透明的整数。
    static MODULE: OnceLock<Option<usize>> = OnceLock::new();
    let module = (*MODULE.get_or_init(|| {
        // 先看配置里有没有指定原件的位置，没有才退回"自己旁边那个名字"。
        let configured = config().original.clone();
        let path = wide(configured.as_deref().unwrap_or(ORIGINAL_DLL));
        let module = unsafe { LoadLibraryW(path.as_ptr()) };
        if module.is_null() {
            config().record(&format!(
                "original: 加载 {} 失败（错误码 {}）",
                configured.as_deref().unwrap_or(ORIGINAL_DLL),
                std::io::Error::last_os_error()
            ));
            return None;
        }
        Some(module as usize)
    }))? as *mut c_void;
    let pointer = unsafe { GetProcAddress(module, name.as_ptr()) };
    if pointer.is_null() {
        config().record(&format!("original: 找不到 {}", name.to_string_lossy()));
        return None;
    }
    Some(pointer)
}

#[cfg(windows)]
fn wide(value: &str) -> Vec<u16> {
    value.encode_utf16().chain(std::iter::once(0)).collect()
}

fn decode_hex(value: &str) -> Option<Vec<u8>> {
    let value = value.trim();
    if value.len() % 2 != 0 {
        return None;
    }
    (0..value.len() / 2)
        .map(|index| u8::from_str_radix(&value[index * 2..index * 2 + 2], 16).ok())
        .collect()
}

#[cfg(windows)]
#[repr(C)]
struct MemoryBasicInformation {
    base_address: *mut c_void,
    allocation_base: *mut c_void,
    allocation_protect: u32,
    _padding: u32,
    region_size: usize,
    state: u32,
    protect: u32,
    kind: u32,
    _padding2: u32,
}

#[cfg(windows)]
unsafe extern "system" {
    fn VirtualQuery(address: *const c_void, buffer: *mut MemoryBasicInformation, length: usize) -> usize;
}

#[cfg(windows)]
unsafe extern "system" {
    // 元数据那一块可能映射成只读：写入前临时放开，写完还原。
    fn VirtualProtect(address: *const c_void, length: usize, new_protect: u32, old_protect: *mut u32) -> i32;
}

#[cfg(windows)]
const MEM_COMMIT: u32 = 0x1000;
#[cfg(windows)]
const PAGE_READWRITE: u32 = 0x04;
#[cfg(windows)]
const PAGE_EXECUTE_READWRITE: u32 = 0x40;

/// 在进程内存里找到那 52 字节常量，并把前 32 字节换成我们的公钥。
///
/// 为什么必须这么做：客户端用它跟服务器做 X25519，而那个"对端公钥"是写死的
/// 常量——对应的私钥在官方服务器上，我们不可能有。把常量换成我们自己的公钥之后，
/// 客户端就会把请求加密成只有我们能解开的形式。
///
/// 常量来自内嵌的加密元数据，磁盘上没有明文，只有运行时内存里才有；它所在的内存
/// 是可写的，所以直接改即可。
///
/// 返回值是这次扫描改掉了多少处；0 表示现在内存里没有那份原文（可能还没出现，
/// 也可能已经全改完了）。返回值而不是 bool 是必须的：实测第一遍扫描改到的 7 处
/// 里有"临时副本"，元数据解开之后**又出现**了原文，只用 bool 会在第一次命中后
/// 就收工，客户端照样拿原公钥加密。
#[cfg(windows)]
fn patch_peer_key() -> usize {
    let config = config();
    let (Some(pattern), Some(peer)) = (config.pattern.as_ref(), config.peerkey.as_ref()) else {
        return 0;
    };
    if pattern.len() < peer.len() || peer.len() != 32 {
        config.record("patch: pattern/peerkey 配置不合法，跳过");
        return 0;
    }
    let mut patched = 0;
    let mut address = 0x10000usize;
    let mut info = MemoryBasicInformation {
        base_address: std::ptr::null_mut(),
        allocation_base: std::ptr::null_mut(),
        allocation_protect: 0,
        _padding: 0,
        region_size: 0,
        state: 0,
        protect: 0,
        kind: 0,
        _padding2: 0,
    };
    while address < 0x7fff_ffff_0000 {
        let queried = unsafe {
            VirtualQuery(address as *const c_void, &mut info, std::mem::size_of::<MemoryBasicInformation>())
        };
        if queried == 0 {
            break;
        }
        let base = info.base_address as usize;
        let size = info.region_size;
        if info.state == MEM_COMMIT
            && (info.protect == PAGE_READWRITE || info.protect == PAGE_EXECUTE_READWRITE)
            && size > pattern.len()
            && size < 512 << 20
        {
            // VirtualQuery 已经确认这段是已提交且可读写，扫描它是安全的。
            let region = unsafe { std::slice::from_raw_parts_mut(info.base_address as *mut u8, size) };
            let mut offset = 0;
            while offset + pattern.len() <= region.len() {
                let Some(found) = region[offset..]
                    .windows(pattern.len())
                    .position(|window| window == pattern.as_slice())
                else {
                    break;
                };
                let at = offset + found;
                region[at..at + peer.len()].copy_from_slice(peer);
                patched += 1;
                offset = at + pattern.len();
            }
        }
        address = base + size;
    }
    if patched > 0 {
        // 一次扫描要遍历整个地址空间，很贵；所以只在真的改到时写一行汇总。
        config.record(&format!("patch: 改写完成，共 {patched} 处"));
    }
    patched
}

#[cfg(not(windows))]
fn patch_peer_key() -> usize {
    0
}

/// 已经改写过就置位，避免重复扫描。
#[cfg(windows)]
static PATCHED: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

/// 常量在元数据里的偏移（1.9.1.18238 实测；换版本要重新提取）。
///
/// 为什么要按偏移改，而不是"在内存里找那 52 字节"：实测（1.9.5 客户端）按 32 字节
/// 全内存搜索能改到若干份副本，唯独改不到客户端**真正用来协商密钥的那一份**——它在
/// 内嵌元数据的字段默认值表里，客户端每次调用 CommonHeader() 都从这里重新拷一份。
/// 只改副本的表现是：broker 一直报"载荷认证失败"，而 shim 日志显示改写成功。
const METADATA_CONSTANT_OFFSET: usize = 0xFFC0C8;

/// 用来确认"这真的是元数据"的类型名，与 delta/tools/patch_common_header.py 一致。
#[cfg(windows)]
const METADATA_NEEDLES: [&[u8]; 2] = [b"Wizard2.ServerShared", b"MessagePackObjects"];

/// 判断 offset 处是不是一个合法的元数据头，是则返回它声明的总长度。
///
/// 直接照搬 delta 那边的判定：头部的 header_size 与区间表必须自洽。不对目标做这种
/// 校验就会改错地方——踩过一次，所以这里的判断宁可严一点。
#[cfg(windows)]
fn metadata_size(buffer: &[u8], offset: usize) -> Option<usize> {
    let read_u32 = |at: usize| -> Option<u32> {
        let bytes = buffer.get(at..at + 4)?;
        Some(u32::from_le_bytes([bytes[0], bytes[1], bytes[2], bytes[3]]))
    };
    if offset + 0x1000 > buffer.len() {
        return None;
    }
    let header_size = read_u32(offset + 8)? as usize;
    if !(0x20..=0x2000).contains(&header_size) || header_size % 8 != 0 {
        return None;
    }
    let count = (header_size - 8) / 8;
    if !(8..=60).contains(&count) {
        return None;
    }
    let mut previous_end = 0usize;
    let mut first = true;
    for index in 0..count {
        let start = read_u32(offset + 8 + index * 8)? as usize;
        let size = read_u32(offset + 12 + index * 8)? as usize;
        if start > 0x4000_0000 || size > 0x4000_0000 {
            return None;
        }
        if size > 0 {
            if first {
                if start != header_size {
                    return None;
                }
                first = false;
            } else if start != previous_end {
                return None;
            }
            previous_end = start + size;
        }
    }
    if first || !(0x0010_0000..=0x4000_0000).contains(&previous_end) {
        return None;
    }
    Some(previous_end)
}

/// 在元数据声明的区间里找那两个类型名，确认这真的是我们认识的那份元数据。
#[cfg(windows)]
fn metadata_has_needles(buffer: &[u8], offset: usize, total: usize) -> bool {
    let end = (offset + total).min(buffer.len());
    METADATA_NEEDLES
        .iter()
        .all(|needle| buffer[offset..end].windows(needle.len()).any(|window| window == *needle))
}

/// 按偏移改写元数据里的常量——这是客户端真正会去读的那一份。
#[cfg(windows)]
fn patch_metadata_constant(pattern: &[u8], peer: &[u8]) -> usize {
    let mut patched = 0;
    let mut address = 0x10000usize;
    let mut info = MemoryBasicInformation {
        base_address: std::ptr::null_mut(),
        allocation_base: std::ptr::null_mut(),
        allocation_protect: 0,
        _padding: 0,
        region_size: 0,
        state: 0,
        protect: 0,
        kind: 0,
        _padding2: 0,
    };
    while address < 0x7fff_ffff_0000 {
        let queried = unsafe {
            VirtualQuery(address as *const c_void, &mut info, std::mem::size_of::<MemoryBasicInformation>())
        };
        if queried == 0 {
            break;
        }
        let base = info.base_address as usize;
        let size = info.region_size;
        // 元数据是几十 MB 的匿名块；只看这个量级的区域，别去逐字节翻整个地址空间。
        if info.state == MEM_COMMIT && (1 << 20..=64 << 20).contains(&size) {
            let region = unsafe { std::slice::from_raw_parts(info.base_address as *const u8, size) };
            let mut offset = 0;
            while offset + METADATA_CONSTANT_OFFSET + pattern.len() <= region.len() {
                let Some(total) = metadata_size(region, offset) else {
                    offset += 8;
                    continue;
                };
                if metadata_has_needles(region, offset, total) {
                    let target = offset + METADATA_CONSTANT_OFFSET;
                    if region[target..target + pattern.len()] == *pattern
                        && write_protected(info.base_address, target, peer)
                    {
                        patched += 1;
                    }
                    offset += total;
                    continue;
                }
                offset += 8;
            }
        }
        address = base + size;
    }
    patched
}

/// 往可能只读的页里写 32 字节：临时放开写权限，写完立刻还原。
#[cfg(windows)]
fn write_protected(page_base: *mut c_void, offset: usize, value: &[u8]) -> bool {
    const PAGE_READWRITE: u32 = 0x04;
    let address = unsafe { (page_base as *mut u8).add(offset) };
    let mut previous = 0u32;
    let changed = unsafe {
        VirtualProtect(address as *const c_void, value.len(), PAGE_READWRITE, &mut previous)
    };
    if changed == 0 {
        config().record("patch: VirtualProtect 失败，这一处跳过");
        return false;
    }
    unsafe {
        std::ptr::copy_nonoverlapping(value.as_ptr(), address, value.len());
    }
    let mut ignored = 0u32;
    unsafe {
        VirtualProtect(address as *const c_void, value.len(), previous, &mut ignored);
    }
    true
}

/// 每一轮按元数据偏移改那一份真正的来源。
///
/// 这里**故意不再**顺带扫全内存改所有 32 字节副本。那样做过，代价是：32 字节的
/// 模式在进程里出现 5~8 次，其中有些只是碰巧相同的数据，改掉它们等于随机破坏内存
/// ——实机观察到客户端在启动十几秒后崩在 HTTP 插件里（栈回溯落在 cysharp 那个模块）。
/// 而"按偏移改元数据"这一条已经足够：客户端每次调用 CommonHeader() 都从元数据里
/// 现拷一份，改了这一份，之后所有请求用的都是我们的公钥（实测请求能被 broker 解开）。
///
/// 要重新打开全内存扫描做对照实验时，把 config 里的 copy-scan 设成 true（见
/// patch_peer_key 的调用点）。
#[cfg(windows)]
fn patch_round() -> usize {
    let config = config();
    let (Some(pattern), Some(peer)) = (config.pattern.as_ref(), config.peerkey.as_ref()) else {
        return 0;
    };
    let mut patched = patch_metadata_constant(pattern, peer);
    if config.copy_scan {
        patched += patch_peer_key();
    }
    patched
}

#[cfg(not(windows))]
fn patch_round() -> usize {
    0
}

/// 保证在返回之前公钥已经被换掉。
///
/// `yaha_init_context` 在客户端构建第一个请求包**之前**被调用（实测），所以在这里
/// 阻塞等扫描完成，第一个包就是新公钥加密的。扫一遍地址空间要两三秒，代价是启动
/// 时多等这么久——比拿到一批解不开的包划算得多。
#[cfg(windows)]
fn ensure_patched() {
    use std::sync::atomic::Ordering;
    if PATCHED.load(Ordering::SeqCst) || config().no_patch {
        return;
    }
    if !(config().pattern.is_some() && config().peerkey.is_some()) {
        return;
    }
    for _ in 0..50 {
        let patched = patch_round();
        if patched > 0 {
            PATCHED.store(true, Ordering::SeqCst);
            config().record(&format!("patch: 返回前改写 {patched} 处"));
            return;
        }
        std::thread::sleep(std::time::Duration::from_millis(100));
    }
    config().record("patch: 等不到可改写的常量，继续启动（这一批请求可能解不开）");
}

#[cfg(not(windows))]
fn ensure_patched() {}

/// 启动一个后台线程做改写：常量可能在元数据解开之后才出现，所以隔一会儿多试几次。
///
/// 这里有意**不**在第一次命中后就收工：实测（1.9.5 客户端）第一遍扫描在启动早期
/// 只改到若干份副本，元数据里那份权威的值是稍后才出现的，只扫一遍会漏掉它——表现
/// 就是 broker 一直报"载荷认证失败"，而日志里写着"改写完成"。
///
/// 轮数是有上限的：每轮都要按元数据偏移找一遍、再兜底扫一遍全内存，很贵。20 轮
/// （约 15 秒）足够覆盖"元数据晚出现"这个窗口；再往后重复改同一批副本没有意义。
#[cfg(windows)]
fn spawn_patcher() {
    std::thread::spawn(|| {
        if !(config().pattern.is_some() && config().peerkey.is_some()) {
            return;
        }
        let mut rounds = 0;
        let mut total = 0;
        while rounds < 20 {
            rounds += 1;
            let patched = patch_round();
            if patched > 0 {
                PATCHED.store(true, std::sync::atomic::Ordering::SeqCst);
                total += patched;
            }
            std::thread::sleep(std::time::Duration::from_millis(750));
        }
        config().record(&format!("patch: 监视结束（{rounds} 轮，累计改写 {total} 处）"));
    });
}

#[cfg(not(windows))]
fn spawn_patcher() {}

/// DLL 加载时就把改写线程支起来。
///
/// 扫描整个地址空间要几百毫秒到数秒，而客户端从加载插件到发出第一个请求只有几秒。
/// 挂在 `yaha_init_context` 上太晚（实测那批包已经用原公钥加了密），所以必须在
/// 加载时就启动，让扫描与游戏自己的初始化并行。
///
/// 线程里先等一小会儿再动手：DllMain 期间持有加载器锁，尽量别在锁里做事。
#[cfg(windows)]
#[no_mangle]
pub extern "system" fn DllMain(_instance: *mut c_void, reason: u32, _reserved: *mut c_void) -> i32 {
    const DLL_PROCESS_ATTACH: u32 = 1;
    if reason == DLL_PROCESS_ATTACH {
        std::thread::spawn(|| {
            // 线程里先等一小会儿再动手：DllMain 期间持有加载器锁，尽量别在锁里做事。
            std::thread::sleep(std::time::Duration::from_millis(200));
            // 改写逻辑只有一份：见 spawn_patcher。这里不放第二份副本——两处各写各的
            // 最容易的就是"一处改成持续监视、另一处还是命中一次就收工"。
            if config().no_patch {
                return;
            }
            spawn_patcher();
        });
    }
    1
}

/// 这个导出原本是直接转发给原件的。现在截下来：转发之后顺手启动改写线程。
///
/// 选它当触发点是因为它在 HTTP 处理栈建立时被调用，早于客户端构建第一个请求包。
#[cfg(windows)]
#[no_mangle]
pub unsafe extern "C" fn yaha_init_runtime(worker_threads: i32) -> *mut c_void {
    type InitRuntime = unsafe extern "C" fn(i32) -> *mut c_void;
    let Some(pointer) = original_proc(c"yaha_init_runtime") else {
        return std::ptr::null_mut();
    };
    let original: InitRuntime = std::mem::transmute(pointer);
    let result = original(worker_threads);
    config().record("hook: yaha_init_runtime 被调用");
    spawn_patcher();
    result
}

#[cfg(windows)]
#[no_mangle]
pub unsafe extern "C" fn yaha_init_context(
    runtime: *mut c_void,
    on_headers: *mut c_void,
    on_receive: *mut c_void,
    on_complete: *mut c_void,
) -> *mut c_void {
    type InitContext =
        unsafe extern "C" fn(*mut c_void, *mut c_void, *mut c_void, *mut c_void) -> *mut c_void;
    let Some(pointer) = original_proc(c"yaha_init_context") else {
        return std::ptr::null_mut();
    };
    let original: InitContext = std::mem::transmute(pointer);
    let result = original(runtime, on_headers, on_receive, on_complete);
    config().record("hook: yaha_init_context 被调用");
    // 关键：在这里等改写完成再返回。客户端紧接着就会构建第一个请求包，晚一步那批
    // 包就是用原公钥加的密，谁也解不开。
    ensure_patched();
    result
}

/// 把 `scheme://host[:port]/path` 换成 `<local>/path`。相对 URI 或不认识的写法
/// 一律返回 None，交给原件按原样处理。
fn rewrite(uri: &str, local: &str, passthrough: &[String]) -> Option<String> {
    let (_, rest) = uri.split_once("://")?;
    let (authority, path) = match rest.find('/') {
        Some(index) => (&rest[..index], &rest[index..]),
        None => (rest, ""),
    };
    if authority.is_empty() || authority.contains('@') {
        return None;
    }
    let host = match authority.rfind(':') {
        Some(index) => &authority[..index],
        None => authority,
    };
    let host = host.to_ascii_lowercase();
    if passthrough.iter().any(|allowed| *allowed == host) {
        return None;
    }
    Some(format!("{}{}", local.trim_end_matches('/'), path))
}

#[cfg(windows)]
#[no_mangle]
pub unsafe extern "C" fn yaha_request_set_uri(
    ctx: *const c_void,
    request: *const c_void,
    value: *const StringBuffer,
) -> bool {
    let Some(original) = original() else {
        return false;
    };
    if value.is_null() {
        return original(ctx, request, value);
    }
    let buffer = &*value;
    if buffer.ptr.is_null() || buffer.length <= 0 {
        return original(ctx, request, value);
    }
    let bytes = std::slice::from_raw_parts(buffer.ptr, buffer.length as usize);
    let Ok(uri) = std::str::from_utf8(bytes) else {
        return original(ctx, request, value);
    };
    let config = config();
    match rewrite(uri, &config.local, &config.passthrough) {
        Some(rewritten) => {
            // 原始 URI 是这次运行最有价值的观测数据：客户端连过哪些地址全靠它。
            config.record(&format!("rewrite {uri} -> {rewritten}"));
            let rewritten = rewritten.into_bytes();
            let replacement = StringBuffer {
                ptr: rewritten.as_ptr(),
                length: rewritten.len() as i32,
            };
            let result = original(ctx, request, &replacement);
            // rewritten 必须活到调用返回之后才能释放，所以这里不能提前 drop。
            drop(rewritten);
            result
        }
        None => original(ctx, request, value),
    }
}

#[cfg(test)]
mod tests {
    use super::{rewrite, StringBuffer};

    #[test]
    fn rewrites_absolute_uri() {
        assert_eq!(
            rewrite("https://api.example.com/Version/info?x=1", "http://127.0.0.1:50172", &[]),
            Some("http://127.0.0.1:50172/Version/info?x=1".to_string())
        );
    }

    #[test]
    fn keeps_local_path_prefix() {
        assert_eq!(
            rewrite("https://api.example.com/a/b", "http://127.0.0.1:50172/portal/", &[]),
            Some("http://127.0.0.1:50172/portal/a/b".to_string())
        );
    }

    #[test]
    fn handles_uri_without_path_and_with_port() {
        assert_eq!(
            rewrite("https://api.example.com:443", "http://127.0.0.1:50172", &[]),
            Some("http://127.0.0.1:50172".to_string())
        );
    }

    #[test]
    fn leaves_passthrough_and_relative_alone() {
        let passthrough = vec!["api.example.com".to_string()];
        assert_eq!(rewrite("https://api.example.com/x", "http://127.0.0.1:50172", &passthrough), None);
        assert_eq!(rewrite("/Version/info", "http://127.0.0.1:50172", &[]), None);
        assert_eq!(rewrite("not a uri", "http://127.0.0.1:50172", &[]), None);
    }

    #[test]
    fn buffer_layout_matches_upstream() {
        // 布局错了会把指针和长度读串，这是整个代理唯一的硬约束。
        assert_eq!(std::mem::size_of::<StringBuffer>(), 16);
        assert_eq!(std::mem::align_of::<StringBuffer>(), 8);
    }
}
