#ifndef FILESYSTEM_WATCHER_H
#define FILESYSTEM_WATCHER_H

#include <string>
#include <functional>
#include <memory>
#include <chrono>
#include <vector>
#include <atomic>

enum class WatchEventType { Created, Modified, Deleted, Moved };

struct WatchEvent {
    WatchEventType type;
    std::string path;
    std::string newPath;
};

class FileSystemWatcher {
public:
    using Callback = std::function<void(const WatchEvent&)>;

    FileSystemWatcher(const std::string& watchPath, Callback callback);
    virtual ~FileSystemWatcher() = default;

    virtual bool start() = 0;
    virtual void stop();
    bool isRunning() const { return running_; }

protected:
    std::string watchPath_;
    Callback callback_;
    std::atomic<bool> running_{false};
};

std::unique_ptr<FileSystemWatcher> createWatcher(
    const std::string& watchPath,
    FileSystemWatcher::Callback callback,
    std::chrono::milliseconds pollInterval = std::chrono::milliseconds(1000));

#endif
