/* 在真实 Windows 环境里验证代理 DLL。
 *
 * 它不碰游戏，只验证三件事：代理能不能加载、转发能不能解析到 yaha_orig、以及
 * yaha_request_set_uri 有没有真的把 URI 改写掉并记进日志。
 *
 * 用法：放在同时含代理和 yaha_orig.dll 的目录里，用 Proton/Wine 运行。
 */
#include <windows.h>
#include <stdio.h>
#include <string.h>
#include <stdarg.h>

/* Proton 会把控制台输出吞掉，所以同时写一份到文件——那才是可信的结果。 */
static FILE *report;

static void say(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vprintf(format, args);
    va_end(args);
    if (report) {
        va_start(args, format);
        vfprintf(report, format, args);
        va_end(args);
        fflush(report);
    }
}

/* 与上游 interop::StringBuffer 同布局。 */
typedef struct {
    const unsigned char *ptr;
    int length;
} StringBuffer;

static const char *URI = "https://api.example.com/Version/info?x=1";

static void *symbol(HMODULE module, const char *name) {
    void *pointer = (void *)GetProcAddress(module, name);
    say("  %-52s %s\n", name, pointer ? "解析成功" : "解析失败");
    return pointer;
}

/* 真游戏是从 ShadowverseWB_Data\Plugins\x86_64\ 加载这个插件的（Unity 把插件目录
 * 加进了 DLL 搜索路径），不是从 exe 同目录。夹具必须照做，否则"代理到底盖在哪"
 * 这件事就没被真正验证。找不到插件目录时退回 exe 同目录，方便小范围试验。 */
static HMODULE load_shim(char *chosen, size_t size) {
    char self[MAX_PATH];
    DWORD length = GetModuleFileNameA(NULL, self, sizeof(self));
    if (length == 0 || length >= sizeof(self)) return NULL;
    char *slash = strrchr(self, '\\');
    if (!slash) return NULL;
    *slash = '\0';
    const char *name = "Cysharp.Net.Http.YetAnotherHttpHandler.Native.dll";
    snprintf(chosen, size, "%s\\ShadowverseWB_Data\\Plugins\\x86_64\\%s", self, name);
    HMODULE module = LoadLibraryA(chosen);
    if (module) return module;
    snprintf(chosen, size, "%s\\%s", self, name);
    return LoadLibraryA(chosen);
}

// 配置紧挨着代理 DLL，日志路径写在里面。夹具照它的指示去读，这样"日志在哪"永远
// 和代理自己的判断一致，不会各说各话。
/* 找到代理的配置，再从里面读出日志路径。
 *
 * 配置来源和代理自己的判断保持一致：先看 YAHA_SHIM_CONFIG（launcher 用的方式），
 * 再看模块旁边的 yaha-shim.conf。两边都按 Z:\ 前缀翻回宿主机路径。 */
static void unix_from_windows(char *out, size_t size, const char *value) {
    if (strncmp(value, "Z:\\", 3) == 0) {
        snprintf(out, size, "/%s", value + 3);
        for (char *p = out; *p; p++) if (*p == '\\') *p = '/';
        return;
    }
    snprintf(out, size, "%s", value);
}

static void log_path_from_config(char *out, size_t size) {
    char config[MAX_PATH] = {0};
    const char *from_env = getenv("YAHA_SHIM_CONFIG");
    if (from_env && *from_env) {
        unix_from_windows(config, sizeof(config), from_env);
    } else {
        char self[MAX_PATH];
        if (GetModuleFileNameA(NULL, self, sizeof(self)) == 0) return;
        char *slash = strrchr(self, '\\');
        if (!slash) return;
        *slash = '\0';
        snprintf(config, sizeof(config), "%s\\ShadowverseWB_Data\\Plugins\\x86_64\\yaha-shim.conf", self);
    }
    FILE *file = fopen(config, "rb");
    if (!file) return;
    char line[1024];
    while (fgets(line, sizeof(line), file)) {
        char *trimmed = line;
        while (*trimmed == ' ' || *trimmed == '\t') trimmed++;
        if (strncmp(trimmed, "log", 3) != 0) continue;
        char *equals = strchr(trimmed, '=');
        if (!equals) continue;
        char *value = equals + 1;
        while (*value == ' ' || *value == '\t') value++;
        size_t length = strlen(value);
        while (length > 0 && (value[length - 1] == '\n' || value[length - 1] == '\r')) {
            value[--length] = '\0';
        }
        unix_from_windows(out, size, value);
        fclose(file);
        return;
    }
    fclose(file);
}

static int dump_log(const char *path) {
    FILE *file = fopen(path, "rb");
    if (!file) {
        say("  （读不到 %s）\n", path);
        return 1;
    }
    char line[1024];
    while (fgets(line, sizeof(line), file)) {
        say("  | %s", line);
    }
    fclose(file);
    return 0;
}

int main(void) {
    int failures = 0;
    report = fopen("yahatest-result.txt", "wb");
    say("== 加载代理\n");
    char chosen[MAX_PATH];
    HMODULE shim = load_shim(chosen, sizeof(chosen));
    say("  路径 %s\n", chosen);
    if (!shim) {
        say("  加载失败，GetLastError=%lu\n", (unsigned long)GetLastError());
        return 1;
    }
    say("  已加载 %p\n", (void *)shim);

    say("== 导出符号\n");
    /* 这一项是转发：能解析就说明 yaha_orig.dll 被加载且目标存在。 */
    void *get_last_error = symbol(shim, "yaha_get_last_error");
    void *init_runtime = symbol(shim, "yaha_init_runtime");
    void *init_context = symbol(shim, "yaha_init_context");
    void *build_client = symbol(shim, "yaha_build_client");
    void *request_new = symbol(shim, "yaha_request_new");
    void *request_set_uri = symbol(shim, "yaha_request_set_uri");
    void *request_destroy = symbol(shim, "yaha_request_destroy");
    void *dispose_context = symbol(shim, "yaha_dispose_context");
    void *dispose_runtime = symbol(shim, "yaha_dispose_runtime");
    if (!get_last_error || !init_runtime || !init_context || !build_client ||
        !request_new || !request_set_uri || !request_destroy || !dispose_context || !dispose_runtime) {
        say("  有符号没解析出来，后面的步骤没有意义\n");
        return 1;
    }

    say("== 走一遍初始化 → 建请求 → 设置 URI\n");
    void *runtime = ((void *(*)(int))init_runtime)(2);
    say("  init_runtime      = %p\n", runtime);
    if (!runtime) return 1;
    void *context = ((void *(*)(void *, void *, void *, void *))init_context)(runtime, NULL, NULL, NULL);
    say("  init_context      = %p\n", context);
    if (!context) return 1;
    ((void (*)(void *))build_client)(context);
    say("  build_client      完成\n");
    void *request = ((void *(*)(void *, int))request_new)(context, 1);
    say("  request_new       = %p\n", request);
    if (!request) return 1;

    StringBuffer value = {(const unsigned char *)URI, (int)strlen(URI)};
    int ok = ((int (*)(void *, void *, StringBuffer *))request_set_uri)(context, request, &value);
    say("  set_uri(\"%s\") = %s\n", URI, ok ? "成功" : "失败");
    if (!ok) failures++;

    ((int (*)(void *, void *))request_destroy)(context, request);
    ((void (*)(void *))dispose_context)(context);
    ((void (*)(void *))dispose_runtime)(runtime);

    say("== 代理日志\n");
    char log_path[MAX_PATH] = "yaha-shim.log";
    log_path_from_config(log_path, sizeof(log_path));
    say("  （日志 %s）\n", log_path);
    failures += dump_log(log_path);

    /* 游戏数据目录必须真的接上了。Unity 在启动时会检查它，找不到就报
     * "There should be 'ShadowverseWB_Data' folder next to the executable"。
     * 这里用一个不在 Plugins 下的文件来验证——只查插件目录是查不出问题的，
     * 因为插件目录是单独挂的，缺了别的也能加载成功。 */
    say("== 数据目录\n");
    char self[MAX_PATH];
    if (GetModuleFileNameA(NULL, self, sizeof(self)) != 0) {
        char *slash = strrchr(self, '\\');
        if (slash) {
            *slash = '\0';
            char marker[MAX_PATH];
            snprintf(marker, sizeof(marker), "%s\\ShadowverseWB_Data\\marker.txt", self);
            FILE *file = fopen(marker, "rb");
            if (!file) {
                say("  读不到 %s —— 数据目录没有接上\n", marker);
                failures++;
            } else {
                char line[64] = {0};
                fgets(line, sizeof(line), file);
                fclose(file);
                say("  读到 %s\n", line);
                if (strncmp(line, "real-data", 9) != 0) {
                    say("  内容不是真实数据目录里的那一份\n");
                    failures++;
                }
            }
        }
    }

    say("== 结论：%s\n", failures ? "有失败项" : "全部通过");
    if (report) fclose(report);
    return failures ? 1 : 0;
}
