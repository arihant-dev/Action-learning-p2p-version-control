#ifdef _WIN32

#include "ipc_client.h"
#include <windows.h>
#include <iostream>
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

#endif
