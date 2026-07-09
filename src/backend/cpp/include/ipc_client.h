#ifndef IPC_CLIENT_H
#define IPC_CLIENT_H

#include <string>
#include <memory>
#include <nlohmann/json.hpp>

class IpcClient {
public:
    virtual ~IpcClient() = default;
    virtual bool connect(const std::string& path) = 0;
    virtual bool send(const std::string& message) = 0;
    virtual bool receive(std::string& message) = 0;
    virtual void disconnect() = 0;
    virtual bool isConnected() = 0;

    static std::unique_ptr<IpcClient> create();
};

#endif
