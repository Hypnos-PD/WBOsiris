/* 探针：在 Proton 里做一次 DNS 解析和一次 TCP 连接。
 *
 * 用途是验证 LD_PRELOAD/WINE_LD_PRELOAD 到底能不能进到 Wine 进程里——如果
 * netlog.so 没被加载，netlog.log 里就不会有这两行。结果写文件而不是 stdout，
 * 因为 Proton 会吞掉控制台输出。
 */
#include <winsock2.h>
#include <ws2tcpip.h>
#include <stdio.h>

int main(void) {
    WSADATA data;
    WSAStartup(MAKEWORD(2, 2), &data);
    FILE *report = fopen("netprobe-result.txt", "wb");

    struct addrinfo hints = {0}, *result = NULL;
    hints.ai_family = AF_INET;
    hints.ai_socktype = SOCK_STREAM;
    int rc = getaddrinfo("example.com", "80", &hints, &result);
    fprintf(report, "getaddrinfo rc=%d\n", rc);
    if (rc == 0) {
        SOCKET socket_handle = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
        int connected = connect(socket_handle, result->ai_addr, (int)result->ai_addrlen);
        fprintf(report, "connect rc=%d err=%d\n", connected, WSAGetLastError());
        closesocket(socket_handle);
        freeaddrinfo(result);
    }
    fclose(report);
    WSACleanup();
    return 0;
}
