#include "ipc_client.h"

#include <iostream>
#include <cstring>
#include <cerrno>
#include <mutex>

#if defined(_WIN32)
#include <windows.h>
#include <sstream>

class WindowsIpcClient : public IpcClient {
public:
    WindowsIpcClient() : pipe_(INVALID_HANDLE_VALUE) {}
    ~WindowsIpcClient() override { disconnect(); }

    bool connect(const std::string& pipeName) override {
        disconnect();
        std::wstring wpath(pipeName.begin(), pipeName.end());
        pipe_ = CreateFileW(
            wpath.c_str(),
            GENERIC_READ | GENERIC_WRITE,
            0, nullptr, OPEN_EXISTING, 0, nullptr);
        if (pipe_ == INVALID_HANDLE_VALUE) {
            std::cerr << "[WindowsIpcClient] Failed to connect to pipe\n";
            return false;
        }
        DWORD mode = PIPE_READMODE_MESSAGE;
        SetNamedPipeHandleState(pipe_, &mode, nullptr, nullptr);
        return true;
    }

    bool send(const std::string& message) override {
        if (pipe_ == INVALID_HANDLE_VALUE) return false;
        DWORD written = 0;
        uint32_t len = static_cast<uint32_t>(message.size());
        if (!WriteFile(pipe_, &len, sizeof(len), &written, nullptr)) return false;
        return WriteFile(pipe_, message.data(), len, &written, nullptr);
    }

    bool receive(std::string& message) override {
        if (pipe_ == INVALID_HANDLE_VALUE) return false;
        uint32_t len = 0;
        DWORD read = 0;
        if (!ReadFile(pipe_, &len, sizeof(len), &read, nullptr)) return false;
        message.resize(len);
        return ReadFile(pipe_, &message[0], len, &read, nullptr);
    }

    void disconnect() override {
        if (pipe_ != INVALID_HANDLE_VALUE) {
            CloseHandle(pipe_);
            pipe_ = INVALID_HANDLE_VALUE;
        }
    }

    bool isConnected() override {
        return pipe_ != INVALID_HANDLE_VALUE;
    }

private:
    HANDLE pipe_;
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
        if (::write(socketFd_, &netLen, sizeof(netLen)) < 0) return false;
        return ::write(socketFd_, message.data(), len) >= 0;
    }

    bool receive(std::string& message) override {
        if (socketFd_ < 0) return false;
        uint32_t netLen = 0;
        if (::read(socketFd_, &netLen, sizeof(netLen)) <= 0) return false;
        uint32_t len = ntohl(netLen);
        message.resize(len);
        return ::read(socketFd_, &message[0], len) > 0;
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
