#pragma once

#include <string>
#include <nlohmann/json.hpp>

namespace ipc {

class IpcClient {
public:
    IpcClient();
    ~IpcClient();

    bool connect(const std::string &socket_path);
    void disconnect();
    bool send_message(const nlohmann::json &message);
    bool read_message(nlohmann::json &message);
    bool is_connected() const { return socket_fd_ >= 0; }

private:
    int socket_fd_;
};

} // namespace ipc
