#include "ipc_client.h"

#include <iostream>
#include <cstring>
#include <cerrno>
#include <mutex>

#if defined(_WIN32)
#include <winsock2.h>
#include <ws2tcpip.h>
#pragma comment(lib, "ws2_32.lib")

class WindowsIpcClient : public IpcClient {
public:
    WindowsIpcClient() : socket_(-1) {}
    ~WindowsIpcClient() override { disconnect(); }

    bool connect(const std::string& socketPath) override {
        disconnect();
        
        WSADATA wsaData;
        if (WSAStartup(MAKEWORD(2, 2), &wsaData) != 0) {
            return false;
        }

        socket_ = ::socket(AF_INET, SOCK_STREAM, 0);
        if (socket_ == INVALID_SOCKET) {
            WSACleanup();
            return false;
        }

        // Derive port from socketPath (FNV-1a hash)
        unsigned int h = 2166136261;
        for (char c : socketPath) {
            h = (h ^ static_cast<unsigned char>(c)) * 16777619;
        }
        int port = 10000 + (h % 20000);

        struct sockaddr_in addr;
        std::memset(&addr, 0, sizeof(addr));
        addr.sin_family = AF_INET;
        addr.sin_port = htons(port);
        ::inet_pton(AF_INET, "127.0.0.1", &addr.sin_addr);

        if (::connect(socket_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            closesocket(socket_);
            socket_ = -1;
            WSACleanup();
            return false;
        }
        return true;
    }

    bool send(const std::string& message) override {
        if (socket_ == -1) return false;
        uint32_t len = static_cast<uint32_t>(message.size());
        uint32_t netLen = htonl(len);
        uint32_t totalWritten = 0;
        while (totalWritten < sizeof(netLen)) {
            int n = ::send(socket_, reinterpret_cast<const char*>(&netLen) + totalWritten, sizeof(netLen) - totalWritten, 0);
            if (n <= 0) return false;
            totalWritten += n;
        }
        totalWritten = 0;
        while (totalWritten < len) {
            int n = ::send(socket_, message.data() + totalWritten, len - totalWritten, 0);
            if (n <= 0) return false;
            totalWritten += n;
        }
        return true;
    }

    bool receive(std::string& message) override {
        if (socket_ == -1) return false;
        uint32_t netLen = 0;
        uint32_t totalRead = 0;
        while (totalRead < sizeof(netLen)) {
            int n = ::recv(socket_, reinterpret_cast<char*>(&netLen) + totalRead, sizeof(netLen) - totalRead, 0);
            if (n <= 0) return false;
            totalRead += n;
        }
        uint32_t len = ntohl(netLen);
        if (len == 0 || len > 10 * 1024 * 1024) return false;
        message.resize(len);
        totalRead = 0;
        while (totalRead < len) {
            int n = ::recv(socket_, &message[totalRead], len - totalRead, 0);
            if (n <= 0) return false;
            totalRead += n;
        }
        return true;
    }

    void disconnect() override {
        if (socket_ != -1) {
            closesocket(socket_);
            socket_ = -1;
            WSACleanup();
        }
    }

    bool isConnected() override {
        return socket_ != -1;
    }

private:
    SOCKET socket_;
};

#else
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <arpa/inet.h>

class UnixIpcClient : public IpcClient {
public:
    UnixIpcClient() : socketFd_(-1) {}
    ~UnixIpcClient() override { disconnect(); }

    bool connect(const std::string& path) override {
        disconnect();
        socketFd_ = ::socket(AF_UNIX, SOCK_STREAM, 0);
        if (socketFd_ < 0) return false;

        struct sockaddr_un addr;
        std::memset(&addr, 0, sizeof(addr));
        addr.sun_family = AF_UNIX;
        std::strncpy(addr.sun_path, path.c_str(), sizeof(addr.sun_path) - 1);

        if (::connect(socketFd_, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
            ::close(socketFd_);
            socketFd_ = -1;
            return false;
        }
        return true;
    }

    bool send(const std::string& message) override {
        if (socketFd_ < 0) return false;
        uint32_t len = static_cast<uint32_t>(message.size());
        uint32_t netLen = htonl(len);
        size_t totalWritten = 0;
        while (totalWritten < sizeof(netLen)) {
            ssize_t n = ::write(socketFd_, reinterpret_cast<const char*>(&netLen) + totalWritten, sizeof(netLen) - totalWritten);
            if (n <= 0) return false;
            totalWritten += n;
        }
        totalWritten = 0;
        while (totalWritten < len) {
            ssize_t n = ::write(socketFd_, message.data() + totalWritten, len - totalWritten);
            if (n <= 0) return false;
            totalWritten += n;
        }
        return true;
    }

    bool receive(std::string& message) override {
        if (socketFd_ < 0) return false;
        uint32_t netLen = 0;
        size_t totalRead = 0;
        while (totalRead < sizeof(netLen)) {
            ssize_t n = ::read(socketFd_, reinterpret_cast<char*>(&netLen) + totalRead, sizeof(netLen) - totalRead);
            if (n <= 0) return false;
            totalRead += n;
        }
        uint32_t len = ntohl(netLen);
        if (len == 0 || len > 10 * 1024 * 1024) return false;
        message.resize(len);
        totalRead = 0;
        while (totalRead < len) {
            ssize_t n = ::read(socketFd_, &message[totalRead], len - totalRead);
            if (n <= 0) return false;
            totalRead += n;
        }
        return true;
    }

    void disconnect() override {
        if (socketFd_ >= 0) {
            ::close(socketFd_);
            socketFd_ = -1;
        }
    }

    bool isConnected() override {
        return socketFd_ >= 0;
    }

private:
    int socketFd_;
};
#endif

std::unique_ptr<IpcClient> IpcClient::create() {
#if defined(_WIN32)
    return std::make_unique<WindowsIpcClient>();
#else
    return std::make_unique<UnixIpcClient>();
#endif
}
