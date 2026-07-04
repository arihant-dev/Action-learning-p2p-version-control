#include "ipc_client.h"

#include <iostream>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <cstring>
#include <cerrno>

namespace ipc {

// Helper: read exactly 'count' bytes from a socket fd (handle short reads)
static bool read_full(int fd, void *buf, size_t count) {
    auto *ptr = static_cast<char *>(buf);
    size_t remaining = count;
    while (remaining > 0) {
        ssize_t n = ::read(fd, ptr, remaining);
        if (n <= 0) {
            return false;
        }
        ptr += n;
        remaining -= static_cast<size_t>(n);
    }
    return true;
}

// Helper: write exactly 'count' bytes to a socket fd (handle short writes)
static bool write_full(int fd, const void *buf, size_t count) {
    auto *ptr = static_cast<const char *>(buf);
    size_t remaining = count;
    while (remaining > 0) {
        ssize_t n = ::write(fd, ptr, remaining);
        if (n <= 0) {
            return false;
        }
        ptr += n;
        remaining -= static_cast<size_t>(n);
    }
    return true;
}

// Max message size (must match Go side: 1MB)
static const uint32_t MAX_MESSAGE_SIZE = 1024 * 1024;

IpcClient::IpcClient() : socket_fd_(-1) {}

IpcClient::~IpcClient() {
    disconnect();
}

bool IpcClient::is_connected() {
    std::lock_guard<std::mutex> lock(mtx_);
    return socket_fd_ >= 0;
}

bool IpcClient::connect(const std::string &socket_path) {
    std::lock_guard<std::mutex> lock(mtx_);
    if (socket_fd_ >= 0) {
        ::close(socket_fd_);
        socket_fd_ = -1;
    }

    socket_fd_ = ::socket(AF_UNIX, SOCK_STREAM, 0);
    if (socket_fd_ < 0) {
        std::cerr << "[IpcClient] Error: Failed to create socket\n";
        return false;
    }

    struct sockaddr_un addr;
    std::memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    std::strncpy(addr.sun_path, socket_path.c_str(), sizeof(addr.sun_path) - 1);

    if (::connect(socket_fd_, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        std::cerr << "[IpcClient] Error: Connection to " << socket_path << " failed\n";
        ::close(socket_fd_);
        socket_fd_ = -1;
        return false;
    }

    std::cout << "[IpcClient] Connected to IPC socket at " << socket_path << "\n";
    return true;
}

void IpcClient::disconnect() {
    std::lock_guard<std::mutex> lock(mtx_);
    if (socket_fd_ >= 0) {
        ::close(socket_fd_);
        socket_fd_ = -1;
        std::cout << "[IpcClient] Disconnected\n";
    }
}

bool IpcClient::send_message(const nlohmann::json &message) {
    int fd = -1;
    {
        std::lock_guard<std::mutex> lock(mtx_);
        fd = socket_fd_;
    }
    if (fd < 0) {
        std::cerr << "[IpcClient] Error: Not connected\n";
        return false;
    }

    std::string data = message.dump();
    uint32_t len = data.size();
    uint32_t net_len = htonl(len);

    if (!write_full(fd, &net_len, 4)) {
        std::cerr << "[IpcClient] Error: Failed to write length prefix\n";
        disconnect();
        return false;
    }

    if (!write_full(fd, data.data(), data.size())) {
        std::cerr << "[IpcClient] Error: Failed to write complete payload\n";
        disconnect();
        return false;
    }

    return true;
}

bool IpcClient::read_message(nlohmann::json &message) {
    int fd = -1;
    {
        std::lock_guard<std::mutex> lock(mtx_);
        fd = socket_fd_;
    }
    if (fd < 0) {
        return false;
    }

    uint32_t net_len = 0;
    if (!read_full(fd, &net_len, 4)) {
        disconnect();
        return false;
    }

    uint32_t len = ntohl(net_len);

    if (len > MAX_MESSAGE_SIZE) {
        std::cerr << "[IpcClient] Error: Message too large: " << len << " bytes (max " << MAX_MESSAGE_SIZE << ")\n";
        disconnect();
        return false;
    }

    std::string payload(len, '\0');
    if (!read_full(fd, &payload[0], len)) {
        std::cerr << "[IpcClient] Error: Failed to read payload\n";
        disconnect();
        return false;
    }

    try {
        message = nlohmann::json::parse(payload);
        return true;
    } catch (const std::exception &e) {
        std::cerr << "[IpcClient] Error parsing JSON payload: " << e.what() << "\n";
        return false;
    }
}

} // namespace ipc
