// 把 proxy.def 交给链接器。这一份导出表就是全部的对外契约：原版有 46 个导出，
// 其中 45 个直接转发回改名后的原件，只有 yaha_request_set_uri 由我们自己实现。

use std::env;
use std::path::PathBuf;

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("缺少 CARGO_MANIFEST_DIR"));
    let def = manifest.join("proxy.def");
    println!("cargo:rerun-if-changed={}", def.display());

    // 这是一份 Windows 专用代理。在别的平台上（比如为了跑纯函数的单元测试而构建
    // 本机目标）不碰链接参数，否则会把 .def 塞给 Linux 的链接器。
    if env::var("CARGO_CFG_TARGET_OS").as_deref() != Ok("windows") {
        return;
    }

    // MSVC 与 gnullvm 都用 lld-link/MSVC 风格的链接器，直接吃 /DEF:；
    // windows-gnu（mingw 的 ld）要把 .def 当成一个输入文件。
    match env::var("CARGO_CFG_TARGET_ENV").as_deref() {
        Ok("msvc") => println!("cargo:rustc-link-arg=/DEF:{}", def.display()),
        Ok("gnu") => {
            // gnullvm 走 clang + COFF 链接器，同样认 /DEF:。
            let is_gnullvm = env::var("CARGO_CFG_TARGET_ABI").as_deref() == Ok("llvm");
            if is_gnullvm {
                println!("cargo:rustc-link-arg=/DEF:{}", def.display());
            } else {
                println!("cargo:rustc-link-arg={}", def.display());
            }
        }
        other => panic!("这个 crate 只构建 Windows 目标，当前 target env: {other:?}"),
    }
}
