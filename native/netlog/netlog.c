/* 记录进程发起的所有 DNS 解析与 TCP 连接。
 *
 * 为什么需要它：客户端自己的类型名在那份构建里是加密的，静态找端点这条路断了。
 * 但 Proton/Wine 进程本质上是 Linux 进程，用 LD_PRELOAD 拦 getaddrinfo 和 connect
 * 就能拿到它在跟谁说话——不需要 root，不需要抓包权限，也不需要解密任何东西。
 *
 * 只记录、不改变行为：每个调用都原样转给真正的 libc。
 */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <netdb.h>
#include <netinet/in.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <stdarg.h>

static FILE *log_file(void) {
    static FILE *file;
    static int tried;
    if (!tried) {
        tried = 1;
        const char *path = getenv("NETLOG_PATH");
        if (!path) path = "/tmp/netlog.log";
        file = fopen(path, "a");
    }
    return file;
}

static void record(const char *format, ...) {
    FILE *file = log_file();
    if (!file) return;
    va_list args;
    va_start(args, format);
    fprintf(file, "%ld ", (long)time(NULL));
    vfprintf(file, format, args);
    fputc('\n', file);
    fflush(file);
    va_end(args);
}

int getaddrinfo(const char *node, const char *service, const struct addrinfo *hints,
                struct addrinfo **res) {
    static int (*real)(const char *, const char *, const struct addrinfo *, struct addrinfo **);
    if (!real) real = dlsym(RTLD_NEXT, "getaddrinfo");
    int rc = real(node, service, hints, res);
    if (node) record("getaddrinfo %s service=%s -> %s", node, service ? service : "-",
                     rc == 0 ? "ok" : gai_strerror(rc));
    return rc;
}

static const char *describe(const struct sockaddr *address, char *buffer, size_t size) {
    if (!address) return "?";
    if (address->sa_family == AF_INET) {
        const struct sockaddr_in *v4 = (const struct sockaddr_in *)address;
        char text[INET_ADDRSTRLEN] = {0};
        inet_ntop(AF_INET, &v4->sin_addr, text, sizeof(text));
        snprintf(buffer, size, "%s:%u", text, ntohs(v4->sin_port));
        return buffer;
    }
    if (address->sa_family == AF_INET6) {
        const struct sockaddr_in6 *v6 = (const struct sockaddr_in6 *)address;
        char text[INET6_ADDRSTRLEN] = {0};
        inet_ntop(AF_INET6, &v6->sin6_addr, text, sizeof(text));
        snprintf(buffer, size, "[%s]:%u", text, ntohs(v6->sin6_port));
        return buffer;
    }
    if (address->sa_family == AF_UNIX) return "unix";
    snprintf(buffer, size, "family=%d", address->sa_family);
    return buffer;
}

int connect(int socket, const struct sockaddr *address, socklen_t length) {
    static int (*real)(int, const struct sockaddr *, socklen_t);
    if (!real) real = dlsym(RTLD_NEXT, "connect");
    char buffer[128];
    int rc = real(socket, address, length);
    record("connect %s -> %s", describe(address, buffer, sizeof(buffer)), rc == 0 ? "ok" : "失败");
    return rc;
}

ssize_t sendto(int socket, const void *buffer, size_t length, int flags,
               const struct sockaddr *address, socklen_t address_length) {
    static ssize_t (*real)(int, const void *, size_t, int, const struct sockaddr *, socklen_t);
    if (!real) real = dlsym(RTLD_NEXT, "sendto");
    if (address && address->sa_family != AF_INET && address->sa_family != AF_INET6) {
        char text[128];
        record("sendto %s", describe(address, text, sizeof(text)));
    }
    return real(socket, buffer, length, flags, address, address_length);
}
